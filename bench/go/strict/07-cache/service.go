package benchtask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
)

var (
	// ErrNotFound is returned by an Upstream when the item does not exist.
	ErrNotFound = errors.New("upstream: item not found")
	// ErrUpstream is returned by HTTPUpstream for unexpected upstream responses.
	ErrUpstream = errors.New("upstream: bad response")
)

// Item is the resource served by the service.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Upstream fetches items from the slow backing service.
type Upstream interface {
	FetchItem(ctx context.Context, id string) (Item, error)
}

// HTTPUpstream is an Upstream backed by an HTTP API serving GET /items/{id}.
type HTTPUpstream struct {
	BaseURL string
	Client  *http.Client
}

// FetchItem implements Upstream.
func (u *HTTPUpstream) FetchItem(ctx context.Context, id string) (item Item, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.BaseURL+"/items/"+url.PathEscape(id), nil)
	if err != nil {
		return Item{}, fmt.Errorf("upstream request: %w", err)
	}
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Item{}, fmt.Errorf("upstream request: %w", err)
	}
	if resp == nil {
		return Item{}, fmt.Errorf("%w: no response", ErrUpstream)
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("upstream close: %w", cerr)
		}
	}()
	if resp.StatusCode == http.StatusNotFound {
		return Item{}, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return Item{}, fmt.Errorf("%w: status %d", ErrUpstream, resp.StatusCode)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&item); err != nil {
		return Item{}, fmt.Errorf("%w: decode: %v", ErrUpstream, err)
	}
	return item, nil
}

// Server serves items through a Cache.
type Server struct {
	cache *Cache[string, Item]
}

// NewServer creates a Server whose cache loads from up.
func NewServer(up Upstream, opts Options) (*Server, error) {
	if up == nil {
		return nil, fmt.Errorf("%w: nil upstream", ErrInvalidOptions)
	}
	c, err := New[string, Item](up.FetchItem, opts)
	if err != nil {
		return nil, err
	}
	return &Server{cache: c}, nil
}

// Metrics returns the cache metrics.
func (s *Server) Metrics() Metrics {
	return s.cache.Metrics()
}

// Handler returns the HTTP routes.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", s.getItem)
	mux.HandleFunc("DELETE /items/{id}/cache", s.invalidate)
	return mux
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	item, err := s.cache.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream failure"})
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) invalidate(w http.ResponseWriter, r *http.Request) {
	s.cache.Invalidate(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(body); err != nil {
		log.Printf("write response: %v", err)
	}
}
