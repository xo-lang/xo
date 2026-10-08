package jobqueue

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func doRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, path, rd)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeBody[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode %q: %v", rec.Body.String(), err)
	}
	return v
}

func TestHTTPEnqueueAndIdempotency(t *testing.T) {
	q := New(Config{Clock: newFakeClock()})
	h := NewHTTPHandler(q, t.Logf)
	payload := base64.StdEncoding.EncodeToString([]byte("hello"))
	body := `{"payload":"` + payload + `","priority":3,"idempotency_key":"abc"}`

	rec := doRequest(t, h, http.MethodPost, "/jobs", body)
	if rec.Code != http.StatusCreated {
		t.Fatalf("first POST status = %d, body %s", rec.Code, rec.Body.String())
	}
	first := decodeBody[idResponse](t, rec)
	if first.ID == "" {
		t.Fatal("empty id")
	}

	rec = doRequest(t, h, http.MethodPost, "/jobs", body)
	if rec.Code != http.StatusOK {
		t.Fatalf("duplicate POST status = %d", rec.Code)
	}
	if dup := decodeBody[idResponse](t, rec); dup.ID != first.ID {
		t.Fatalf("duplicate id = %s, want %s", dup.ID, first.ID)
	}

	rec = doRequest(t, h, http.MethodGet, "/jobs/"+first.ID, "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	jr := decodeBody[jobResponse](t, rec)
	if jr.State != "queued" || jr.Attempts != 0 || jr.Priority != 3 || jr.Payload != payload || jr.LastError != "" {
		t.Fatalf("job = %+v", jr)
	}

	rec = doRequest(t, h, http.MethodGet, "/stats", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d", rec.Code)
	}
	if s := decodeBody[Stats](t, rec); s != (Stats{Queued: 1}) {
		t.Fatalf("stats = %+v", s)
	}
}

func TestHTTPBadRequests(t *testing.T) {
	h := NewHTTPHandler(New(Config{}), t.Logf)
	cases := []string{
		`not json`,
		`{"payload":"!!!not base64"}`,
		`{"priority":1}`,
		`{"payload":"","unknown":1}`,
	}
	for _, body := range cases {
		if rec := doRequest(t, h, http.MethodPost, "/jobs", body); rec.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400", body, rec.Code)
		}
	}
	if rec := doRequest(t, h, http.MethodGet, "/jobs/missing", ""); rec.Code != http.StatusNotFound {
		t.Errorf("missing job status = %d", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodGet, "/jobs", ""); rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /jobs status = %d, want 405", rec.Code)
	}
}

func TestHTTPRetryDead(t *testing.T) {
	clk := newFakeClock()
	q := New(Config{Clock: clk, MaxAttempts: 1})
	h := NewHTTPHandler(q, t.Logf)
	id, _ := q.Enqueue([]byte("x"), 0, "")

	if rec := doRequest(t, h, http.MethodPost, "/dead/"+id+"/retry", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("retry of queued job status = %d, want 404", rec.Code)
	}
	if rec := doRequest(t, h, http.MethodPost, "/dead/nope/retry", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("retry of missing job status = %d, want 404", rec.Code)
	}

	l := mustLease(t, q, "w", time.Minute)
	if err := q.Nack(id, l.Token, "bad input"); err != nil {
		t.Fatal(err)
	}
	rec := doRequest(t, h, http.MethodGet, "/jobs/"+id, "")
	jr := decodeBody[jobResponse](t, rec)
	if jr.State != "dead" || jr.Attempts != 1 || jr.LastError != "bad input" {
		t.Fatalf("dead job = %+v", jr)
	}

	rec = doRequest(t, h, http.MethodPost, "/dead/"+id+"/retry", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("retry status = %d, body %s", rec.Code, rec.Body.String())
	}
	if jr := decodeBody[jobResponse](t, rec); jr.State != "queued" || jr.Attempts != 0 {
		t.Fatalf("retried job = %+v", jr)
	}
}

func TestHTTPOverLoopbackServer(t *testing.T) {
	srv := httptest.NewServer(NewHTTPHandler(New(Config{}), t.Logf))
	defer srv.Close()
	resp, err := srv.Client().Post(srv.URL+"/jobs", "application/json", strings.NewReader(`{"payload":"aGk="}`))
	if err != nil {
		t.Fatal(err)
	}
	if resp == nil {
		t.Fatal("nil response with nil error")
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			t.Errorf("close body: %v", err)
		}
	}()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got idResponse
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.ID == "" {
		t.Fatal("empty id")
	}
}
