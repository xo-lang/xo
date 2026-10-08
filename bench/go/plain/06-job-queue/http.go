package jobqueue

import (
	"encoding/json"
	"errors"
	"net/http"
)

const maxBodyBytes = 1 << 20

type enqueueRequest struct {
	Payload        []byte `json:"payload"` // base64 in JSON
	Priority       int    `json:"priority"`
	IdempotencyKey string `json:"idempotency_key"`
}

type jobResponse struct {
	ID        string `json:"id"`
	State     State  `json:"state"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	Priority  int    `json:"priority"`
}

// NewHandler returns an http.Handler exposing the queue.
func NewHandler(q *Queue) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /jobs", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		var req enqueueRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
			return
		}
		id, dup := q.Enqueue(req.Payload, req.Priority, req.IdempotencyKey)
		status := http.StatusCreated
		if dup {
			status = http.StatusOK
		}
		writeJSON(w, status, map[string]string{"id": id})
	})

	mux.HandleFunc("GET /jobs/{id}", func(w http.ResponseWriter, r *http.Request) {
		j, ok := q.Get(r.PathValue("id"))
		if !ok {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeJSON(w, http.StatusOK, toResponse(j))
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, q.Stats())
	})

	mux.HandleFunc("POST /dead/{id}/retry", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if err := q.RetryDead(id); err != nil {
			if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotDead) {
				writeError(w, http.StatusNotFound, "no dead job with that id")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		j, _ := q.Get(id)
		writeJSON(w, http.StatusOK, toResponse(j))
	})

	return mux
}

func toResponse(j Job) jobResponse {
	return jobResponse{ID: j.ID, State: j.State, Attempts: j.Attempts, LastError: j.LastError, Priority: j.Priority}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
