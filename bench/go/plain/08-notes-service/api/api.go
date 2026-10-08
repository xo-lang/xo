// Package api exposes the notes store over HTTP. It depends on domain and
// store.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"benchtask/domain"
	"benchtask/store"
)

const (
	defaultLimit = 20
	maxLimit     = 100
	maxBodyBytes = 1 << 20
)

// API serves the notes HTTP interface.
type API struct {
	store *store.Store
	now   func() time.Time
	mux   *http.ServeMux
}

// New returns an API backed by s. now supplies timestamps; nil means
// time.Now.
func New(s *store.Store, now func() time.Time) *API {
	if now == nil {
		now = time.Now
	}
	a := &API{store: s, now: now, mux: http.NewServeMux()}
	a.mux.HandleFunc("POST /notes", a.create)
	a.mux.HandleFunc("GET /notes", a.list)
	a.mux.HandleFunc("GET /notes/{id}", a.get)
	a.mux.HandleFunc("PUT /notes/{id}", a.update)
	a.mux.HandleFunc("DELETE /notes/{id}", a.remove)
	a.mux.HandleFunc("GET /export", a.export)
	return a
}

// ServeHTTP implements http.Handler.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *API) timestamp() time.Time {
	// UTC also strips the monotonic clock reading.
	return a.now().UTC()
}

type noteInput struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

type errorBody struct {
	Error    string   `json:"error"`
	Problems []string `json:"problems,omitempty"`
}

type listResponse struct {
	Notes      []domain.Note `json:"notes"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("api: write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string, problems []string) {
	writeJSON(w, status, errorBody{Error: msg, Problems: problems})
}

func etag(version int64) string {
	return strconv.Quote(strconv.FormatInt(version, 10))
}

// decodeInput reads and validates a note body. It writes the error response
// itself and returns false on failure.
func decodeInput(w http.ResponseWriter, r *http.Request) (noteInput, bool) {
	var in noteInput
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err := dec.Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), nil)
		return in, false
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid JSON body: trailing data", nil)
		return in, false
	}
	if err := domain.Validate(in.Title, in.Tags); err != nil {
		var ve *domain.ValidationError
		if errors.As(err, &ve) {
			writeError(w, http.StatusBadRequest, "validation failed", ve.Problems)
		} else {
			writeError(w, http.StatusBadRequest, err.Error(), nil)
		}
		return in, false
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	return in, true
}

// parseIfMatch extracts the expected version from If-Match. It returns a
// non-zero HTTP status when the request must be rejected: 428 when the header
// is absent, 412 when it cannot match any version (weak tags, "*", lists,
// or garbage).
func parseIfMatch(r *http.Request) (int64, int) {
	vals := r.Header.Values("If-Match")
	if len(vals) == 0 {
		return 0, http.StatusPreconditionRequired
	}
	v := strings.TrimSpace(strings.Join(vals, ","))
	if v == "" {
		return 0, http.StatusPreconditionRequired
	}
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		return 0, http.StatusPreconditionFailed
	}
	return n, 0
}

func (a *API) create(w http.ResponseWriter, r *http.Request) {
	in, ok := decodeInput(w, r)
	if !ok {
		return
	}
	id, err := domain.NewID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not generate id", nil)
		return
	}
	now := a.timestamp()
	n := domain.Note{ID: id, Title: in.Title, Body: in.Body, Tags: in.Tags, Version: 1, Created: now, Updated: now}
	if err := a.store.Create(n); err != nil {
		log.Printf("api: create: %v", err)
		writeError(w, http.StatusInternalServerError, "could not save note", nil)
		return
	}
	w.Header().Set("ETag", etag(n.Version))
	w.Header().Set("Location", "/notes/"+n.ID)
	writeJSON(w, http.StatusCreated, n)
}

func (a *API) get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.ValidID(id) {
		writeError(w, http.StatusNotFound, "note not found", nil)
		return
	}
	n, err := a.store.Get(id)
	if err != nil {
		a.storeError(w, err)
		return
	}
	w.Header().Set("ETag", etag(n.Version))
	writeJSON(w, http.StatusOK, n)
}

func (a *API) update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.ValidID(id) {
		writeError(w, http.StatusNotFound, "note not found", nil)
		return
	}
	expected, status := parseIfMatch(r)
	if status != 0 {
		writeError(w, status, http.StatusText(status), nil)
		return
	}
	in, ok := decodeInput(w, r)
	if !ok {
		return
	}
	now := a.timestamp()
	n, err := a.store.Update(id, expected, func(n *domain.Note) {
		n.Title = in.Title
		n.Body = in.Body
		n.Tags = in.Tags
		n.Updated = now
	})
	if err != nil {
		a.storeError(w, err)
		return
	}
	w.Header().Set("ETag", etag(n.Version))
	writeJSON(w, http.StatusOK, n)
}

func (a *API) remove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !domain.ValidID(id) {
		writeError(w, http.StatusNotFound, "note not found", nil)
		return
	}
	expected, status := parseIfMatch(r)
	if status != 0 {
		writeError(w, status, http.StatusText(status), nil)
		return
	}
	if err := a.store.Delete(id, expected); err != nil {
		a.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "note not found", nil)
	case errors.Is(err, store.ErrVersionConflict):
		writeError(w, http.StatusPreconditionFailed, "version does not match", nil)
	default:
		log.Printf("api: store: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error", nil)
	}
}

func (a *API) list(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := defaultLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			writeError(w, http.StatusBadRequest, "limit must be an integer from 1 to "+strconv.Itoa(maxLimit), nil)
			return
		}
		limit = n
	}
	var after *domain.Cursor
	if s := q.Get("cursor"); s != "" {
		c, err := domain.DecodeCursor(s)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cursor", nil)
			return
		}
		after = &c
	}
	filter := domain.Filter{Tag: q.Get("tag"), Query: q.Get("q")}
	page, next := domain.Page(a.store.List(), filter, limit, after)
	resp := listResponse{Notes: page}
	if next != nil {
		resp.NextCursor = next.Encode()
	}
	writeJSON(w, http.StatusOK, resp)
}

func (a *API) export(w http.ResponseWriter, r *http.Request) {
	notes := a.store.List()
	domain.SortNewestFirst(notes)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	rc := http.NewResponseController(w)
	for _, n := range notes {
		if r.Context().Err() != nil {
			return
		}
		if err := enc.Encode(n); err != nil {
			return
		}
		_ = rc.Flush()
	}
}
