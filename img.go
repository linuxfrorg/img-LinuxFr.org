package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bmizerany/pat"
	httpclient "github.com/mreiferson/go-httpclient"
	"github.com/nfnt/resize"
	redis "gopkg.in/redis.v3"
	"path/filepath"
)

// The maximal size for an image is 5MiB
const MaxSize = 5 * (1 << 20)

// Force the height of the avatar, width is computed to preserve ratio
const AvatarHeight = 64

// Don't try ro refresh the cache more than once per hour
const CacheRefreshInterval = 1 * time.Hour

// HTTP headers struct
type Headers struct {
	contentType  string
	lastModified string
	cacheControl string
}

// Behaviour is a way to customize handlers
type Behaviour struct {
	// Manipulate the image before sending it (resize for example)
	Manipulate func(body []byte) []byte
	// NotFound is called when we can't find a valid image at the original location
	NotFound func(http.ResponseWriter, *http.Request)
}

// The behaviour for normal images
var ImgBehaviour = Behaviour{
	func(body []byte) []byte {
		return body
	},
	func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	},
}

// The behaviour for avatars
var AvatarBehaviour = Behaviour{
	func(body []byte) []byte {
		img, format, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			return body
		}
		m := resize.Resize(0, AvatarHeight, img, resize.Lanczos3)
		var buf bytes.Buffer
		switch format {
		case "png":
			err = png.Encode(&buf, m)
		case "jpeg":
			err = jpeg.Encode(&buf, m, nil)
		}
		if err != nil || buf.Len() == 0 {
			return body
		}
		return buf.Bytes()
	},
	func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", defaultAvatarUrl)
		w.WriteHeader(http.StatusFound)
	},
}

// The directory for caching files
var directory string

// The connection to redis
var connection *redis.Client

// The HTTP client
var httpClient *http.Client

// The User-Agent to use for HTTP requests
var userAgent string

// The address for avatars by default
var defaultAvatarUrl string

// Check if an URL is valid and not temporarily in error
func urlStatus(uri string) error {
	hexists := connection.HExists("img/" + uri, "created_at")
	if err := hexists.Err(); err != nil {
		return err
	}
	if ok := hexists.Val(); !ok {
		return errors.New("invalid URL")
	}

	hget := connection.HGet("img/" + uri, "status")
	if err := hget.Err(); err == nil {
		if status := hget.Val(); status == "Blocked" {
			return errors.New("invalid URL")
		}
	}

	get := connection.Get("img/err/" + uri)
	if err := get.Err(); err == nil {
		str := get.Val()
		try := connection.HGet("img/" + uri, "checksum")
		if try.Err() == nil && try.Val() != "" {
			return nil // fallback on cache disk despite error
		}
		return errors.New(str)
	}

	return nil
}

// Generate a key for cache from a string
func generateKeyForCache(s string) (string, error) {
	h := sha1.New()
	_, err := io.WriteString(h, s)
	if err != nil {
		return "", errors.New("unable to generate key for cache")
	}
	key := h.Sum(nil)

	// Use 3 levels of hashing to avoid having too many files in the same directory
	return fmt.Sprintf("%s/%x/%x/%x/%x", directory, key[0:1], key[1:2], key[2:3], key[3:]), nil
}

