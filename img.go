package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"io"
	"io/ioutil"
	"log"
	"net/http"
)

// The secret for checking the HMAC on requests
var secret []byte

type CacheEntry struct {
	headers map[string]string
	body    []byte
}

type ReadRequest struct {
	key string
	ch  chan *CacheEntry
}

type SaveRequest struct {
	key   string
	entry *CacheEntry
}

var readChan chan ReadRequest
var saveChan chan SaveRequest

// Run the cache in its own goroutine
func runCache() {
	// TODO don't use an in-memory cache, but a file-disk one
	cache := make(map[string]CacheEntry)

	for {
		select {
		case read := <-readChan:
			entry, ok := cache[read.key]
			if ok {
				read.ch <- &entry
			} else {
				read.ch <- nil
			}
		case save := <-saveChan:
			cache[save.key] = *save.entry
		}
	}
}

// Generate a key for cache from a string
func generateKeyForCache(s string) string {
	h := sha1.New()
	io.WriteString(h, s)
	key := h.Sum(nil)

	// Use 2 levels of hasing to avoid having too many files in the same directory
	key[3] = '/'
	key[7] = '/'

	return fmt.Sprintf("%x", key)
}

// Fetch image from cache
func fetchImageFromCache(key string) (headers map[string]string, body []byte, ok bool) {
	ch := make(chan *CacheEntry)
	readChan <- ReadRequest{key, ch}
	entry := <-ch
	if ok = entry != nil; ok {
		headers = entry.headers
		body = entry.body
	}
	return
}

// Save the body and the headers in cache
func saveImageInCache(key string, headers map[string]string, body []byte) {
	entry := CacheEntry{headers, body}
	saveChan <- SaveRequest{key, &entry}
}

// Fetch image from cache if available, or from the server
func fetchImage(url string) (headers map[string]string, body []byte, err error) {
	key := generateKeyForCache(url)

	headers, body, ok := fetchImageFromCache(key)
	if ok {
		return
	}

	headers, body, err = fetchImageFromServer(url)
	if err == nil {
		go saveImageInCache(key, headers, body)
	}

	return
}

// Fetch the image from the distant server
func fetchImageFromServer(url string) (headers map[string]string, body []byte, err error) {
	res, err := http.Get(url)
	if err != nil {
		return
	}

	defer res.Body.Close()
	body, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	res.Body.Close()

	headers = make(map[string]string)
	for key, values := range res.Header {
		headers[key] = values[0]
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
		http.Error(w, "Invalid parameters", 400)
		return
	}
	if expected != actual {
		http.Error(w, "Invalid HMAC", 403)
		return
	}
	headers, body, err := fetchImage(string(url))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	for key, value := range headers {
		w.Header().Add(key, value)
	}
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
	flag.StringVar(&addr, "addr", "127.0.0.1:8000", "Bind to this address:port")
	flag.StringVar(&secr, "secret", "252c38cdb9f638908fab5df7263d156c759d590b1251785fa612e7874ee9bbcc32a61f8d795e7593ca31f8f47396c497b215e1abde6e947d7e25772f30115a7e", "The secret for HMAC check")
	flag.Parse()
	secret = []byte(secr)

	// Start the cache
	readChan = make(chan ReadRequest)
	saveChan = make(chan SaveRequest)
	go runCache()

	// Routing
	m := pat.New()
	m.Get("/status", http.HandlerFunc(Status))
	m.Get("/img/:hmac/:encoded_url", http.HandlerFunc(Img))
	http.Handle("/", m)

	// Start the HTTP server
	fmt.Printf("Listening on http://%s/\n", addr)
	err := http.ListenAndServe(addr, nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}
}
