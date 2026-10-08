// Package service exposes items from a slow upstream through a loading cache.
package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"benchtask/cache"
)

// ErrNotFound is returned by an Upstream when the item does not exist.
var ErrNotFound = errors.New("item not found")

// Item is the upstream resource.
type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Upstream fetches items from the slow backing service.
type Upstream interface {
	GetItem(ctx context.Context, id string) (Item, error)
}

// HTTPUpstream is an Upstream that calls GET {BaseURL}/items/{id}.
type HTTPUpstream struct {
	BaseURL string
	Client  *http.Client
}

// GetItem implements Upstream.
func (u *HTTPUpstream) GetItem(ctx context.Context, id string) (Item, error) {
	client := u.Client
	if client == nil {
		client = http.DefaultClient
	}
	endpoint := strings.TrimRight(u.BaseURL, "/") + "/items/" + url.PathEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Item{}, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return Item{}, fmt.Errorf("upstream request: %w", err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return Item{}, ErrNotFound
	case resp.StatusCode != http.StatusOK:
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return Item{}, fmt.Errorf("upstream status %d", resp.StatusCode)
	}
	var item Item
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&item); err != nil {
		return Item{}, fmt.Errorf("decode upstream response: %w", err)
	}
	return item, nil
}

// Server serves cached items over HTTP.
type Server struct {
	cache *cache.Cache[string, Item]
	mux   *http.ServeMux
	log   *slog.Logger
}

// New builds a Server. Only ErrNotFound is negatively cached unless
// opts.CacheError is set; other upstream failures are retried on the next
// request.
func New(up Upstream, opts cache.Options, log *slog.Logger) (*Server, error) {
	if up == nil {
		return nil, errors.New("service: upstream is nil")
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	if opts.CacheError == nil {
		opts.CacheError = func(err error) bool { return errors.Is(err, ErrNotFound) }
	}
	c, err := cache.New(func(ctx context.Context, id string) (Item, error) {
		return up.GetItem(ctx, id)
	}, opts)
	if err != nil {
		return nil, err
	}
	s := &Server{cache: c, mux: http.NewServeMux(), log: log}
	s.mux.HandleFunc("GET /items/{id}", s.handleGet)
	s.mux.HandleFunc("DELETE /items/{id}/cache", s.handleInvalidate)
	return s, nil
}

// Cache exposes the underlying cache (for metrics).
func (s *Server) Cache() *cache.Cache[string, Item] { return s.cache }

// ServeHTTP implements http.Handler.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	item, err := s.cache.Get(r.Context(), id)
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, item)
	case errors.Is(err, ErrNotFound):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case r.Context().Err() != nil:
		// Client went away; nothing useful to write.
		return
	default:
		s.log.Warn("upstream failure", "id", id, "err", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream failure"})
	}
}

func (s *Server) handleInvalidate(w http.ResponseWriter, r *http.Request) {
	s.cache.Invalidate(r.PathValue("id"))
	w.WriteHeader(http.StatusNoContent)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
