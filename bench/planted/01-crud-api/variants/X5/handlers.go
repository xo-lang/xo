package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxBodyBytes = 1 << 20

// NewHandler returns the HTTP handler for the users API.
func NewHandler(s *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /users", func(w http.ResponseWriter, r *http.Request) { handleCreate(s, w, r) })
	mux.HandleFunc("GET /users", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, http.StatusOK, s.List()) })
	mux.HandleFunc("GET /users/{id}", func(w http.ResponseWriter, r *http.Request) { handleGet(s, w, r) })
	mux.HandleFunc("PATCH /users/{id}", func(w http.ResponseWriter, r *http.Request) { handlePatch(s, w, r) })
	mux.HandleFunc("DELETE /users/{id}", func(w http.ResponseWriter, r *http.Request) { handleDelete(s, w, r) })
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
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

// readFields decodes a JSON object body into raw fields, rejecting unknown keys.
func readFields(r *http.Request) (map[string]json.RawMessage, error) {
	type readResult struct {
		body []byte
		err  error
	}
	done := make(chan readResult)
	go func() {
		b, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, maxBodyBytes))
		done <- readResult{b, err}
	}()
	var body []byte
	select {
	case res := <-done:
		if res.err != nil {
			return nil, errors.New("could not read body")
		}
		body = res.body
	case <-r.Context().Done():
		return nil, errors.New("request canceled")
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || body[0] != '{' {
		return nil, errors.New("body must be a JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, errors.New("invalid JSON body")
	}
	for k := range fields {
		switch k {
		case "name", "email", "nickname":
		default:
			return nil, fmt.Errorf("unknown field %q", k)
		}
	}
	return fields, nil
}

func isNull(raw json.RawMessage) bool {
	return string(bytes.TrimSpace(raw)) == "null"
}

// stringField decodes a field that must be a JSON string.
func stringField(fields map[string]json.RawMessage, key string) (string, bool, error) {
	raw, ok := fields[key]
	if !ok {
		return "", false, nil
	}
	var s string
	if isNull(raw) || json.Unmarshal(raw, &s) != nil {
		return "", true, fmt.Errorf("%s must be a string", key)
	}
	return s, true, nil
}

// nicknameField decodes the optional nickname. A JSON null means "no nickname".
func nicknameField(fields map[string]json.RawMessage) (nick *string, present bool, err error) {
	raw, ok := fields["nickname"]
	if !ok {
		return nil, false, nil
	}
	if isNull(raw) {
		return nil, true, nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, true, errors.New("nickname must be a string or null")
	}
	return &s, true, nil
}

func parseID(r *http.Request) (int, bool) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil || id < 1 {
		return 0, false
	}
	return id, true
}

func handleCreate(s *Store, w http.ResponseWriter, r *http.Request) {
	fields, err := readFields(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	name, ok, err := stringField(fields, "name")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if err := validateName(name); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	email, ok, err := stringField(fields, "email")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusBadRequest, "email is required")
		return
	}
	if err := validateEmail(email); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	nick, _, err := nicknameField(fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	u, err := s.Create(name, email, nick)
	if errors.Is(err, errDuplicateEmail) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func handleGet(s *Store, w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound.Error())
		return
	}
	u, err := s.Get(id)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func handlePatch(s *Store, w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound.Error())
		return
	}
	fields, err := readFields(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var p Patch
	if name, present, err := stringField(fields, "name"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	} else if present {
		if err := validateName(name); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Name = &name
	}
	if email, present, err := stringField(fields, "email"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	} else if present {
		if err := validateEmail(email); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		p.Email = &email
	}
	nick, present, err := nicknameField(fields)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if present {
		if nick == nil {
			p.ClearNickname = true
		} else {
			p.Nickname = nick
		}
	}
	u, err := s.Update(id, p)
	switch {
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errDuplicateEmail):
		writeError(w, http.StatusConflict, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, "internal error")
	default:
		writeJSON(w, http.StatusOK, u)
	}
}

func handleDelete(s *Store, w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(r)
	if !ok {
		writeError(w, http.StatusNotFound, errNotFound.Error())
		return
	}
	if err := s.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
