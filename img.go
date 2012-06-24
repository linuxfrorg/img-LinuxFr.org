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

// The directory for caching files
var directory string

// The secret for checking the HMAC on requests
var secret []byte

// Read an entry of cache from the disk and send it to the chan
func readEntryFromDisk(filename string, ch chan *CacheEntry) {
	f, err := os.Open(filename)
	if err != nil {
		ch <- nil
		return
	}
	defer f.Close()

	entry := &CacheEntry{}
	dec := gob.NewDecoder(f)
	err = dec.Decode(entry)
	if err != nil {
		entry = nil
	}
	ch <- entry
}

// Save an entry of cache on the disk
func saveEntryToDisk(filename string, entry *CacheEntry) {
	dirname := path.Dir(filename)
	err := os.MkdirAll(dirname, 0755)
	if err != nil {
		return
	}

	f, err := os.OpenFile(filename+".tmp", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	enc := gob.NewEncoder(f)
	err = enc.Encode(entry)
	if err != nil {
		os.Remove(filename + ".tmp")
		return
	}
	err = os.Rename(filename+".tmp", filename)
	if err != nil {
		os.Remove(filename + ".tmp")
		return
	}
}

// Run the cache in its own goroutine
func runCache() {
	for {
		select {
		case read := <-readChan:
			filename := path.Join(directory, read.key)
			go readEntryFromDisk(filename, read.ch)
		case save := <-saveChan:
			filename := path.Join(directory, save.key)
			go saveEntryToDisk(filename, save.entry)
		}
	}
}

// Generate a key for cache from a string
func generateKeyForCache(s string) string {
	h := sha1.New()
	io.WriteString(h, s)
	key := h.Sum(nil)

	// Use 3 levels of hasing to avoid having too many files in the same directory
	return fmt.Sprintf("%x/%x/%x/%x", key[0:1], key[1:2], key[2:3], key[3:])
}

// Fetch image from cache
func fetchImageFromCache(key string) (contentType string, body []byte, ok bool) {
	ch := make(chan *CacheEntry)
	readChan <- ReadRequest{key, ch}
	entry := <-ch
	if ok = entry != nil; ok {
		contentType = entry.ContentType
		body = entry.Body
	}
	return
}

// Save the body and the headers in cache
func saveImageInCache(key string, contentType string, body []byte) {
	entry := CacheEntry{contentType, body}
	saveChan <- SaveRequest{key, &entry}
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
	fmt.Printf("Content-Type = %s\n", contentType)

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
		http.Error(w, "Invalid parameters", 400)
		return
	}
	if expected != actual {
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
