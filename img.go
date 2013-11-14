package main

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"github.com/nfnt/resize"
	"github.com/vmihailenco/redis"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The URL for the default avatar
const DefaultAvatarUrl = "//linuxfr.org/images/default-avatar.png"

// The maximal size for an image is 5MB
const MaxSize = 5 * (1 << 20)

// Force the height of the avatar, width is computed to preserve ratio
const AvatarHeight = 64

// Don't try ro refresh the cache more than once per hour
const CacheRefreshInterval = 3600

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
		m := resize.Resize(0, AvatarHeight, img, resize.Lanczos3Lut)
		var buf bytes.Buffer
		switch format {
		case "png":
			png.Encode(&buf, m)
		case "jpeg":
			jpeg.Encode(&buf, m, nil)
		}
		if buf.Len() == 0 {
			return body
		}
		return buf.Bytes()
	},
	func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", DefaultAvatarUrl)
		w.WriteHeader(http.StatusFound)
	},
}

// The directory for caching files
var directory string

// The connection to redis
var connection *redis.Client

// Check if an URL is valid and not temporary in error
func urlStatus(uri string) error {
	hexists := connection.HExists("img/"+uri, "created_at")
	if err := hexists.Err(); err != nil {
		return err
	}
	if ok := hexists.Val(); !ok {
		return errors.New("Invalid URL")
	}

	hget := connection.HGet("img/"+uri, "status")
	if err := hget.Err(); err == nil {
		if status := hget.Val(); status == "Blocked" {
			return errors.New("Invalid URL")
		}
	}

	get := connection.Get("img/err/" + uri)
	if err := get.Err(); err == nil {
		str := get.Val()
		return errors.New(str)
	}

	return nil
}

// Generate a key for cache from a string
func generateKeyForCache(s string) string {
	h := sha1.New()
	io.WriteString(h, s)
	key := h.Sum(nil)

	// Use 3 levels of hasing to avoid having too many files in the same directory
	return fmt.Sprintf("%s/%x/%x/%x/%x", directory, key[0:1], key[1:2], key[2:3], key[3:])
}

