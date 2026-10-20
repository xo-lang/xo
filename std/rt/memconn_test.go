package rt

import (
	"strings"
	"testing"
)

func memTask() *Task {
	return &Task{node: newNode(nil, nil), th: newRootThread("test"), held: new(heldSet)}
}

// TestMemConn checks FakeNet's raw connections (decision 0292): bytes in
// order both ways, a write larger than the buffer, end of stream after
// close, writes to a closed peer, and a listener's backlog and close.
func TestMemConn(t *testing.T) {
	q := &memQueue{changed: newEvent()}
	client, server := newMemPair("db.test:5432", 49152)
	if !q.push(server) {
		t.Fatal("push on an open queue")
	}
	srv := q.accept(memTask())
	if !srv.Ok {
		t.Fatalf("accept: %v", srv.E)
	}
	s := srv.V.c.(*memEnd)
	if s.LocalAddr().String() != "db.test:5432" || client.RemoteAddr().String() != "db.test:5432" {
		t.Fatalf("addresses %v %v", s.LocalAddr(), client.RemoteAddr())
	}
	// more than the 64 KiB buffer: the writer waits for the reader
	big := strings.Repeat("x", memPipeSize*3+5)
	done := make(chan Result[Unit, NetErr])
	go func() { done <- client.write(memTask(), Bytes(big)) }()
	var got strings.Builder
	for got.Len() < len(big) {
		r := s.read(memTask(), 1<<20)
		if !r.Ok || !r.V.Some {
			t.Fatalf("read after %d bytes: %+v", got.Len(), r)
		}
		got.WriteString(string(r.V.V))
	}
	if w := <-done; !w.Ok || got.String() != big {
		t.Fatalf("write %+v, %d bytes read", w, got.Len())
	}
	// the other way, then close: the bytes, then the end of the stream
	if w := s.write(memTask(), Bytes("pong")); !w.Ok {
		t.Fatalf("server write %+v", w)
	}
	s.close()
	if r := client.read(memTask(), 2); !r.Ok || !r.V.Some || string(r.V.V) != "po" {
		t.Fatalf("first read %+v", r)
	}
	if r := client.read(memTask(), 10); !r.Ok || !r.V.Some || string(r.V.V) != "ng" {
		t.Fatalf("second read %+v", r)
	}
	if r := client.read(memTask(), 10); !r.Ok || r.V.Some {
		t.Fatalf("end of stream %+v", r)
	}
	if w := client.write(memTask(), Bytes("late")); w.Ok || w.E.Tag != 4 {
		t.Fatalf("write to a closed peer %+v", w)
	}
	// a closed listener: a waiting accept fails, the backlog is reset,
	// and a push is refused
	c2, s2 := newMemPair("db.test:5432", 49153)
	q.push(s2)
	q.close()
	if r := c2.read(memTask(), 1); !r.Ok || r.V.Some {
		t.Fatalf("read of a reset backlog connection %+v", r)
	}
	if q.push(s2) {
		t.Fatal("push on a closed queue")
	}
	if a := q.accept(memTask()); a.Ok {
		t.Fatal("accept on a closed queue")
	}
}
