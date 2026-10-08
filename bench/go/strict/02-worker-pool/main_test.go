package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRunPrintsOneLinePerURLInOrder(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			w.WriteHeader(http.StatusOK)
		case "/teapot":
			w.WriteHeader(http.StatusTeapot)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	input := strings.Join([]string{
		srv.URL + "/ok",
		"",
		"  " + srv.URL + "/missing  ",
		"://not-a-url",
		srv.URL + "/teapot",
	}, "\n")

	var out strings.Builder
	if err := run(context.Background(), strings.NewReader(input), &out, srv.Client(), 8, 10*time.Second); err != nil {
		t.Fatalf("run: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5:\n%s", len(lines), out.String())
	}
	want := []string{
		srv.URL + "/ok 200",
		srv.URL + "/missing 404",
		"://not-a-url error: ",
		srv.URL + "/teapot 418",
	}
	for i, w := range want {
		if !strings.HasPrefix(lines[i], w) {
			t.Errorf("line %d = %q, want prefix %q", i, lines[i], w)
		}
	}
	if !strings.HasPrefix(lines[4], "summary: 4 urls, 3 ok, 1 failed, elapsed ") {
		t.Errorf("summary = %q", lines[4])
	}
}

func TestRunEmptyInput(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), strings.NewReader("\n\n"), &out, http.DefaultClient, 8, time.Second); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.HasPrefix(out.String(), "summary: 0 urls, 0 ok, 0 failed") {
		t.Errorf("output = %q", out.String())
	}
}

func TestRunNilClient(t *testing.T) {
	var out strings.Builder
	if err := run(context.Background(), strings.NewReader(""), &out, nil, 8, time.Second); err == nil {
		t.Fatal("expected error for nil client")
	}
}
