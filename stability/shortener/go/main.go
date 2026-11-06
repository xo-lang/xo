// Command shortener is the Go twin of stability/shortener (decision 0520
// phase 2): the same routes, environment, journal and snapshot format, for
// the load comparison. Standard library only.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

const (
	maxURL     = 2048
	snapshotAt = 1 << 20
	codeChars  = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-"
	genLen     = 7
	logFile    = "links.log"
	snapFile   = "snapshot.json"
)

var reserved = map[string]bool{"links": true, "health": true, "debug": true}

type link struct {
	Code    string    `json:"code"`
	URL     string    `json:"url"`
	Created time.Time `json:"created"`
	Hits    int64     `json:"hits"`
}

type created struct {
	Code    string    `json:"code"`
	URL     string    `json:"url"`
	Created time.Time `json:"created"`
}

type createReq struct {
	URL  *string `json:"url"`
	Code *string `json:"code"`
}

type rec struct {
	Seq     int64            `json:"seq"`
	Op      string           `json:"op"`
	Code    string           `json:"code,omitempty"`
	URL     string           `json:"url,omitempty"`
	Created *time.Time       `json:"created,omitempty"`
	Hits    map[string]int64 `json:"hits,omitempty"`
}

type snap struct {
	Seq   int64  `json:"seq"`
	Links []link `json:"links"`
}

type entry struct {
	url     string
	created time.Time
	hits    atomic.Int64
	flushed atomic.Int64
}

type store struct {
	mu      sync.RWMutex
	links   map[string]*entry
	order   []string // insertion order, for the snapshot
	dir     string   // "" in memory
	file    *os.File
	logSize int64
	seq     int64
	token   []byte
}

type apiErr struct {
	status int
	msg    string
}

func (e *apiErr) Error() string { return e.msg }

var (
	errNotFound = &apiErr{404, "not found"}
	errConflict = &apiErr{409, "code already exists"}
	errUnauth   = &apiErr{401, "unauthorized"}
)

func badRequest(msg string) error { return &apiErr{400, msg} }

func newEntry(url string, t time.Time, hits int64) *entry {
	e := &entry{url: url, created: t}
	e.hits.Store(hits)
	e.flushed.Store(hits)
	return e
}

func (s *store) put(code string, e *entry) {
	if _, ok := s.links[code]; !ok {
		s.order = append(s.order, code)
	}
	s.links[code] = e
}

