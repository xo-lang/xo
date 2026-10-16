// GET one URL over HTTPS (URL, default https://example.com/) and print the
// status and the body's length.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	url := os.Getenv("URL")
	if url == "" {
		url = "https://example.com/"
	}
	r, err := http.Get(url)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer r.Body.Close()
	b, err := io.ReadAll(r.Body)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(r.StatusCode, len(b))
}