// Generate a key for cache from a string
func generateChecksumForCache(body []byte) string {
	h := sha1.New()
	h.Write(body)
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Retrieve mtime of the cached file
func getModTime(uri string) (modTime string, err error) {
	filename := generateKeyForCache(uri)
	stat, err := os.Stat(filename)
	if err != nil {
		return
	}
	modTime = stat.ModTime().Format(time.RFC1123)
	return
}

// Tell the cache that the metadata we have for that URL is still valid
func resetCacheTimer(uri string) {
	mtime, err := getModTime(uri)
	if err != nil {
		log.Printf("Couldn't Get mtime while resetting cache timer for %s: %s\n", uri, err)
		return
	}
	connection.Set("img/updated/"+uri, mtime)
	connection.Expire("img/updated/"+uri, CacheRefreshInterval)
}

// Fetch image from cache
func fetchImageFromCache(uri string, behaviour Behaviour) (headers Headers, body []byte, ok bool) {
	ok = false

	hget := connection.HGet("img/"+uri, "type")
	if err := hget.Err(); err != nil {
		return
	}
	contentType := hget.Val()

	filename := generateKeyForCache(uri)
	lastModified, err := getModTime(uri)
	if err != nil {
		return
	}

	headers.contentType = contentType
	headers.lastModified = lastModified

	body, err = ioutil.ReadFile(filename)
	ok = err == nil

	exists := connection.Exists("img/updated/" + uri)
	if err := exists.Err(); err == nil {
		if present := exists.Val(); !present {
			go fetchImageFromServer(uri, behaviour)
		}
	}

	return
}

// Save the body and the content-type header in cache
func saveImageInCache(uri string, headers Headers, body []byte) {
	go func() {
		checksum := generateChecksumForCache(body)
		hget := connection.HGet("img/"+uri, "checksum")
		if err := hget.Err(); err == nil {
			if was := hget.Val(); checksum == was {
				resetCacheTimer(uri)
				return
			}
		}

		filename := generateKeyForCache(uri)
		dirname := path.Dir(filename)
		err := os.MkdirAll(dirname, 0755)
		if err != nil {
			return
		}

		// Save the body on disk
		err = ioutil.WriteFile(filename, body, 0644)
		if err != nil {
			log.Printf("Error while writing %s\n", filename)
			return
		}

		// And other infos in redis
		connection.HSet("img/"+uri, "type", headers.contentType)
		connection.HSet("img/"+uri, "checksum", checksum)
		resetCacheTimer(uri)
	}()
}

// Save the error in redis for 10 minutes
func saveErrorInCache(uri string, err error) {
	go func() {
		connection.Set("img/err/"+uri, err.Error())
		connection.Expire("img/err/"+uri, CacheRefreshInterval)
	}()
}

// Fetch the image from the distant server
func fetchImageFromServer(uri string, behaviour Behaviour) (headers Headers, body []byte, err error) {
	// Accepts any certificate in HTTPS
	cfg := &tls.Config{InsecureSkipVerify: true}
	tr := &http.Transport{TLSClientConfig: cfg}
	client := &http.Client{Transport: tr}
	res, err := client.Get(uri)
	if err != nil {
		log.Printf("Error on client.Get %s: %s\n", uri, err)
		return
	}
	defer res.Body.Close()

	if res.StatusCode != 200 {
		log.Printf("Status code of %s is: %d\n", uri, res.StatusCode)
		err = errors.New("Unexpected status code")
		saveErrorInCache(uri, err)
		return
	}
	if res.ContentLength > MaxSize {
		log.Printf("Exceeded max size for %s: %d\n", uri, res.ContentLength)
		err = errors.New("Exceeded max size")
		saveErrorInCache(uri, err)
		return
	}
	contentType := res.Header.Get("Content-Type")
	if len(contentType) < 5 || contentType[0:5] != "image" {
		log.Printf("%s has an invalid content-type: %s\n", uri, contentType)
		err = errors.New("Invalid content-type")
		saveErrorInCache(uri, err)
		return
	}
	log.Printf("Fetch %s (%s)\n", uri, contentType)

	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		log.Printf("Error on ioutil.ReadAll for %s: %s\n", uri, err)
		return
	}

	body = behaviour.Manipulate(body)

	headers.contentType = contentType
	headers.lastModified = time.Now().Format(time.RFC1123)
	if urlStatus(uri) == nil {
		saveImageInCache(uri, headers, body)
	}
	return
}

// Fetch image from cache if available, or from the server
func fetchImage(uri string, behaviour Behaviour) (headers Headers, body []byte, err error) {
	err = urlStatus(uri)
	if err != nil {
		return
	}

	headers, body, ok := fetchImageFromCache(uri, behaviour)
	if !ok {
		headers, body, err = fetchImageFromServer(uri, behaviour)
	}
	headers.cacheControl = fmt.Sprintf("public, max-age=%d", CacheRefreshInterval)

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
	w.Write(body)
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
	fmt.Fprintf(w, "OK")
}

func main() {
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Parse the command-line
	var addr string
	var logs string
	var conn string
	flag.StringVar(&addr, "a", "127.0.0.1:8000", "Bind to this address:port")
	flag.StringVar(&logs, "l", "-", "Use this file for logs")
	flag.StringVar(&conn, "r", "localhost:6379/0", "The redis database to use for caching meta")
	flag.StringVar(&directory, "d", "cache", "The directory for the caching files")
	flag.Parse()

	// Logging
	if logs != "-" {
		f, err := os.OpenFile(logs, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal("OpenFile: ", err)
		}
		syscall.Dup2(int(f.Fd()), int(os.Stdout.Fd()))
		syscall.Dup2(int(f.Fd()), int(os.Stderr.Fd()))
	}

	// Redis
	parts := strings.Split(conn, "/")
	host := parts[0]
	db := 0
	if len(parts) >= 2 {
		db, _ = strconv.Atoi(parts[1])
	}
	fmt.Printf("Connection %s  %d\n", host, db)
	connection = redis.NewTCPClient(host, "", int64(db))
	defer connection.Close()

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
