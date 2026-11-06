package main

import (
	"math/rand"
	"sync/atomic"
	"testing"
	"time"
)

// The open loop sends the offered rate, even above one request a
// millisecond (a ticker per request sent about 1200 of 4000 a second).
func TestOpenLoopRate(t *testing.T) {
	var n int64
	d := &driver{op: func(*rand.Rand) { atomic.AddInt64(&n, 1) }}
	stop := d.open(4000)
	time.Sleep(time.Second)
	stop()
	got := atomic.LoadInt64(&n)
	t.Logf("4000/s offered for 1 s: %d sent, %d dropped", got, d.drops)
	if got < 3600 || got > 4400 {
		t.Errorf("sent %d in 1 s at 4000/s, want 3600 to 4400", got)
	}
}
