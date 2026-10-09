package rt

import "strings"

// Prelude methods of Option and Result (std.md 2.5).

// Option_is_some is `o.is_some()`.
func Option_is_some[T any](_ *Task, o Option[T]) bool { return o.Some }

// Option_is_none is `o.is_none()`.
func Option_is_none[T any](_ *Task, o Option[T]) bool { return !o.Some }

// Option_or_fault is `o.or_fault(msg)`.
func Option_or_fault[T any](_ *Task, o Option[T], msg string) T {
	if !o.Some {
		orFault(msg)
	}
	return o.V
}

// Option_or_err is `o.or_err(e)`.
func Option_or_err[T, E any](_ *Task, o Option[T], e E) Result[T, E] {
	if o.Some {
		return Ok[T, E](o.V)
	}
	return Err[T](e)
}

// Option_map is `o.map(f)`.
func Option_map[T, U any](tk *Task, o Option[T], f func(*Task, T) Result[U, Never]) Option[U] {
	if !o.Some {
		return Option[U]{}
	}
	return Some(f(tk, o.V).V)
}

// Option_and_then is `o.and_then(f)`.
func Option_and_then[T, U any](tk *Task, o Option[T], f func(*Task, T) Result[Option[U], Never]) Option[U] {
	if !o.Some {
		return Option[U]{}
	}
	return f(tk, o.V).V
}

// Result_is_ok is `r.is_ok()`.
func Result_is_ok[T, E any](_ *Task, r Result[T, E]) bool { return r.Ok }

// Result_is_err is `r.is_err()`.
func Result_is_err[T, E any](_ *Task, r Result[T, E]) bool { return !r.Ok }

// Result_ok is `r.ok()`.
func Result_ok[T, E any](_ *Task, r Result[T, E]) Option[T] {
	if r.Ok {
		return Some(r.V)
	}
	return Option[T]{}
}

// Result_err is `r.err()`.
func Result_err[T, E any](_ *Task, r Result[T, E]) Option[E] {
	if r.Ok {
		return Option[E]{}
	}
	return Some(r.E)
}

// Result_or_fault is `r.or_fault(msg)`: the message includes the error.
func Result_or_fault[T, E any](tk *Task, r Result[T, E], msg string) T {
	if !r.Ok {
		orFault(msg + ": " + Display(tk, any(r.E)))
	}
	return r.V
}

// Result_ignore_err is `r.ignore_err(reason)` (core 2.6): the success value or
// None; the error and the reason are logged at debug level.
func Result_ignore_err[T, E any](tk *Task, r Result[T, E], reason string) Option[T] {
	if r.Ok {
		return Some(r.V)
	}
	logRecord(tk, levelDebug, "error ignored: "+reason, []logField{{k: "error", v: Display(tk, any(r.E))}})
	return Option[T]{}
}

// Result_map is `r.map(f)`.
func Result_map[T, E, U any](tk *Task, r Result[T, E], f func(*Task, T) Result[U, Never]) Result[U, E] {
	if !r.Ok {
		return ErrTr[U](r.E, r.Tr)
	}
	return Ok[U, E](f(tk, r.V).V)
}

// Result_map_err is `r.map_err(f)`.
func Result_map_err[T, E, F any](tk *Task, r Result[T, E], f func(*Task, E) Result[F, Never]) Result[T, F] {
	if r.Ok {
		return Ok[T, F](r.V)
	}
	return ErrTr[T](f(tk, r.E).V, r.Tr)
}

// The Option and Result methods for Eq, Hash, Ord, and Debug.

// XoEq implements Eq.
func (o Option[T]) XoEq(x any) bool {
	p, ok := x.(Option[T])
	return ok && o.Some == p.Some && (!o.Some || Eq(o.V, p.V))
}

// XoHash implements Hash.
func (o Option[T]) XoHash() uint64 {
	if !o.Some {
		return 0x6e6f6e65
	}
	return mix(0x736f6d65, Hash(o.V))
}

// XoCmp orders None before every Some.
func (o Option[T]) XoCmp(x any) int {
	p := x.(Option[T])
	switch {
	case !o.Some && !p.Some:
		return 0
	case !o.Some:
		return -1
	case !p.Some:
		return 1
	}
	return Cmp(o.V, p.V)
}

// XoDebug implements Debug.
func (o Option[T]) XoDebug(b *strings.Builder) {
	if !o.Some {
		b.WriteString("None")
		return
	}
	b.WriteString("Some(")
	debugTo(b, o.V)
	b.WriteByte(')')
}

// XoEq implements Eq; error frames are not part of the value.
func (r Result[T, E]) XoEq(x any) bool {
	p, ok := x.(Result[T, E])
	if !ok || r.Ok != p.Ok {
		return false
	}
	if r.Ok {
		return Eq(r.V, p.V)
	}
	return Eq(r.E, p.E)
}

// XoHash implements Hash.
func (r Result[T, E]) XoHash() uint64 {
	if r.Ok {
		return mix(0x6f6b, Hash(r.V))
	}
	return mix(0x657272, Hash(r.E))
}

// XoDebug implements Debug.
func (r Result[T, E]) XoDebug(b *strings.Builder) {
	if r.Ok {
		b.WriteString("Ok(")
		debugTo(b, r.V)
	} else {
		b.WriteString("Err(")
		debugTo(b, r.E)
	}
	b.WriteByte(')')
}
