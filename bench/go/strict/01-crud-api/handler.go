package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxBodyBytes = 1 << 20

// NewHandler returns the HTTP handler for the users API.
func NewHandler(s *Store) http.Handler {
	a := &api{store: s}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", a.create)
	mux.HandleFunc("GET /users", a.list)
	mux.HandleFunc("GET /users/{id}", a.get)
	mux.HandleFunc("PATCH /users/{id}", a.patch)
	mux.HandleFunc("DELETE /users/{id}", a.remove)
	return mux
}

type api struct {
	store *Store
}

type errorBody struct {
	Error string `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorBody{Error: msg})
}

func validateName(name string) error {
	n := utf8.RuneCountInString(name)
	if n < 1 || n > 100 {
		return errors.New("name must be 1 to 100 characters")
	}
	return nil
}

func validateEmail(email string) error {
	if !strings.Contains(email, "@") {
		return errors.New("email must contain @")
	}
	return nil
}

// decodeStrict decodes exactly one JSON value from r into v.
func decodeStrict(r io.Reader, v any, disallowUnknown bool) error {
	dec := json.NewDecoder(r)
	if disallowUnknown {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("invalid JSON body: %w", err)
	}
	var extra json.RawMessage
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("invalid JSON body: unexpected trailing data")
	}
	return nil
}

type createRequest struct {
	Name     *string `json:"name"`
	Email    *string `json:"email"`
	Nickname *string `json:"nickname"`
}

func (a *api) create(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := decodeStrict(http.MaxBytesReader(w, r.Body, maxBodyBytes), &req, true); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Email == nil {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	name, email := *req.Name, *req.Email
	if err := validateName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateEmail(email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := a.store.Create(name, email, req.Nickname)
	if err != nil {
		a.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (a *api) list(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.store.List())
}

// parseID extracts the {id} path value. ok is false if it is not a positive integer.
func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

func (a *api) get(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	u, err := a.store.Get(id)
	if err != nil {
		a.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func isNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// parsePatch converts a JSON object into a validated Patch.
func parsePatch(body io.Reader) (Patch, error) {
	var fields map[string]json.RawMessage
	if err := decodeStrict(body, &fields, false); err != nil {
		return Patch{}, err
	}
	if fields == nil {
		return Patch{}, errors.New("body must be a JSON object")
	}
	var p Patch
	for key, raw := range fields {
		switch key {
		case "name":
			if isNull(raw) {
				return Patch{}, errors.New("name cannot be null")
			}
			var name string
			if err := json.Unmarshal(raw, &name); err != nil {
				return Patch{}, errors.New("name must be a string")
			}
			if err := validateName(name); err != nil {
				return Patch{}, err
			}
			p.Name = &name
		case "email":
			if isNull(raw) {
				return Patch{}, errors.New("email cannot be null")
			}
			var email string
			if err := json.Unmarshal(raw, &email); err != nil {
				return Patch{}, errors.New("email must be a string")
			}
			if err := validateEmail(email); err != nil {
				return Patch{}, err
			}
			p.Email = &email
		case "nickname":
			p.SetNickname = true
			if isNull(raw) {
				p.Nickname = nil
				continue
			}
			var nick string
			if err := json.Unmarshal(raw, &nick); err != nil {
				return Patch{}, errors.New("nickname must be a string or null")
			}
			p.Nickname = &nick
		default:
			return Patch{}, fmt.Errorf("unknown field %q", key)
		}
	}
	return p, nil
}

func (a *api) patch(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	p, err := parsePatch(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := a.store.Update(id, p)
	if err != nil {
		a.storeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *api) remove(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, ErrNotFound.Error())
		return
	}
	if err := a.store.Delete(id); err != nil {
		a.storeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *api) storeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, ErrDuplicateEmail):
		writeError(w, http.StatusConflict, err.Error())
	default:
		log.Printf("store error: %v", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