// Generate a checksum for cache from a string
func generateChecksumForCache(body []byte) string {
	h := sha1.New()
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Retrieve mtime of the cached file
func getModTime(uri string) (modTime string, err error) {
	filename, err := generateKeyForCache(uri)
	if err != nil {
		return
	}
	stat, err := os.Stat(filename)
	if err != nil {
		return
	}
	gmt, err := time.LoadLocation("GMT")
	if err != nil {
		return
	}
	modTime = stat.ModTime().In(gmt).Format(time.RFC1123)
	return
}

// Tell the cache that the metadata we have for that URL is still valid
func resetCacheTimer(uri string) {
	mtime, err := getModTime(uri)
	if err != nil {
		log.Printf("Couldn't Get mtime while resetting cache timer for %s: %s\n", uri, err)
		return
	}
	connection.Set("img/updated/" + uri, mtime, CacheRefreshInterval)
}

// Fetch image from cache
func fetchImageFromCache(uri string, behaviour Behaviour) (headers Headers, body []byte, err error) {
	exists := connection.Exists("img/updated/" + uri)
	if exists.Err() != nil || !exists.Val() {
		err = fetchImageFromServer(uri, behaviour)
		if err != nil {
			hget := connection.HGet("img/" + uri, "checksum")
			if hget.Err() != nil || hget.Val() == "" {
				return
			} else {
				log.Printf("Fail to fetch %s (serve from disk cache anyway)\n", uri)
			}
		}
	}

	hget := connection.HGet("img/" + uri, "type")
	if err = hget.Err(); err != nil {
		return
	}
	contentType := hget.Val()

	filename, err := generateKeyForCache(uri)
	if err != nil {
		return
	}
	lastModified, err := getModTime(uri)
	if err != nil {
		return
	}

	headers.contentType = contentType
	headers.lastModified = lastModified

	body, err = os.ReadFile(filename)

	return
}

// Save the body and the content-type header in cache
func saveImageInCache(uri string, contentType string, etag string, body []byte) (err error) {
	checksum := generateChecksumForCache(body)
	hget := connection.HGet("img/" + uri, "checksum")
	if err = hget.Err(); err == nil {
		if was := hget.Val(); checksum == was {
			resetCacheTimer(uri)
			return
		}
	}

	filename, err := generateKeyForCache(uri)
	if err != nil {
		return
	}
	dirname := path.Dir(filename)
	err = os.MkdirAll(dirname, 0755)
	if err != nil {
		return
	}

	// Save the body on disk
	err = os.WriteFile(filename, body, 0644)
	if err != nil {
		log.Printf("Error while writing %s\n", filename)
		return
	}

	// And other infos in redis
	connection.HSet("img/" + uri, "type", contentType)
	connection.HSet("img/" + uri, "checksum", checksum)
	if etag == "" {
		connection.HDel("img/" + uri, "etag")
	} else {
		connection.HSet("img/" + uri, "etag", etag)
	}
	resetCacheTimer(uri)

	return
}

// Save the error in redis for the cache refresh interval duration
func saveErrorInCache(uri string, err error) {
	go func() {
		connection.Set("img/err/" + uri, err.Error(), CacheRefreshInterval)
	}()
}

// Fetch the image from the distant server
func fetchImageFromServer(uri string, behaviour Behaviour) (err error) {
	req, err := http.NewRequest("GET", uri, nil)
	if err != nil {
		log.Printf("Error on http.NewRequest GET %s: %s\n", uri, err)
		return
	}
	hget := connection.HGet("img/" + uri, "etag")
	if err = hget.Err(); err == nil {
		etag := hget.Val()
		req.Header.Set("If-None-Match", etag)
	}

	req.Header.Set("User-Agent", userAgent)
	res, err := httpClient.Do(req)
	if err != nil {
		log.Printf("Error on httpClient.Get %s: %s\n", uri, err)
		return
	}
	defer func() {
		err := res.Body.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	if res.StatusCode == 304 {
		resetCacheTimer(uri)
		err = nil
		return
	}
	if res.StatusCode != 200 {
		log.Printf("Status code of %s is: %d\n", uri, res.StatusCode)
		err = errors.New("unexpected status code")
		saveErrorInCache(uri, err)
		return
	}
	if res.ContentLength > MaxSize {
		log.Printf("Exceeded max size for %s: %d\n", uri, res.ContentLength)
		err = errors.New("exceeded max size")
		saveErrorInCache(uri, err)
		return
	}
	fullContentType := res.Header.Get("Content-Type")
	if len(fullContentType) < 5 || fullContentType[0:5] != "image" {
		log.Printf("%s has an invalid content-type: %s\n", uri, fullContentType)
		err = errors.New("invalid content-type")
		saveErrorInCache(uri, err)
		return
	}
	contentType,_,_ := strings.Cut(fullContentType, ";")

	etag := res.Header.Get("ETag")
	log.Printf("Fetch %s (%s) (ETag: %s)\n", uri, contentType, etag)

	body, err := io.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error on io.ReadAll for %s: %s\n", uri, err)
		return
	}

	body = behaviour.Manipulate(body)

	if urlStatus(uri) == nil {
		err = saveImageInCache(uri, contentType, etag, body)
	}
	return
}

// Fetch image from cache if available, or from the server
func fetchImage(uri string, behaviour Behaviour) (headers Headers, body []byte, err error) {
	err = urlStatus(uri)
	if err != nil {
		return
	}

	headers, body, err = fetchImageFromCache(uri, behaviour)
	headers.cacheControl = fmt.Sprintf("public, max-age=%d", CacheRefreshInterval/time.Second)

	return
}

// Receive an HTTP request, fetch the image and respond with it
func Image(w http.ResponseWriter, r *http.Request, behaviour Behaviour) {
	encoded_url := r.URL.Query().Get(":encoded_url")
	chars, err := hex.DecodeString(encoded_url)
	if err != nil {
		log.Printf("Invalid URL %s\n", encoded_url)
		http.Error(w, "Invalid parameters", 400)
		return
	}
	uri := string(chars)

	headers, body, err := fetchImage(uri, behaviour)
	if err != nil {
		behaviour.NotFound(w, r)
		return
	}
	if headers.lastModified == r.Header.Get("If-Modified-Since") {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.Header().Add("Content-Type", headers.contentType)
	w.Header().Add("Last-Modified", headers.lastModified)
	w.Header().Add("Cache-Control", headers.cacheControl)
	_, err = w.Write(body)
	if err != nil {
		http.Error(w, "Internal server error", 500)
		return
	}
}

// Receive an HTTP request for an image and respond with it
func Img(w http.ResponseWriter, r *http.Request) {
	Image(w, r, ImgBehaviour)
}

// Receive an HTTP request for an avatar and respond with it
func Avatar(w http.ResponseWriter, r *http.Request) {
	Image(w, r, AvatarBehaviour)
}

// Returns 200 OK if the server is running (for monitoring)
func Status(w http.ResponseWriter, r *http.Request) {
	_, err := fmt.Fprintf(w, "OK")
	if err != nil {
			log.Fatal(err)
	}
}

// Return a map filepath / checksum from cache disk
func FilesInCache(root string) (map[string]string, error) {
	var files map[string]string
	var body []byte
	files = make(map[string]string)
	werr := filepath.Walk(root, func(path string, info os.FileInfo, err error) (error) {
		// pedantic: could check that directory is named [0-9a-f][0-9a-f]
		if err != nil {
			fmt.Printf("Prevent panic by handling failure accessing a path %q: %v\n", path, err)
			return err
		} else if !info.IsDir() {
			body, err = os.ReadFile(path)
			if (err != nil) {
				log.Printf("Error while reading files: %s\n", err);
				return err
			} else {
				h := sha1.New()
				h.Write(body)
				files[path] = fmt.Sprintf("%x", h.Sum(nil))
			}
		}
		return nil
	})
	return files, werr
}

// Ensure entries from img/latest exist as img/<uri>
func sanityCheckImgLatest() (latest_img []string) {
	latest_img, err := connection.LRange("img/latest", 0, -1).Result();
	if err != nil {
		log.Printf("Error getting img/latest entry: %s\n", err);
	} else {
		for _, v := range latest_img {
			img, err := connection.Exists("img/" + v).Result()
			if (err != nil) {
				log.Printf("Error getting %s listed in img/latest: %s\n", v, err);
			} else if !img {
				log.Printf("Image %s listed in img/latest but unknown\n", v);
			}
		}
	}
	return latest_img
}

// Ensure entries from img/blocked exist as img/<uri>
func sanityCheckImgBlocked() (blocked_img []string) {
	blocked_img, err := connection.LRange("img/blocked", 0, -1).Result();
	if err != nil {
		log.Printf("Error getting img/blocked entry: %s\n", err);
	} else if len(blocked_img) > 0 {
		log.Printf("Entries still in getting img/blocked list: %d\n", len(blocked_img));
		for _, v := range blocked_img {
			img, err := connection.Exists("img/" + v).Result()
			if (err != nil) {
				log.Printf("Error getting %s listed in img/blocked: %s\n", v, err);
			} else if !img {
				log.Printf("Image %s listed in img/blocked but unknown\n", v);
			}
		}
	}
	return blocked_img
}

// Scan img related keys in redis and files in cache disk for sanity check
func sanityCheck() (bool) {
	sane := true
	files, err := FilesInCache(directory);
	if err != nil {
		log.Printf("Unable to walk through cache disk: %s\n", err);
		return false
	}
	log.Printf("Files in cache: %d\n", len(files));

	sanityCheckImgLatest()
	blocked_img := sanityCheckImgBlocked()

	var cursor int64
	var total_img_keys int

	for {
		var keys []string
		var err error
		cursor, keys, err = connection.Scan(cursor, "img/*", 10000).Result()
		if err != nil {
			panic(err)
		}
		total_img_keys += len(keys)
		for _, img_key := range keys {
			if strings.HasPrefix(img_key, "img/latest") || strings.HasPrefix(img_key, "img/blocked") {
				// handled above
			} else if strings.HasPrefix(img_key, "img/updated/") || strings.HasPrefix(img_key, "img/err/") {
				ttl, err := connection.TTL(img_key).Result()
				if (err != nil) {
					log.Printf("Error getting TTL on %s", img_key);
					sane = false
				} else if ttl == -1 * time.Second {
					log.Printf("Missing TTL %s", img_key);
					sane = false
				}
			} else { // img/<uri>
				hgetall, err := connection.HGetAll(img_key).Result()
				if err != nil {
					log.Printf("Image %s: unable to get all fields/values %s\n", img_key, err);
					sane = false
				} else {
					uri := img_key[4:]
					has_created_at := false
					has_type := false
					has_checksum := false
					allowed_types := [...]string{"image/jpeg", "image/png", "image/gif", "image/svg+xml", "image/webp", "image/tiff", "image/avif", "image/x-ms-bmp", "image/bmp", "image/x-icon", "image/vnd.microsoft.icon"};
					for i := 0; i < len(hgetall) ; i += 1 {
						field := hgetall[i]
						i += 1
						value := hgetall[i]
						switch field {
							case "created_at":
								has_created_at = true
								// pedantic: could check it's an int, lesser or equals to img/latest value
							case "type":
								has_type = true
								allowed_type := false
								for _, v := range allowed_types {
									if v == value {
										allowed_type = true
										break
									}
								}
								if !allowed_type {
									log.Printf("Image %s: unknown value %s for field %s\n", img_key, value, field);
									sane = false
								}
								// pedantic: could check type vs file
							case "checksum":
								has_checksum = true
							case "etag":
								// nothing
							case "status":
								if value != "Blocked" {
									log.Printf("Image %s: unknown value %s for field %s\n", img_key, value, field);
									sane = false
								} else {
									found_in_blocked := false
									for _, v := range blocked_img {
										if v == uri {
											found_in_blocked = true
											break
										}
									}
									if !found_in_blocked {
										log.Printf("Image %s with status Blocked not declared in img/blocked list\n", img_key);
										sane = false
									}
								}
							default:
								log.Printf("Image %s: unknown field %s %s\n", img_key, field, value);
								sane = false
						}
					}
					if !has_created_at {
						log.Printf("Image %s: no created_at field\n", img_key);
						sane = false
					}
					if !has_checksum {
						sane = false
						img_updated_key := "img/updated/" + uri
						img_err_key := "img/err/" + uri
						no_checksum := fmt.Sprintf("Image %s: no checksum field", img_key);
						key, err := generateKeyForCache(uri)
						if err != nil {
							log.Fatal("generateKeyForCache: ", err)
						} else if files[key] != "" {
							no_checksum += " (unexpected cache entry exists)";
						}
						exists_updated, err := connection.Exists(img_updated_key).Result()
						if (err != nil) {
							no_checksum += fmt.Sprintf(" (error on /updated/ %s)", err);
						} else {
							no_checksum += fmt.Sprintf(" (/updated/? %t)", exists_updated);
						}
						exists_err, err := connection.Exists(img_err_key).Result()
						if (err != nil) {
							no_checksum += fmt.Sprintf(" (error on /err/ %s)", err);
						} else {
							no_checksum += fmt.Sprintf(" (/err/? %t)", exists_err);
						}
						if !has_type {
							no_checksum += " (no type field)";
						}
						log.Printf("%s\n", no_checksum);
					} else { // has_checksum
						if !has_type {
							log.Printf("Image %s: no type field\n", img_key);
							sane = false
						}
						key, err := generateKeyForCache(uri);
						if err != nil {
							log.Fatal("generateKeyForCache: ", err);
						} else if files[key] == "" {
							log.Printf("Image %s has a checksum but no file in cache\n", img_key);
							sane = false
						} else {
							// remove found filename/checksum to detect unknown files in cache (or collision)
							files[key] = ""
						}
					}
				}
			}
		}
		if cursor == 0 {
			break
		}
	}
	log.Printf("Total img keys in redis: %d\n", total_img_keys)
	for filename, checksum := range files {
		if checksum != "" {
			log.Printf("Unexpected file in cache: %s\n", filename)
			sane = false
		}
	}
	return sane
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Parse the command-line
	var addr string
	var logs string
	var conn string
	var check bool
	flag.StringVar(&addr, "a", "127.0.0.1:8000", "Bind to this address:port")
	flag.StringVar(&logs, "l", "-", "Use this file for logs")
	flag.StringVar(&conn, "r", "localhost:6379/0", "Use this redis database for caching meta")
	flag.StringVar(&directory, "d", "cache", "Cache files in this directory")
	flag.StringVar(&userAgent, "u", "img_LinuxFr.org/1.0", "Use this User-Agent making HTTP requests")
	flag.StringVar(&defaultAvatarUrl, "e", "//nginx/default-avatar.svg", "Default to this avatar URL")
	flag.BoolVar(&check, "c", false, "Do no start daemon, just do a sanity check on redis database and cache disk")
	flag.Parse()

	// Logging
	if logs != "-" {
		f, err := os.OpenFile(logs, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal("OpenFile: ", err)
		}
		err = syscall.Dup2(int(f.Fd()), int(os.Stdout.Fd()))
		if err != nil {
			log.Fatal("Dup2 (stdout): ", err)
		}
		err = syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
		if err != nil {
			log.Fatal("Dup2 (stderr): ", err)
		}
	}

	// Redis
	parts := strings.Split(conn, "/")
	host := parts[0]
	db := 0
	if len(parts) >= 2 {
		db, _ = strconv.Atoi(parts[1])
	}
	fmt.Printf("Connection %s %d\n", host, db)
	connection = redis.NewClient(&redis.Options{
		Addr: host,
		DB:   int64(db),
	})
	defer func() {
		err := connection.Close()
		if err != nil {
			log.Fatal(err)
		}
	}()

	// Cache directory
	stat, err := os.Stat(directory)
	if err != nil {
		log.Printf("Invalid cache directory %s: %s\n", directory, err)
		os.Exit(1)
	} else if !stat.IsDir() {
		log.Printf("Invalid cache directory %s: not a directory\n", directory)
		os.Exit(1)
	} else if directory[len(directory)-1:] == "/" {
		log.Printf("Invalid cache directory %s: do not put a final /\n", directory)
		os.Exit(1)
	} else if stat.Mode().Perm() & (1 << (uint(7))) == 0 {
		log.Printf("Invalid cache directory %s: unable to write in it as a user\n", directory)
		os.Exit(1)
	}

	if check {
		log.Printf("Sanity check mode only\n")
		if !sanityCheck() {
			os.Exit(1)
		}
	} else {
		// Accepts any certificate in HTTPS
		cfg := &tls.Config{InsecureSkipVerify: true}
		trp := &httpclient.Transport{
			TLSClientConfig:       cfg,
			ConnectTimeout:        10 * time.Second,
			RequestTimeout:        10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
		}
		httpClient = &http.Client{Transport: trp}

		// Routing
		m := pat.New()
		m.Get("/status", http.HandlerFunc(Status))
		m.Get("/img/:encoded_url/:filename", http.HandlerFunc(Img))
		m.Get("/img/:encoded_url", http.HandlerFunc(Img))
		m.Get("/avatars/:encoded_url/:filename", http.HandlerFunc(Avatar))
		m.Get("/avatars/:encoded_url", http.HandlerFunc(Avatar))
		http.Handle("/", m)

		// Start the HTTP server
		log.Printf("Listening on http://%s/\n", addr)
		server := &http.Server{
			Addr:         addr,
			ReadTimeout:  30 * time.Second,
			WriteTimeout: 30 * time.Second,
		}
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal("ListenAndServe: ", err)
		}
	}
}
