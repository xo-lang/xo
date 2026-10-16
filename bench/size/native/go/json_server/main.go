// A JSON HTTP server: GET /items lists the items, POST /items adds one
// (JSON body {"name": ..., "qty": ...}), GET /items/{id} gets one.
package main

import (
	"encoding/json"
	"net/http"
	"os"
	"strconv"
	"sync"
)

type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Qty  int    `json:"qty"`
}

func main() {
	var mu sync.RWMutex
	var items []Item
	write := func(w http.ResponseWriter, code int, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items", func(w http.ResponseWriter, r *http.Request) {
		mu.RLock()
		defer mu.RUnlock()
		write(w, 200, items)
	})
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			http.Error(w, "bad id", 400)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		for _, it := range items {
			if it.ID == id {
				write(w, 200, it)
				return
			}
		}
		http.Error(w, "not found", 404)
	})
	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		n := struct {
			Name string `json:"name"`
			Qty  int    `json:"qty"`
		}{Qty: 1}
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		mu.Lock()
		it := Item{ID: len(items) + 1, Name: n.Name, Qty: n.Qty}
		items = append(items, it)
		mu.Unlock()
		write(w, 201, it)
	})
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	if err := http.ListenAndServe(addr, mux); err != nil {
		os.Exit(1)
	}
}
