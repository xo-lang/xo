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
	mux.HandleFunc("GET /alloc-hand", func(w http.ResponseWriter, r *http.Request) {
		b := appendPage(make([]byte, 0, 16384), pageStrconv(items))
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

// appendPage is json.Marshal(p) written by hand, GET /alloc-hand: Xo's
// derived encoder is code generated per type, encoding/json reflects, so
// this is the Go side of that comparison (decision 0324's option 3). The
// same bytes as json.Marshal: TestAppendPage.
func appendPage(b []byte, p Page) []byte {
	b = append(b, `{"total":`...)
	b = strconv.AppendInt(b, int64(p.Total), 10)
	b = append(b, `,"items":`...)
	if p.Items == nil {
		b = append(b, "null"...)
	} else {
		b = append(b, '[')
		for i, it := range p.Items {
			if i > 0 {
				b = append(b, ',')
			}
			b = append(b, `{"id":`...)
			b = strconv.AppendInt(b, int64(it.ID), 10)
			b = append(b, `,"name":`...)
			b = appendString(b, it.Name)
			b = append(b, `,"tags":`...)
			if it.Tags == nil {
				b = append(b, "null"...)
			} else {
				b = append(b, '[')
				for j, t := range it.Tags {
					if j > 0 {
						b = append(b, ',')
					}
					b = appendString(b, t)
				}
				b = append(b, ']')
			}
			b = append(b, `,"score":`...)
			b = appendFloat(b, it.Score)
			b = append(b, `,"active":`...)
			b = strconv.AppendBool(b, it.Active)
			b = append(b, '}')
		}
		b = append(b, ']')
	}
	return append(b, '}')
}

// appendString quotes s as encoding/json does for ASCII without <, >, & or
// controls; anything else goes through json.Marshal.
func appendString(b []byte, s string) []byte {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 || c >= 0x80 || c == '"' || c == '\\' || c == '<' || c == '>' || c == '&' {
			q, _ := json.Marshal(s)
			return append(b, q...)
		}
	}
	b = append(b, '"')
	b = append(b, s...)
	return append(b, '"')
}

// appendFloat is encoding/json's float64 form: 'f' unless the exponent is
// small or large, then 'e' with a two digit exponent.
func appendFloat(b []byte, f float64) []byte {
	abs := f
	if abs < 0 {
		abs = -abs
	}
	if abs != 0 && (abs < 1e-6 || abs >= 1e21) {
		q, _ := json.Marshal(f)
		return append(b, q...)
	}
	return strconv.AppendFloat(b, f, 'f', -1, 64)
}
