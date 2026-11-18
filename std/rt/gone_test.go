package rt

import (
	"context"
	"testing"
	"time"
)

func goneClosed(c *Chan[Unit]) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// TestRequestGone: a served request's gone channel closes when net/http
// cancels its context; a request made in Xo has one that never closes.
func TestRequestGone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	req := withGone(Http_Request{F_method: "GET"}, ctx)
	ch := req.M_gone(nil)
	if req.M_gone(nil) != ch {
		t.Fatal("gone gives another channel for the same request")
	}
	if goneClosed(ch) {
		t.Fatal("gone is closed before the context is done")
	}
	cancel()
	for end := time.Now().Add(3 * time.Second); !goneClosed(ch); {
		if time.Now().After(end) {
			t.Fatal("gone did not close after the context was cancelled")
		}
		time.Sleep(time.Millisecond)
	}
	if goneClosed(Http_Request{}.M_gone(nil)) {
		t.Fatal("a request made in Xo has a closed gone channel")
	}
}