func (s *store) remove(code string) {
	delete(s.links, code)
	for i, c := range s.order {
		if c == code {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func open(dir, token string) (*store, error) {
	s := &store{links: map[string]*entry{}, dir: dir, token: []byte(token)}
	if dir == "" {
		return s, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	replayed, err := s.load()
	if err != nil {
		return nil, err
	}
	if replayed > 0 {
		if err := s.writeSnapshot(); err != nil {
			return nil, err
		}
		if err := os.WriteFile(filepath.Join(dir, logFile), nil, 0o644); err != nil {
			return nil, err
		}
	}
	f, err := os.OpenFile(filepath.Join(dir, logFile), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	s.file, s.logSize = f, st.Size()
	return s, nil
}

func (s *store) load() (int, error) {
	if b, err := os.ReadFile(filepath.Join(s.dir, snapFile)); err == nil {
		var sn snap
		if err := json.Unmarshal(b, &sn); err != nil {
			return 0, fmt.Errorf("%s: %w", snapFile, err)
		}
		for _, l := range sn.Links {
			s.put(l.Code, newEntry(l.URL, l.Created, l.Hits))
		}
		s.seq = sn.Seq
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, err
	}
	b, err := os.ReadFile(filepath.Join(s.dir, logFile))
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	} else if err != nil {
		return 0, err
	}
	lines := bytes.Split(b, []byte("\n"))
	applied := 0
	for i, line := range lines {
		if len(line) == 0 {
			continue
		}
		var r rec
		if err := json.Unmarshal(line, &r); err != nil {
			if i == len(lines)-1 {
				log.Printf("dropping a cut short journal line %d", i+1)
				break
			}
			return 0, fmt.Errorf("%s line %d: %w", logFile, i+1, err)
		}
		if r.Seq <= s.seq {
			continue
		}
		switch r.Op {
		case "create":
			t := time.Unix(0, 0).UTC()
			if r.Created != nil {
				t = *r.Created
			}
			s.put(r.Code, newEntry(r.URL, t, 0))
		case "delete":
			s.remove(r.Code)
		case "hits":
			for code, n := range r.Hits {
				if e, ok := s.links[code]; ok {
					e.hits.Store(n)
					e.flushed.Store(n)
				}
			}
		}
		s.seq = r.Seq
		applied++
	}
	return applied, nil
}

// writeRec appends one record; the caller holds mu.
func (s *store) writeRec(r rec) error {
	if s.file == nil {
		return nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := s.file.Write(b); err != nil {
		return err
	}
	s.logSize += int64(len(b))
	return nil
}

// writeSnapshot writes snapshot.json atomically; the caller holds mu.
func (s *store) writeSnapshot() error {
	sn := snap{Seq: s.seq, Links: make([]link, 0, len(s.order))}
	for _, code := range s.order {
		e := s.links[code]
		h := e.hits.Load()
		sn.Links = append(sn.Links, link{Code: code, URL: e.url, Created: e.created, Hits: h})
		e.flushed.Store(h)
	}
	b, err := json.Marshal(sn)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(s.dir, ".snapshot.*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), filepath.Join(s.dir, snapFile))
}

// maybeSnapshot compacts the journal past 1 MiB; the caller holds mu.
func (s *store) maybeSnapshot() {
	if s.file == nil || s.logSize <= snapshotAt {
		return
	}
	if err := s.writeSnapshot(); err != nil {
		log.Printf("snapshot failed: %v", err)
		return
	}
	s.file.Close()
	f, err := os.OpenFile(filepath.Join(s.dir, logFile), os.O_CREATE|os.O_TRUNC|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		log.Printf("journal reopen failed: %v", err)
		s.file = nil
		return
	}
	s.file, s.logSize = f, 0
}

func validCode(c string) bool {
	if len(c) < 4 || len(c) > 32 {
		return false
	}
	for i := 0; i < len(c); i++ {
		b := c[i]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' || b == '-') {
			return false
		}
	}
	return true
}

func validURL(u string) bool {
	if len(u) > maxURL {
		return false
	}
	for i := 0; i < len(u); i++ {
		if u[i] <= 0x20 || u[i] == 0x7f {
			return false
		}
	}
	rest, ok := strings.CutPrefix(u, "https://")
	if !ok {
		if rest, ok = strings.CutPrefix(u, "http://"); !ok {
			return false
		}
	}
	host := rest
	if i := strings.IndexAny(rest, "/?#"); i >= 0 {
		host = rest[:i]
	}
	if i := strings.LastIndex(host, "@"); i >= 0 {
		host = host[i+1:]
	}
	if strings.HasPrefix(host, "[") {
		i := strings.Index(host, "]")
		return i > 1
	}
	if h, _, ok := strings.Cut(host, ":"); ok {
		host = h
	}
	return host != ""
}

func freshCode(links map[string]*entry) (string, error) {
	for try := 0; try < 16; try++ {
		var b [genLen]byte
		for i := range b {
			n, err := rand.Int(rand.Reader, big.NewInt(64))
			if err != nil {
				return "", err
			}
			b[i] = codeChars[n.Int64()]
		}
		if _, ok := links[string(b[:])]; !ok {
			return string(b[:]), nil
		}
	}
	return "", badRequest("no free code, try again")
}

func (s *store) create(req createReq) (created, error) {
	if req.URL == nil {
		return created{}, badRequest("invalid body: at $: missing field url")
	}
	url := *req.URL
	if !validURL(url) {
		return created{}, badRequest("url must be http:// or https:// with a host, at most 2048 bytes")
	}
	if req.Code != nil {
		if !validCode(*req.Code) {
			return created{}, badRequest("code must be 4 to 32 characters of A-Z a-z 0-9 _ -")
		}
		if reserved[*req.Code] {
			return created{}, badRequest("code is reserved")
		}
	}
	now := time.Unix(time.Now().Unix(), 0).UTC()
	s.mu.Lock()
	defer s.mu.Unlock()
	var code string
	if req.Code != nil {
		code = *req.Code
		if _, ok := s.links[code]; ok {
			return created{}, errConflict
		}
	} else {
		c, err := freshCode(s.links)
		if err != nil {
			return created{}, err
		}
		code = c
	}
	if err := s.writeRec(rec{Seq: s.seq + 1, Op: "create", Code: code, URL: url, Created: &now}); err != nil {
		return created{}, err
	}
	s.seq++
	s.put(code, newEntry(url, now, 0))
	s.maybeSnapshot()
	return created{Code: code, URL: url, Created: now}, nil
}

func (s *store) delete(code string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.links[code]; !ok {
		return errNotFound
	}
	if err := s.writeRec(rec{Seq: s.seq + 1, Op: "delete", Code: code}); err != nil {
		return err
	}
	s.seq++
	s.remove(code)
	s.maybeSnapshot()
	return nil
}

func (s *store) lookup(code string) *entry {
	s.mu.RLock()
	e := s.links[code]
	s.mu.RUnlock()
	return e
}

func (s *store) flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return nil
	}
	hits := map[string]int64{}
	for code, e := range s.links {
		if h := e.hits.Load(); h != e.flushed.Load() {
			hits[code] = h
		}
	}
	if len(hits) == 0 {
		return nil
	}
	if err := s.writeRec(rec{Seq: s.seq + 1, Op: "hits", Hits: hits}); err != nil {
		return err
	}
	s.seq++
	for code, h := range hits {
		s.links[code].flushed.Store(h)
	}
	s.maybeSnapshot()
	return nil
}

func (s *store) close() {
	if err := s.flush(); err != nil {
		log.Printf("final hit flush failed: %v", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file != nil {
		s.file.Close()
		s.file = nil
	}
}

func (s *store) authorized(r *http.Request) bool {
	given, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	return ok && subtle.ConstantTimeCompare([]byte(given), s.token) == 1
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(b)
}

func writeErr(w http.ResponseWriter, err error) {
	var ae *apiErr
	if errors.As(err, &ae) {
		writeJSON(w, ae.status, map[string]string{"error": ae.msg})
		return
	}
	log.Printf("internal error: %v", err)
	writeJSON(w, 500, map[string]string{"error": "internal error"})
}

func (s *store) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("POST /links", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeErr(w, errUnauth)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			writeErr(w, badRequest("invalid body: "+err.Error()))
			return
		}
		var req createReq
		if err := json.Unmarshal(body, &req); err != nil {
			writeErr(w, badRequest("invalid body: "+err.Error()))
			return
		}
		out, err := s.create(req)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, 201, out)
	})
	mux.HandleFunc("GET /links/{code}", func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		e := s.lookup(code)
		if e == nil {
			writeErr(w, errNotFound)
			return
		}
		writeJSON(w, 200, link{Code: code, URL: e.url, Created: e.created, Hits: e.hits.Load()})
	})
	mux.HandleFunc("DELETE /links/{code}", func(w http.ResponseWriter, r *http.Request) {
		if !s.authorized(r) {
			writeErr(w, errUnauth)
			return
		}
		if err := s.delete(r.PathValue("code")); err != nil {
			writeErr(w, err)
			return
		}
		w.WriteHeader(204)
	})
	mux.HandleFunc("GET /{code}", func(w http.ResponseWriter, r *http.Request) {
		e := s.lookup(r.PathValue("code"))
		if e == nil {
			writeErr(w, errNotFound)
			return
		}
		e.hits.Add(1)
		w.Header().Set("Location", e.url)
		w.WriteHeader(302)
	})
	return mux
}

func main() {
	token := os.Getenv("SHORT_TOKEN")
	if token == "" {
		fmt.Fprintln(os.Stderr, "shortener: SHORT_TOKEN is required")
		os.Exit(2)
	}
	addr := os.Getenv("SHORT_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	s, err := open(os.Getenv("SHORT_DATA"), token)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shortener:", err)
		os.Exit(1)
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, "shortener:", err)
		os.Exit(1)
	}
	fmt.Printf("listening on %s\n", ln.Addr())
	srv := &http.Server{Handler: s.routes()}

	done := make(chan struct{})
	go func() {
		t := time.NewTicker(time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				if err := s.flush(); err != nil {
					log.Printf("hit flush failed: %v", err)
				}
			case <-done:
				return
			}
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		<-ctx.Done()
		// stop accepting, finish in flight requests for at most 5 s
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(sctx)
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Fprintln(os.Stderr, "shortener:", err)
		os.Exit(1)
	}
	// Serve returns as soon as Shutdown starts; wait for it to drain.
	<-drained
	close(done)
	s.close()
}
