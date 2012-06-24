package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/gob"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
)

type CacheEntry struct {
	ContentType string
	Body        []byte
}

// The directory for caching files
var directory string

// The secret for checking the HMAC on requests
var secret []byte

// Fetch image from cache
func fetchImageFromCache(filename string) (contentType string, body []byte, ok bool) {
	ok = false

	f, err := os.Open(filename)
	if err != nil {
		return
	}
	defer f.Close()

	var entry CacheEntry
	dec := gob.NewDecoder(f)
	err = dec.Decode(&entry)
	if err != nil {
		log.Printf("Error while decoding %s\n", filename)
	} else {
		contentType = entry.ContentType
		body = entry.Body
		ok = true
	}

	return
}

// Save the body and the headers in cache (on disk)
func saveImageInCache(filename string, contentType string, body []byte) {
	dirname := path.Dir(filename)
	err := os.MkdirAll(dirname, 0755)
	if err != nil {
		return
	}

	f, err := os.OpenFile(filename+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		log.Printf("Error while opening %s\n", filename)
		return
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	err = enc.Encode(CacheEntry{contentType, body})
	if err != nil {
		log.Printf("Error while encoding %s\n", filename)
		os.Remove(filename + ".tmp")
		return
	}
	err = os.Rename(filename+".tmp", filename)
	if err != nil {
		log.Printf("Error while renaming %s\n", filename)
		os.Remove(filename + ".tmp")
	}
}

// Generate a key for cache from a string
func generateKeyForCache(s string) string {
	h := sha1.New()
	io.WriteString(h, s)
	key := h.Sum(nil)

	// Use 3 levels of hasing to avoid having too many files in the same directory
	return fmt.Sprintf("%s/%x/%x/%x/%x", directory, key[0:1], key[1:2], key[2:3], key[3:])
}

// Fetch the image from the distant server
func fetchImageFromServer(url string) (contentType string, body []byte, err error) {
	res, err := http.Get(url)
	if err != nil {
		return
	}

	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	contentType = res.Header.Get("Content-Type")
	log.Printf("Fetch %s (%s)\n", url, contentType)

	return
}

// Fetch image from cache if available, or from the server
func fetchImage(url string) (contentType string, body []byte, err error) {
	key := generateKeyForCache(url)

	contentType, body, ok := fetchImageFromCache(key)
	if ok {
		return
	}

	contentType, body, err = fetchImageFromServer(url)
	if err == nil {
		go saveImageInCache(key, contentType, body)
	}

	return
}

// Decode the URL and compute the associated HMAC
func decodeUrl(encoded_url string) (url []byte, actual string, err error) {
	url, err = hex.DecodeString(encoded_url)
	if err != nil {
		return
	}

	h := hmac.New(sha1.New, secret)
	_, err = h.Write(url)
	if err != nil {
		return
	}
	actual = fmt.Sprintf("%x", h.Sum(nil))

	return
}

// Fetch the image from the URL (or cache) and respond with it
func Img(w http.ResponseWriter, r *http.Request) {
	expected := r.URL.Query().Get(":hmac")
	encoded_url := r.URL.Query().Get(":encoded_url")
	url, actual, err := decodeUrl(encoded_url)
	if err != nil {
		log.Printf("Invalid URL %s%s\n", encoded_url)
		http.Error(w, "Invalid parameters", 400)
		return
	}
	if expected != actual {
		log.Printf("Invalid HMAC expected=%s actual=%s\n", expected, actual)
		http.Error(w, "Invalid HMAC", 403)
		return
	}
	contentType, body, err := fetchImage(string(url))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Add("Content-Type", contentType)
	w.Write(body)
}

// Returns 200 OK if the server is running (for monitoring)
func Status(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "OK")
}

func main() {
	// Parse the command-line
	var addr string
	var secr string
	flag.StringVar(&addr, "a", "127.0.0.1:8000", "Bind to this address:port")
	flag.StringVar(&secr, "s", "252c38cdb9f638908fab5df7263d156c759d590b1251785fa612e7874ee9bbcc32a61f8d795e7593ca31f8f47396c497b215e1abde6e947d7e25772f30115a7e", "The secret for HMAC check")
	flag.StringVar(&directory, "d", "cache", "The directory for the caching files")
	flag.Parse()
	secret = []byte(secr)

	// Routing
	m := pat.New()
	m.Get("/status", http.HandlerFunc(Status))
	m.Get("/img/:hmac/:encoded_url", http.HandlerFunc(Img))
	http.Handle("/", m)

	// Start the HTTP server
	log.Printf("Listening on http://%s/\n", addr)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
