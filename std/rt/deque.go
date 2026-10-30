package rt

import (
	"strconv"
	"strings"
)

// Deque_Deque is `deque.Deque[T]` (std.md 47): a ring of slots in an Xo
// List, so a copy shares the ring until one of them writes it (List's
// copy on write, value semantics as core 5.1). A slot is None when free,
// so a popped element is released at once. The Go backend keeps this
// hand written version: the Go generator does not route the generic and
// var methods of a module compiled from std/native (go_impl).
type Deque_Deque[T any] struct {
	ring List[Option[T]]
	head int64
	n    int64
}

// XoMark implements marker: the ring is a list.
func (d *Deque_Deque[T]) XoMark() { d.ring.XoMark() }

// XoDebug implements Debug.
func (d Deque_Deque[T]) XoDebug(b *strings.Builder) {
	b.WriteString("Deque(")
	b.WriteString(strconv.FormatInt(d.n, 10))
	b.WriteString(")")
}

// Deque_new is `deque.new[T](cap)`.
func Deque_new[T any](tk *Task, cap int64) Deque_Deque[T] {
	var ring List[Option[T]]
	for i := int64(0); i < cap; i++ {
		List_push(tk, &ring, None[T]())
	}
	return Deque_Deque[T]{ring: ring}
}

// grow copies the ring in order from the front, twice as large (at least
// 8 slots).
func (d *Deque_Deque[T]) grow(tk *Task) {
	c := int64(len(d.ring.s))
	var nr List[Option[T]]
	for i := int64(0); i < d.n; i++ {
		List_push(tk, &nr, Own(d.ring.s[(d.head+i)%c]))
	}
	size := c * 2
	if c < 4 {
		size = 8
	}
	for int64(len(nr.s)) < size {
		List_push(tk, &nr, None[T]())
	}
	d.ring = nr
	d.head = 0
}

// M_push_back is `d.push_back(x)`.
func (d *Deque_Deque[T]) M_push_back(tk *Task, x T) {
	if d.n == int64(len(d.ring.s)) {
		d.grow(tk)
	}
	c := int64(len(d.ring.s))
	List_set(tk, &d.ring, (d.head+d.n)%c, Some(x))
	d.n++
}

// M_push_front is `d.push_front(x)`.
func (d *Deque_Deque[T]) M_push_front(tk *Task, x T) {
	if d.n == int64(len(d.ring.s)) {
		d.grow(tk)
	}
	c := int64(len(d.ring.s))
	d.head = (d.head + c - 1) % c
	List_set(tk, &d.ring, d.head, Some(x))
	d.n++
}

// M_pop_front is `d.pop_front()`.
func (d *Deque_Deque[T]) M_pop_front(tk *Task) Option[T] {
	if d.n == 0 {
		return None[T]()
	}
	c := int64(len(d.ring.s))
	v := Own(d.ring.s[d.head])
	List_set(tk, &d.ring, d.head, None[T]())
	d.head = (d.head + 1) % c
	d.n--
	return v
}

// M_pop_back is `d.pop_back()`.
func (d *Deque_Deque[T]) M_pop_back(tk *Task) Option[T] {
	if d.n == 0 {
		return None[T]()
	}
	c := int64(len(d.ring.s))
	i := (d.head + d.n - 1) % c
	v := Own(d.ring.s[i])
	List_set(tk, &d.ring, i, None[T]())
	d.n--
	return v
}

// M_get is `d.get(i)`: None out of range.
func (d Deque_Deque[T]) M_get(tk *Task, i int64) Option[T] {
	if i < 0 || i >= d.n {
		return None[T]()
	}
	return Own(d.ring.s[(d.head+i)%int64(len(d.ring.s))])
}

// M_front is `d.front()`.
func (d Deque_Deque[T]) M_front(tk *Task) Option[T] { return d.M_get(tk, 0) }

// M_back is `d.back()`.
func (d Deque_Deque[T]) M_back(tk *Task) Option[T] { return d.M_get(tk, d.n-1) }

// M_len is `d.len()`.
func (d Deque_Deque[T]) M_len(*Task) int64 { return d.n }

// M_is_empty is `d.is_empty()`.
func (d Deque_Deque[T]) M_is_empty(*Task) bool { return d.n == 0 }

// M_to_list is `d.to_list()`: the elements front to back, a new list.
func (d Deque_Deque[T]) M_to_list(tk *Task) List[T] {
	var out List[T]
	c := int64(len(d.ring.s))
	for i := int64(0); i < d.n; i++ {
		List_push(tk, &out, Own(d.ring.s[(d.head+i)%c].V))
	}
	return out
}
