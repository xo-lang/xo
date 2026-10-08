package rt

import "testing"

// TestCurG: curG is stable on one goroutine and differs between live
// goroutines (decision 0067: it tells a synchronous Go callback from one
// on another goroutine).
func TestCurG(t *testing.T) {
	g := curG()
	if g == 0 || curG() != g {
		t.Fatalf("curG = %x, then %x", g, curG())
	}
	const n = 8
	ids := make(chan uintptr, n)
	stop := make(chan struct{})
	for i := 0; i < n; i++ {
		go func() {
			ids <- curG()
			<-stop
		}()
	}
	seen := map[uintptr]bool{g: true}
	for i := 0; i < n; i++ {
		id := <-ids
		if seen[id] {
			t.Errorf("two live goroutines share curG %x", id)
		}
		seen[id] = true
	}
	close(stop)
}

// TestGoCallbackForeign: a callback called on another goroutine while its
// task is in a Go call runs as a child task with its own thread and held
// set; one called on the task's goroutine during a Go call runs as the
// task (decision 0067).
func TestGoCallbackForeign(t *testing.T) {
	tk := NewRootTask()
	tk.held = new(heldSet)
	cb := NewGoCallback(tk)
	GoEnter(tk)
	done := make(chan *Task)
	go func() {
		ctk := cb.Begin()
		defer cb.End(ctk)
		done <- ctk
	}()
	ctk := <-done
	if ctk == tk || ctk.th == tk.th || ctk.held == tk.held || ctk.node.parent != tk.node {
		t.Errorf("foreign call shares the task: %p %p", ctk, tk)
	}
	if got := cb.Begin(); got != tk {
		t.Errorf("synchronous call got another task")
	} else {
		cb.End(got)
	}
	GoExit(tk, "pkg.F")
	if n := tk.th.inGo.Load(); n != 0 {
		t.Errorf("inGo = %d after the call", n)
	}
	// after the Go call, a call on the task's goroutine is not synchronous
	// (a stored callback called later): it gets a task of its own
	late := cb.Begin()
	if late == tk {
		t.Errorf("call outside a Go call shares the task")
	}
	cb.End(late)
}
