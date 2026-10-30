package rt

// Go_Recv is `go.Recv[T]` (decision 0421): a Go channel Xo receives from,
// each element converted to T by the binding. The binding makes one with
// GoRecvOf; a nil Go channel is one that is closed (a receive on Go's nil
// channel would wait forever).
type Go_Recv[T any] struct {
	recv func(tk *Task, wait bool) (T, recvState)
	// name is the Go type for replay and messages; rec reports whether
	// `--record` keeps the received values (T holds no handle)
	name string
	rec  bool
}

type recvState uint8

const (
	recvValue recvState = iota
	recvClosed
	recvNone
)

// GoRecvOf wraps a Go channel; conv converts an element to its Xo value.
func GoRecvOf[E, T any](ch <-chan E, name string, rec bool, conv func(E) T) Go_Recv[T] {
	return Go_Recv[T]{name: name, rec: rec, recv: func(tk *Task, wait bool) (T, recvState) {
		var zero T
		if ch == nil {
			return zero, recvClosed
		}
		if !wait {
			select {
			case e, ok := <-ch:
				if !ok {
					return zero, recvClosed
				}
				return conv(e), recvValue
			default:
				return zero, recvNone
			}
		}
		// a cancellation point (core 7.2): the task's cancellation, a
		// task.with_timeout deadline included, ends the wait, except in
		// a shielded defer
		var done <-chan struct{}
		if tk != nil && tk.shield == 0 && tk.node != nil {
			done = tk.node.doneCh()
		}
		e, ok, canceled := goRecvWait(tk, ch, done)
		if canceled {
			panic(cancelUnwind{})
		}
		if !ok {
			return zero, recvClosed
		}
		return conv(e), recvValue
	}}
}

// goRecvWait waits on ch inside a Go call that lends the task's turn under
// the deterministic scheduler (the sender may be an Xo callback that needs
// the turn), so the return is a scheduling point like any Go call's.
func goRecvWait[E any](tk *Task, ch <-chan E, done <-chan struct{}) (e E, ok, canceled bool) {
	if tk != nil && tk.th != nil {
		goEnterLend(tk.th)
		defer goExit(tk.th)
	}
	select {
	case e, ok = <-ch:
		return e, ok, false
	case <-done:
		return e, false, true
	}
}

// M_recv is `recv()`: the next value, None once the channel is closed and
// empty.
func (r Go_Recv[T]) M_recv(tk *Task) Option[T] {
	real := func() Option[T] {
		v, st := r.recv(tk, true)
		return Option[T]{Some: st == recvValue, V: v}
	}
	if r.rec && GoFxOn() {
		return GoFxValue(tk, "go.Recv["+r.name+"].recv", nil, real)
	}
	return real()
}

// M_try_recv is `try_recv()`: None when no value is ready, Some(None) once
// the channel is closed.
func (r Go_Recv[T]) M_try_recv(tk *Task) Option[Option[T]] {
	real := func() Option[Option[T]] {
		v, st := r.recv(tk, false)
		if st == recvNone {
			return Option[Option[T]]{}
		}
		return Option[Option[T]]{Some: true, V: Option[T]{Some: st == recvValue, V: v}}
	}
	if r.rec && GoFxOn() {
		return GoFxValue(tk, "go.Recv["+r.name+"].try_recv", nil, real)
	}
	return real()
}

// M_each is `each(f)`: f with each value until the channel is closed.
func (r Go_Recv[T]) M_each(tk *Task, f func(*Task, T) Result[Unit, Never]) {
	for {
		o := r.M_recv(tk)
		if !o.Some {
			return
		}
		f(tk, o.V)
	}
}
