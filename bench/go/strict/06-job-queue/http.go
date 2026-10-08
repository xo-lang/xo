package jobqueue

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log"
	"net/http"
)

const maxBodyBytes = 1 << 20

type api struct {
	q    *Queue
	logf func(format string, args ...any)
}

type enqueueRequest struct {
	Payload        *string `json:"payload"`
	Priority       int     `json:"priority"`
	IdempotencyKey string  `json:"idempotency_key"`
}

type idResponse struct {
	ID string `json:"id"`
}

type jobResponse struct {
	ID        string `json:"id"`
	State     string `json:"state"`
	Priority  int    `json:"priority"`
	Attempts  int    `json:"attempts"`
	LastError string `json:"last_error"`
	Payload   string `json:"payload"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// NewHTTPHandler returns the HTTP API for q. logf receives response write
// failures; nil means log.Printf.
func NewHTTPHandler(q *Queue, logf func(format string, args ...any)) http.Handler {
	if logf == nil {
		logf = log.Printf
	}
	a := &api{q: q, logf: logf}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /jobs", a.enqueue)
	mux.HandleFunc("GET /jobs/{id}", a.getJob)
	mux.HandleFunc("GET /stats", a.stats)
	mux.HandleFunc("POST /dead/{id}/retry", a.retryDead)
	return mux
}

func toResponse(j Job) jobResponse {
	return jobResponse{
		ID:        j.ID,
		State:     j.State.String(),
		Priority:  j.Priority,
		Attempts:  j.Attempts,
		LastError: j.LastError,
		Payload:   base64.StdEncoding.EncodeToString(j.Payload),
	}
}

func (a *api) enqueue(w http.ResponseWriter, r *http.Request) {
	var req enqueueRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		a.writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body: " + err.Error()})
		return
	}
	if req.Payload == nil {
		a.writeJSON(w, http.StatusBadRequest, errorResponse{Error: "payload is required"})
		return
	}
	payload, err := base64.StdEncoding.DecodeString(*req.Payload)
	if err != nil {
		a.writeJSON(w, http.StatusBadRequest, errorResponse{Error: "payload is not valid base64"})
		return
	}
	id, created := a.q.Enqueue(payload, req.Priority, req.IdempotencyKey)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	a.writeJSON(w, status, idResponse{ID: id})
}

func (a *api) getJob(w http.ResponseWriter, r *http.Request) {
	j, err := a.q.Get(r.PathValue("id"))
	if err != nil {
		a.writeErr(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, toResponse(j))
}

func (a *api) stats(w http.ResponseWriter, _ *http.Request) {
	a.writeJSON(w, http.StatusOK, a.q.Stats())
}

func (a *api) retryDead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := a.q.RetryDead(id); err != nil {
		a.writeErr(w, err)
		return
	}
	j, err := a.q.Get(id)
	if err != nil {
		a.writeErr(w, err)
		return
	}
	a.writeJSON(w, http.StatusOK, toResponse(j))
}

func (a *api) writeErr(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	if errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotDead) {
		status = http.StatusNotFound
	}
	a.writeJSON(w, status, errorResponse{Error: err.Error()})
}

func (a *api) writeJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		a.logf("jobqueue: encode response: %v", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(append(b, '\n')); err != nil {
		a.logf("jobqueue: write response: %v", err)
	}
}
