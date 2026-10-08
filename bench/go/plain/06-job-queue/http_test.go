package jobqueue

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func doReq(t *testing.T, srv *httptest.Server, method, path, body string) (int, map[string]any) {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var m map[string]any
	if len(raw) > 0 && strings.HasPrefix(resp.Header.Get("Content-Type"), "application/json") {
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("%s %s: bad json %q", method, path, raw)
		}
	}
	return resp.StatusCode, m
}

func TestHTTPAPI(t *testing.T) {
	q, clk := newTestQueue(1)
	srv := httptest.NewServer(NewHandler(q))
	defer srv.Close()

	// "aGVsbG8=" is base64 for "hello".
	code, m := doReq(t, srv, "POST", "/jobs", `{"payload":"aGVsbG8=","priority":3,"idempotency_key":"k"}`)
	if code != http.StatusCreated {
		t.Fatalf("create: %d %v", code, m)
	}
	id, _ := m["id"].(string)
	if id == "" {
		t.Fatalf("no id: %v", m)
	}
	code, m = doReq(t, srv, "POST", "/jobs", `{"payload":"aGVsbG8=","priority":3,"idempotency_key":"k"}`)
	if code != http.StatusOK || m["id"] != id {
		t.Fatalf("duplicate: %d %v", code, m)
	}
	if code, _ := doReq(t, srv, "POST", "/jobs", `{"payload":"!!notbase64","priority":0}`); code != http.StatusBadRequest {
		t.Fatalf("bad base64: %d", code)
	}
	if code, _ := doReq(t, srv, "POST", "/jobs", `{`); code != http.StatusBadRequest {
		t.Fatalf("bad json: %d", code)
	}

	j, _ := q.Get(id)
	if string(j.Payload) != "hello" || j.Priority != 3 {
		t.Fatalf("stored job %+v", j)
	}

	code, m = doReq(t, srv, "GET", "/jobs/"+id, "")
	if code != http.StatusOK || m["state"] != "queued" || m["attempts"] != float64(0) || m["last_error"] != "" {
		t.Fatalf("get: %d %v", code, m)
	}
	if code, _ := doReq(t, srv, "GET", "/jobs/missing", ""); code != http.StatusNotFound {
		t.Fatalf("get missing: %d", code)
	}

	if code, _ := doReq(t, srv, "POST", "/dead/"+id+"/retry", ""); code != http.StatusNotFound {
		t.Fatalf("retry non dead: %d", code)
	}
	if code, _ := doReq(t, srv, "POST", "/dead/missing/retry", ""); code != http.StatusNotFound {
		t.Fatalf("retry missing: %d", code)
	}

	// Dead letter it (MaxAttempts is 1).
	_, tok, _ := q.Lease("w", time.Minute)
	if err := q.Nack(id, tok, "bad input"); err != nil {
		t.Fatal(err)
	}
	clk.Advance(time.Second)

	code, m = doReq(t, srv, "GET", "/stats", "")
	if code != http.StatusOK || m["dead"] != float64(1) || m["queued"] != float64(0) {
		t.Fatalf("stats: %d %v", code, m)
	}
	code, m = doReq(t, srv, "GET", "/jobs/"+id, "")
	if m["state"] != "dead" || m["last_error"] != "bad input" || m["attempts"] != float64(1) {
		t.Fatalf("get dead: %d %v", code, m)
	}
	code, m = doReq(t, srv, "POST", "/dead/"+id+"/retry", "")
	if code != http.StatusOK || m["state"] != "queued" || m["attempts"] != float64(0) {
		t.Fatalf("retry: %d %v", code, m)
	}
	code, m = doReq(t, srv, "GET", "/stats", "")
	if m["queued"] != float64(1) || m["dead"] != float64(0) {
		t.Fatalf("stats after retry: %v", m)
	}
	if code, _ := doReq(t, srv, "DELETE", "/jobs/"+id, ""); code != http.StatusMethodNotAllowed {
		t.Fatalf("delete: %d", code)
	}
}
