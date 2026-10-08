package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunPrintsOneLinePerURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/missing" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	input := strings.Join([]string{
		srv.URL + "/ok",
		"",
		srv.URL + "/missing",
		"://bad-url",
	}, "\n")

	var out bytes.Buffer
	if err := run(context.Background(), strings.NewReader(input), &out, srv.Client(), 8, 10*time.Second); err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4:\n%s", len(lines), out.String())
	}
	if lines[0] != srv.URL+"/ok 200" {
		t.Errorf("line 0 = %q", lines[0])
	}
	if lines[1] != srv.URL+"/missing 404" {
		t.Errorf("line 1 = %q", lines[1])
	}
	if !strings.HasPrefix(lines[2], "://bad-url error: ") {
		t.Errorf("line 2 = %q", lines[2])
	}
	if !strings.HasPrefix(lines[3], "summary: 3 urls, 2 ok, 1 failed") {
		t.Errorf("summary = %q", lines[3])
	}
}
