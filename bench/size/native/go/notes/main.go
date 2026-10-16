// notes: examples/notes-service in Go, for the binary size comparison
// (bench/results/binary-size-native-2026-10-16.md): users and expiring
// notes behind a bearer token auth middleware, tokens stored as SHA-256
// digests and compared in constant time, Argon2id passwords and POST
// /login, a background purge, and two phase shutdown. Go's std has no
// SQL engine, so the store is maps under a mutex (the Xo service uses
// std/sql's in memory driver); Argon2id is golang.org/x/crypto's.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/crypto/argon2"
)

const purgeEvery = time.Minute

type user struct {
	id       int
	name     string
	digest   string
	password string // PHC Argon2id, or ""
}

type note struct {
	ID      int       `json:"id"`
	Owner   int       `json:"owner"`
	Body    string    `json:"body"`
	Created time.Time `json:"created"`
	Expires time.Time `json:"expires"`
}

type store struct {
	mu     sync.Mutex
	users  []*user
	notes  map[int]note
	nextID int
}

func digest(token string) string {
	d := sha256.Sum256([]byte(token))
	return hex.EncodeToString(d[:])
}

func newToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	k := argon2.IDKey([]byte(pw), salt, 2, 19456, 1, 32)
	enc := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=19$m=19456,t=2,p=1$%s$%s", enc.EncodeToString(salt), enc.EncodeToString(k))
}

func verifyPassword(pw, phc string) bool {
	f := strings.Split(phc, "$")
	if len(f) != 6 || f[1] != "argon2id" {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(f[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	enc := base64.RawStdEncoding
	salt, err1 := enc.DecodeString(f[4])
	want, err2 := enc.DecodeString(f[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (s *store) userForToken(token string) *user {
	d := digest(token)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, u := range s.users {
		if subtle.ConstantTimeCompare([]byte(u.digest), []byte(d)) == 1 {
			return u
		}
	}
	return nil
}

type ctxKey struct{}

type apiErr struct {
	code int
	msg  string
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, e apiErr) { writeJSON(w, e.code, map[string]string{"error": e.msg}) }

func (s *store) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		t := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		if !strings.HasPrefix(h, "Bearer ") || t == "" {
			fail(w, apiErr{401, "missing or invalid bearer token"})
			return
		}
		u := s.userForToken(t)
		if u == nil {
			fail(w, apiErr{401, "missing or invalid bearer token"})
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (s *store) routes() http.Handler {
	notes := http.NewServeMux()
	notes.HandleFunc("POST /notes", func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(ctxKey{}).(*user)
		n := struct {
			Body    string `json:"body"`
			TTLSecs int    `json:"ttl_secs"`
		}{TTLSecs: 3600}
		if err := json.NewDecoder(r.Body).Decode(&n); err != nil {
			fail(w, apiErr{400, err.Error()})
			return
		}
		if n.Body == "" || n.TTLSecs <= 0 {
			fail(w, apiErr{400, "body must not be empty and ttl_secs positive"})
			return
		}
		now := time.Now()
		s.mu.Lock()
		s.nextID++
		nt := note{ID: s.nextID, Owner: u.id, Body: n.Body, Created: now, Expires: now.Add(time.Duration(n.TTLSecs) * time.Second)}
		s.notes[nt.ID] = nt
		s.mu.Unlock()
		writeJSON(w, 201, nt)
	})
	notes.HandleFunc("GET /notes", func(w http.ResponseWriter, r *http.Request) {
		u := r.Context().Value(ctxKey{}).(*user)
		s.mu.Lock()
		out := []note{}
		for id := 1; id <= s.nextID; id++ {
			if n, ok := s.notes[id]; ok && n.Owner == u.id {
				out = append(out, n)
			}
		}
		s.mu.Unlock()
		writeJSON(w, 200, out)
	})
	one := func(w http.ResponseWriter, r *http.Request, del bool) {
		u := r.Context().Value(ctxKey{}).(*user)
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			fail(w, apiErr{400, "bad id"})
			return
		}
		s.mu.Lock()
		n, ok := s.notes[id]
		if ok && n.Owner == u.id && del {
			delete(s.notes, id)
		}
		s.mu.Unlock()
		if !ok || n.Owner != u.id {
			fail(w, apiErr{404, fmt.Sprintf("note %d not found", id)})
		} else if del {
			w.WriteHeader(204)
		} else {
			writeJSON(w, 200, n)
		}
	}
	notes.HandleFunc("GET /notes/{id}", func(w http.ResponseWriter, r *http.Request) { one(w, r, false) })
	notes.HandleFunc("DELETE /notes/{id}", func(w http.ResponseWriter, r *http.Request) { one(w, r, true) })
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("POST /login", func(w http.ResponseWriter, r *http.Request) {
		var c struct {
			Name     string `json:"name"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
			fail(w, apiErr{400, err.Error()})
			return
		}
		s.mu.Lock()
		var found *user
		for _, u := range s.users {
			if u.name == c.Name && u.password != "" {
				found = u
			}
		}
		s.mu.Unlock()
		if found == nil || !verifyPassword(c.Password, found.password) {
			fail(w, apiErr{401, "missing or invalid bearer token"})
			return
		}
		t := newToken()
		s.mu.Lock()
		found.digest = digest(t)
		s.mu.Unlock()
		writeJSON(w, 200, map[string]string{"token": t})
	})
	mux.Handle("/notes", s.auth(notes))
	mux.Handle("/notes/", s.auth(notes))
	return mux
}

func (s *store) purge(ctx context.Context) {
	t := time.NewTicker(purgeEvery)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-t.C:
			s.mu.Lock()
			n := 0
			for id, nt := range s.notes {
				if !nt.Expires.After(now) {
					delete(s.notes, id)
					n++
				}
			}
			s.mu.Unlock()
			if n > 0 {
				slog.Info("purged expired notes", "count", n)
			}
		}
	}
}

func main() {
	s := &store{notes: map[int]note{}}
	token := os.Getenv("NOTES_TOKEN")
	if token == "" {
		token = newToken()
		fmt.Println("admin token:", token)
	}
	admin := &user{id: 1, name: "admin", digest: digest(token)}
	if pw := os.Getenv("NOTES_PASSWORD"); pw != "" {
		admin.password = hashPassword(pw)
	}
	s.users = append(s.users, admin)
	addr := os.Getenv("NOTES_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8080"
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.Info("listening", "addr", l.Addr().String())
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go s.purge(ctx)
	srv := &http.Server{Handler: s.routes()}
	go func() {
		<-ctx.Done()
		srv.Shutdown(context.Background())
	}()
	if err := srv.Serve(l); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	slog.Info("stopped")
}
