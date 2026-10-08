package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// NewHandler returns the HTTP API for a store.
func NewHandler(s *Store) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Stock *int   `json:"stock"`
		}
		if err := decode(r, &req); err != nil || req.Stock == nil {
			writeErr(w, ErrInvalid)
			return
		}
		v, err := s.CreateItem(req.ID, req.Name, *req.Stock)
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.GetItem(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	})
	mux.HandleFunc("POST /reservations", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ItemID         string `json:"item_id"`
			Qty            int    `json:"qty"`
			IdempotencyKey string `json:"idempotency_key"`
		}
		if err := decode(r, &req); err != nil {
			writeErr(w, ErrInvalid)
			return
		}
		if req.IdempotencyKey == "" {
			req.IdempotencyKey = r.Header.Get("Idempotency-Key")
		}
		res, created, err := s.Reserve(req.ItemID, req.Qty, req.IdempotencyKey)
		if err != nil {
			writeErr(w, err)
			return
		}
		code := http.StatusOK
		if created {
			code = http.StatusCreated
		}
		writeJSON(w, code, res)
	})
	mux.HandleFunc("GET /reservations/{id}", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.GetReservation(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /reservations/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.Confirm(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /reservations/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.Cancel(r.PathValue("id"))
		if err != nil {
			writeErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return mux
}

func decode(r *http.Request, v any) error {
	if ct := r.Header.Get("Content-Type"); ct != "" && !strings.HasPrefix(ct, "application/json") {
		return ErrInvalid
	}
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	return nil
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrInvalid), errors.Is(err, ErrIdempotencyReuse):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrInsufficientStock), errors.Is(err, ErrAlreadyConfirmed),
		errors.Is(err, ErrCancelled), errors.Is(err, ErrItemExists):
		return http.StatusConflict
	case errors.Is(err, ErrExpired):
		return http.StatusGone
	}
	return http.StatusInternalServerError
}

func writeErr(w http.ResponseWriter, err error) {
	writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
