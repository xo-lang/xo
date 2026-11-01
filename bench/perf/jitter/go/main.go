// The jitter bench's Go server (bench/perf/jitter), xo/main.xo's twin:
// GET /hello answers a short text; GET /alloc builds a page of 200 items
// and encodes it as JSON with encoding/json.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
)

type Item struct {
	ID     int      `json:"id"`
	Name   string   `json:"name"`
	Tags   []string `json:"tags"`
	Score  float64  `json:"score"`
	Active bool     `json:"active"`
}

type Page struct {
	Total int    `json:"total"`
	Items []Item `json:"items"`
}

const items = 200

func page(n int) Page {
	var it []Item
	for i := 0; i < n; i++ {
		it = append(it, Item{ID: i, Name: fmt.Sprintf("item-%d", i), Tags: []string{fmt.Sprintf("t%d", i%7), fmt.Sprintf("g%d", i%5)}, Score: float64(i) * 0.5, Active: i%2 == 0})
	}
	return Page{Total: n, Items: it}
}

// pageStrconv is page with strconv in place of fmt.Sprintf, GET /alloc-strconv:
// how much of the CPU gap to Xo is the formatting idiom. /alloc stays the
// idiomatic twin.
func pageStrconv(n int) Page {
	var it []Item
	for i := 0; i < n; i++ {
		it = append(it, Item{ID: i, Name: "item-" + strconv.Itoa(i), Tags: []string{"t" + strconv.Itoa(i%7), "g" + strconv.Itoa(i%5)}, Score: float64(i) * 0.5, Active: i%2 == 0})
	}
	return Page{Total: n, Items: it}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "18080"
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /hello", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("hello\n"))
	})
	mux.HandleFunc("GET /alloc", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(page(items))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	})
	mux.HandleFunc("GET /alloc-strconv", func(w http.ResponseWriter, r *http.Request) {
		b, err := json.Marshal(pageStrconv(items))
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Length", strconv.Itoa(len(b)))
		w.Write(b)
	})
	if err := http.ListenAndServe("127.0.0.1:"+port, mux); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
