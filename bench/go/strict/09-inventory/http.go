package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

const maxBody = 1 << 20

// NewHandler exposes the service over HTTP.
func NewHandler(s *Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID    string `json:"id"`
			Name  string `json:"name"`
			Stock *int   `json:"stock"`
		}
		if !decode(w, r, &req) {
			return
		}
		if req.Stock == nil {
			writeError(w, errors.Join(ErrInvalid, errors.New("stock is required")))
			return
		}
		v, err := s.CreateItem(req.ID, req.Name, *req.Stock)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	})
	mux.HandleFunc("GET /items/{id}", func(w http.ResponseWriter, r *http.Request) {
		v, err := s.GetItem(r.PathValue("id"))
		if err != nil {
			writeError(w, err)
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
		if !decode(w, r, &req) {
			return
		}
		key := req.IdempotencyKey
		if key == "" {
			key = r.Header.Get("Idempotency-Key")
		}
		res, replayed, err := s.Reserve(req.ItemID, req.Qty, key)
		if err != nil {
			writeError(w, err)
			return
		}
		code := http.StatusCreated
		if replayed {
			code = http.StatusOK
		}
		writeJSON(w, code, res)
	})
	mux.HandleFunc("GET /reservations/{id}", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.GetReservation(r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /reservations/{id}/confirm", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.Confirm(r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	mux.HandleFunc("POST /reservations/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		res, err := s.Cancel(r.PathValue("id"))
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, res)
	})
	return mux
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBody))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body: " + err.Error()})
		return false
	}
	return true
}

func statusFor(err error) int {
	switch {
	case errors.Is(err, ErrInvalid):
		return http.StatusBadRequest
	case errors.Is(err, ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, ErrExpired):
		return http.StatusGone
	case errors.Is(err, ErrInsufficientStock),
		errors.Is(err, ErrAlreadyConfirmed),
		errors.Is(err, ErrCancelled),
		errors.Is(err, ErrConflict):
		return http.StatusConflict
	default:
		return http.StatusInternalServerError
	}
}

func writeError(w http.ResponseWriter, err error) {
	writeJSON(w, statusFor(err), map[string]string{"error": err.Error()})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("write response: %v", err)
	}
}
