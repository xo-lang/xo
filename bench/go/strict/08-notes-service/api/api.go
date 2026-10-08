// Package api exposes the notes store over HTTP.
package api

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
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

// Options configures an API. Zero fields take production defaults.
type Options struct {
	Now    func() time.Time
	NewID  func() string
	Logger *slog.Logger
}

// API holds the HTTP handlers.
type API struct {
	store *store.Store
	now   func() time.Time
	newID func() string
	log   *slog.Logger
}

// New returns an API backed by st.
func New(st *store.Store, opts Options) *API {
	a := &API{store: st, now: time.Now, newID: domain.NewID, log: slog.Default()}
	if opts.Now != nil {
		a.now = opts.Now
	}
	if opts.NewID != nil {
		a.newID = opts.NewID
	}
	if opts.Logger != nil {
		a.log = opts.Logger
	}
	return a
}

// Handler returns the routed HTTP handler.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /notes", a.createNote)
	mux.HandleFunc("GET /notes", a.listNotes)
	mux.HandleFunc("GET /notes/{id}", a.getNote)
	mux.HandleFunc("PUT /notes/{id}", a.updateNote)
	mux.HandleFunc("DELETE /notes/{id}", a.deleteNote)
	mux.HandleFunc("GET /export", a.export)
	return mux
}

type noteInput struct {
	Title string   `json:"title"`
	Body  string   `json:"body"`
	Tags  []string `json:"tags"`
}

type listResponse struct {
	Notes      []domain.Note `json:"notes"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

func etag(version int64) string {
	return `"` + strconv.FormatInt(version, 10) + `"`
}

func (a *API) createNote(w http.ResponseWriter, r *http.Request) {
	in, ok := a.decodeInput(w, r)
	if !ok {
		return
	}
	now := a.now().UTC()
	n := domain.Note{
		ID:      a.newID(),
		Title:   in.Title,
		Body:    in.Body,
		Tags:    in.Tags,
		Version: 1,
		Created: now,
		Updated: now,
	}
	if err := a.store.Create(n); err != nil {
		a.storeError(w, err)
		return
	}
	w.Header().Set("Location", "/notes/"+n.ID)
	a.writeNote(w, http.StatusCreated, n)
}

func (a *API) getNote(w http.ResponseWriter, r *http.Request) {
	n, err := a.store.Get(r.PathValue("id"))
	if err != nil {
		a.storeError(w, err)
		return
	}
	a.writeNote(w, http.StatusOK, n)
}

func (a *API) updateNote(w http.ResponseWriter, r *http.Request) {
	version, ok := a.ifMatch(w, r)
	if !ok {
		return
	}
	in, ok := a.decodeInput(w, r)
	if !ok {
		return
	}
	now := a.now().UTC()
	n, err := a.store.Update(r.PathValue("id"), version, func(cur domain.Note) domain.Note {
		cur.Title = in.Title
		cur.Body = in.Body
		cur.Tags = in.Tags
		cur.Updated = now
		return cur
	})
	if err != nil {
		a.storeError(w, err)
		return
	}
	a.writeNote(w, http.StatusOK, n)
}

func (a *API) deleteNote(w http.ResponseWriter, r *http.Request) {
	version, ok := a.ifMatch(w, r)
	if !ok {
		return
	}
	if err := a.store.Delete(r.PathValue("id"), version); err != nil {
		a.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) listNotes(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := defaultLimit
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			a.writeError(w, http.StatusBadRequest, "limit must be an integer from 1 to 100")
			return
		}
		limit = n
	}
	notes, err := a.store.List()
	if err != nil {
		a.storeError(w, err)
		return
	}
	page, err := domain.Search(notes, domain.Filter{Tag: q.Get("tag"), Text: q.Get("q")}, limit, q.Get("cursor"))
	if err != nil {
		if errors.Is(err, domain.ErrInvalidCursor) || errors.Is(err, domain.ErrInvalidLimit) {
			a.writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		a.storeError(w, err)
		return
	}
	resp := listResponse{Notes: page.Notes, NextCursor: page.NextCursor}
	if resp.Notes == nil {
		resp.Notes = []domain.Note{}
	}
	a.writeJSON(w, http.StatusOK, resp)
}

func (a *API) export(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	rc := http.NewResponseController(w)
	err := a.store.Each(func(n domain.Note) error {
		if err := enc.Encode(n); err != nil {
			return err
		}
		if err := rc.Flush(); err != nil && !errors.Is(err, http.ErrNotSupported) {
			return err
		}
		return nil
	})
	if err != nil {
		// Headers are already sent; the truncated stream is the signal.
		a.log.Error("export aborted", "err", err)
	}
}

// ifMatch extracts the expected version from If-Match. A missing header is
// 428, a malformed one is 400. Only a single strong entity tag is accepted.
func (a *API) ifMatch(w http.ResponseWriter, r *http.Request) (int64, bool) {
	raw := strings.TrimSpace(r.Header.Get("If-Match"))
	if raw == "" {
		a.writeError(w, http.StatusPreconditionRequired, "If-Match header with the current version is required")
		return 0, false
	}
	v := raw
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 1 {
		a.writeError(w, http.StatusBadRequest, `If-Match must be a single version entity tag such as "3"`)
		return 0, false
	}
	return n, true
}

func (a *API) decodeInput(w http.ResponseWriter, r *http.Request) (noteInput, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	var in noteInput
	if err := dec.Decode(&in); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			a.writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return noteInput{}, false
		}
		a.writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return noteInput{}, false
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		a.writeError(w, http.StatusBadRequest, "request body must be a single JSON object")
		return noteInput{}, false
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if err := domain.ValidateContent(in.Title, in.Tags); err != nil {
		a.writeError(w, http.StatusBadRequest, err.Error())
		return noteInput{}, false
	}
	return in, true
}

func (a *API) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		a.writeError(w, http.StatusNotFound, "note not found")
	case errors.Is(err, store.ErrVersionConflict):
		a.writeError(w, http.StatusPreconditionFailed, "version does not match the current note")
	default:
		a.log.Error("internal error", "err", err)
		a.writeError(w, http.StatusInternalServerError, "internal error")
	}
}

func (a *API) writeNote(w http.ResponseWriter, status int, n domain.Note) {
	w.Header().Set("ETag", etag(n.Version))
	a.writeJSON(w, status, n)
}

func (a *API) writeError(w http.ResponseWriter, status int, msg string) {
	a.writeJSON(w, status, errorResponse{Error: msg})
}

func (a *API) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		a.log.Error("write response", "err", err)
	}
}
