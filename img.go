package main

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/bmizerany/pat"
	"log"
	"net/http"
)

// The secret for checking the HMAC on requests
var secret []byte

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
	fmt.Printf("url = %s\n", url)
	if expected != actual {
		fmt.Printf("hmac = %s vs %s\n", expected, actual)
		http.Error(w, "Invalid HMAC", 403)
		return
	}
	http.NotFound(w, r)
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
