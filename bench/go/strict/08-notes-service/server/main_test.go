package main

import (
	"context"
	"testing"
	"time"
)

func TestRunRejectsUnknownFlag(t *testing.T) {
	if err := run(context.Background(), []string{"--no-such-flag"}); err == nil {
		t.Fatal("expected an error for an unknown flag")
	}
}

func TestRunShutsDownGracefully(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, []string{"--addr", "127.0.0.1:0", "--data-dir", t.TempDir()})
	}()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil after cancellation", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatal("server did not shut down")
	}
}
