package rt

// Heap_new is `heap.new[T](less)`.
func Heap_new[T any](_ *Task, less func(*Task, T, T) Result[bool, Never]) Heap_Heap[T] {
	return Heap_Heap[T]{F__less: less, F__moved: func(*Task, T, int64) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) }}
}

// Heap_tracked is `heap.tracked[T](less, moved)`.
func Heap_tracked[T any](_ *Task, less func(*Task, T, T) Result[bool, Never], moved func(*Task, T, int64) Result[Unit, Never]) Heap_Heap[T] {
	return Heap_Heap[T]{F__less: less, F__moved: moved}
}

// Heap_from_list is `heap.from_list[T](items, less)`.
func Heap_from_list[T any](tk *Task, items List[T], less func(*Task, T, T) Result[bool, Never]) Heap_Heap[T] {
	h := Heap_Heap[T]{F__items: items, F__less: less, F__moved: func(*Task, T, int64) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) }}
	for i := int64(len(items.s))/2 - 1; i >= 0; i-- {
		h.down(tk, i, int64(len(h.F__items.s)))
	}
	return h
}

func (h *Heap_Heap[T]) lt(tk *Task, a, b T) bool { return h.F__less(tk, a, b).V }

func (h *Heap_Heap[T]) mv(tk *Task, x T, i int64) { h.F__moved(tk, x, i) }

func (h *Heap_Heap[T]) swap(tk *Task, i, j int64) {
	a, b := h.F__items.s[i], h.F__items.s[j]
	List_set(tk, &h.F__items, i, b)
	List_set(tk, &h.F__items, j, a)
	h.mv(tk, b, i)
	h.mv(tk, a, j)
}

func (h *Heap_Heap[T]) up(tk *Task, j int64) {
	for j > 0 {
		i := (j - 1) / 2
		if i == j || !h.lt(tk, h.F__items.s[j], h.F__items.s[i]) {
			break
		}
		h.swap(tk, i, j)
		j = i
	}
}

func (h *Heap_Heap[T]) down(tk *Task, i0, n int64) bool {
	i := i0
	for {
		j1 := 2*i + 1
		if j1 >= n {
			break
		}
		j := j1
		if j2 := j1 + 1; j2 < n && h.lt(tk, h.F__items.s[j2], h.F__items.s[j1]) {
			j = j2
		}
		if !h.lt(tk, h.F__items.s[j], h.F__items.s[i]) {
			break
		}
		h.swap(tk, i, j)
		i = j
	}
	return i > i0
}

// M_push is `h.push(x)`.
func (h *Heap_Heap[T]) M_push(tk *Task, x T) {
	List_push(tk, &h.F__items, x)
	n := int64(len(h.F__items.s)) - 1
	h.mv(tk, x, n)
	h.up(tk, n)
}

// M_pop is `h.pop()`.
func (h *Heap_Heap[T]) M_pop(tk *Task) Option[T] { return h.M_remove(tk, 0) }

// M_peek is `h.peek()`.
func (h Heap_Heap[T]) M_peek(tk *Task) Option[T] { return List_get(tk, h.F__items, 0) }

// M_get is `h.get(i)`.
func (h Heap_Heap[T]) M_get(tk *Task, i int64) Option[T] { return List_get(tk, h.F__items, i) }

// M_remove is `h.remove(i)`.
func (h *Heap_Heap[T]) M_remove(tk *Task, i int64) Option[T] {
	n := int64(len(h.F__items.s)) - 1
	if i < 0 || i > n {
		return Option[T]{}
	}
	if n != i {
		h.swap(tk, i, n)
		if !h.down(tk, i, n) {
			h.up(tk, i)
		}
	}
	return List_pop(tk, &h.F__items)
}

// M_fix is `h.fix(i, x)`.
func (h *Heap_Heap[T]) M_fix(tk *Task, i int64, x T) {
	if i < 0 || i >= int64(len(h.F__items.s)) {
		FaultHere("heap.fix: position out of range")
	}
	List_set(tk, &h.F__items, i, x)
	h.mv(tk, x, i)
	if !h.down(tk, i, int64(len(h.F__items.s))) {
		h.up(tk, i)
	}
}

// M_len is `h.len()`.
func (h Heap_Heap[T]) M_len(*Task) int64 { return int64(len(h.F__items.s)) }

// M_is_empty is `h.is_empty()`.
func (h Heap_Heap[T]) M_is_empty(*Task) bool { return len(h.F__items.s) == 0 }

// M_to_list is `h.to_list()`.
func (h Heap_Heap[T]) M_to_list(*Task) List[T] { return h.F__items }
