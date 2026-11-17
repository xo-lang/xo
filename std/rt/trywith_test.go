package rt

import "testing"

// TestMutexTryWith (decision 0698): try_with runs f on a free mutex and
// keeps its write; it returns None at once while another holder has the
// mutex, and inside the same task's with (no XO-F004).
func TestMutexTryWith(t *testing.T) {
	root := NewRootTask()
	s := NewScope(root)
	body := s.Body()
	m := Mutex_new[int64](body, 1)
	add := func(_ *Task, v *int64) Result[int64, Never] { *v += 10; return Result[int64, Never]{Ok: true, V: *v} }
	if r := Mutex_try_with(body, m, add); !r.Some || r.V != 11 || m.v != 11 {
		t.Fatalf("free: got %+v, value %d; want Some(11), 11", r, m.v)
	}
	Mutex_with(body, m, func(tk *Task, _ *int64) Result[Unit, Never] {
		if r := Mutex_try_with(tk, m, add); r.Some {
			t.Errorf("held by this task: got Some(%d), want None", r.V)
		}
		return unit()
	})
	held := make(chan struct{})
	release := make(chan struct{})
	Spawn(body, s, func(tk *Task) Result[Unit, Never] {
		Mutex_with_read(tk, m, readFn(func(*Task, int64) { close(held); <-release }))
		return unit()
	}, false, true)
	<-held
	if r := Mutex_try_with(body, m, add); r.Some {
		t.Errorf("shared by a reader: got Some(%d), want None", r.V)
	}
	close(release)
	if m.v != 11 {
		t.Fatalf("value %d after the refusals, want 11", m.v)
	}
}
