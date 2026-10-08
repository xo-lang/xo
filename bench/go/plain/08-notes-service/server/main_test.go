package main

import (
	"context"
	"net/http"
	"path/filepath"
	"testing"
	"time"
)

func TestRunServesAndShutsDownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	addrc := make(chan string, 1)
	done := make(chan error, 1)
	dataDir := filepath.Join(t.TempDir(), "nested", "data")
	go func() { done <- run(ctx, dataDir, "127.0.0.1:0", func(a string) { addrc <- a }) }()

	var addr string
	select {
	case addr = <-addrc:
	case err := <-done:
		t.Fatalf("run exited early: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("server did not start")
	}

	resp, err := http.Get("http://" + addr + "/notes")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /notes status %d", resp.StatusCode)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("server did not shut down")
	}
}
