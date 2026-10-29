package rt

import (
	"fmt"
	"hash/maphash"
	"reflect"
	"strings"
	"time"
)

// Go_Any is `go.Any` (decision 0412): a Go `any` value a `use go`
// binding takes or returns, opaque to Xo. replayed marks a value a replay
// rebuilt from its recorded text (decision 0412 7): it only displays.
type Go_Any struct {
	V        any
	replayed string
	isReplay bool
}

// GoAnyReplayed is the go.Any a replay hands back for a recorded value's
// text: it displays as the recording did and its as_* calls fault.
func GoAnyReplayed(text string) Go_Any { return Go_Any{replayed: text, isReplay: true} }

// GoValuer is implemented by the handle types a binding generates: the Go
// value a handle holds, as Go's `any` (its pointer for a handle by pointer).
type GoValuer interface{ XoGoValue() any }

// GoFromer is implemented by the handle types a binding generates: the
// handle of a Go value of its type, false for any other dynamic type.
type GoFromer interface{ XoFromGo(v any) (any, bool) }

// Go_any is `go.any(x)`: an Xo value as the Go value a binding would pass
// for it: Int and the sized ints as Go's int64 and sized ints, Float as
// float64, Str as string, Bytes as []byte, Duration as time.Duration, Time
// as time.Time (UTC), a handle as its Go value, a List as a slice, a Map
// as a map, an Option as its value or nil, a go.Any as itself.
func Go_any[T any](_ *Task, x T) Go_Any { return Go_Any{V: goValueOf(any(x))} }

// GoAnyOf is the adapter's conversion of a `...any` element or an `any`
// argument the checker converted implicitly (decision 0412 2).
func GoAnyOf[T any](x T) Go_Any { return Go_Any{V: goValueOf(any(x))} }

func goValueOf(v any) any {
	switch x := v.(type) {
	case nil:
		return nil
	case Go_Any:
		return x.V
	case Bytes:
		return []byte(x)
	case Duration:
		return time.Duration(x)
	case Time:
		return time.Unix(0, int64(x)).UTC()
	case GoValuer:
		return x.XoGoValue()
	case int64, float64, bool, string, float32, int8, int16, int32, uint8, uint16, uint32, uint64:
		return x
	}
	return goValueReflect(reflect.ValueOf(v))
}

// goValueReflect converts the generic Xo values (List, Map, Option) by
// their shape: a List's elements, a Map's entries, an Option's value.
func goValueReflect(rv reflect.Value) any {
	if c, ok := rv.Interface().(interface{ xoGoSeq() []any }); ok {
		return typedSlice(c.xoGoSeq())
	}
	if c, ok := rv.Interface().(interface{ xoGoMap() ([]any, []any) }); ok {
		ks, vs := c.xoGoMap()
		return typedMap(ks, vs)
	}
	if rv.Kind() == reflect.Struct {
		if s, v := rv.FieldByName("Some"), rv.FieldByName("V"); s.IsValid() && v.IsValid() && s.Kind() == reflect.Bool && strings.HasPrefix(rv.Type().Name(), "Option[") {
			if !s.Bool() {
				return nil
			}
			return goValueOf(v.Interface())
		}
	}
	return rv.Interface()
}

// typedSlice makes a slice of the elements' one Go type ([]int64,
// []string), or []any when they differ or there are none to tell.
func typedSlice(xs []any) any {
	if t := oneType(xs); t != nil {
		s := reflect.MakeSlice(reflect.SliceOf(t), len(xs), len(xs))
		for i, x := range xs {
			s.Index(i).Set(reflect.ValueOf(x))
		}
		return s.Interface()
	}
	return xs
}

func typedMap(ks, vs []any) any {
	kt, vt := oneType(ks), oneType(vs)
	if kt == nil || !kt.Comparable() {
		kt = reflect.TypeFor[any]()
	}
	if vt == nil {
		vt = reflect.TypeFor[any]()
	}
	m := reflect.MakeMapWithSize(reflect.MapOf(kt, vt), len(ks))
	for i := range ks {
		k, v := reflect.ValueOf(ks[i]), reflect.ValueOf(vs[i])
		if vs[i] == nil {
			v = reflect.Zero(vt)
		}
		m.SetMapIndex(k, v)
	}
	return m.Interface()
}

func oneType(xs []any) reflect.Type {
	var t reflect.Type
	for _, x := range xs {
		if x == nil {
			return nil
		}
		xt := reflect.TypeOf(x)
		if t != nil && xt != t {
			return nil
		}
		t = xt
	}
	return t
}

func (l List[T]) xoGoSeq() []any {
	s := List_Slice(l)
	out := make([]any, len(s))
	for i, x := range s {
		out[i] = goValueOf(any(x))
	}
	return out
}

func (m Map[K, V]) xoGoMap() ([]any, []any) {
	ks := make([]any, 0, m.n)
	vs := make([]any, 0, m.n)
	for k, v := range Map_All(m) {
		ks = append(ks, goValueOf(any(k)))
		vs = append(vs, goValueOf(any(v)))
	}
	return ks, vs
}

func (a Go_Any) check(tk *Task, what string) {
	if a.isReplay {
		FaultHere("%s", "go.Any."+what+" on a replayed Go value ("+a.replayed+"): mark the function `replay rerun` to read it in a replay")
	}
}

// M_display is Go's %v of the value.
func (a Go_Any) M_display(*Task) string {
	if a.isReplay {
		return a.replayed
	}
	return fmt.Sprintf("%v", a.V)
}

// M_type_name is Go's %T of the value ("<nil>" for nil).
func (a Go_Any) M_type_name(tk *Task) string {
	a.check(tk, "type_name")
	return fmt.Sprintf("%T", a.V)
}

// M_is_nil is whether the value is Go's nil (or a nil pointer, slice or
// map in the interface).
func (a Go_Any) M_is_nil(tk *Task) bool {
	a.check(tk, "is_nil")
	if a.V == nil {
		return true
	}
	rv := reflect.ValueOf(a.V)
	switch rv.Kind() {
	case reflect.Pointer, reflect.Slice, reflect.Map, reflect.Func, reflect.Chan, reflect.Interface:
		return rv.IsNil()
	}
	return false
}

// M_as_int is the value as an Int when it is one of Go's integer kinds
// that fits; None otherwise.
func (a Go_Any) M_as_int(tk *Task) Option[int64] {
	a.check(tk, "as_int")
	rv := reflect.ValueOf(a.V)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return Some(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		if u := rv.Uint(); u <= 1<<63-1 {
			return Some(int64(u))
		}
	}
	return None[int64]()
}

// M_as_float is the value as a Float when it is a float64 or float32.
func (a Go_Any) M_as_float(tk *Task) Option[float64] {
	a.check(tk, "as_float")
	rv := reflect.ValueOf(a.V)
	switch rv.Kind() {
	case reflect.Float32, reflect.Float64:
		return Some(rv.Float())
	}
	return None[float64]()
}

// M_as_bool is the value as a Bool when it is one.
func (a Go_Any) M_as_bool(tk *Task) Option[bool] {
	a.check(tk, "as_bool")
	rv := reflect.ValueOf(a.V)
	if rv.Kind() == reflect.Bool {
		return Some(rv.Bool())
	}
	return None[bool]()
}

// M_as_str is the value as a Str when its kind is string.
func (a Go_Any) M_as_str(tk *Task) Option[string] {
	a.check(tk, "as_str")
	rv := reflect.ValueOf(a.V)
	if rv.Kind() == reflect.String {
		return Some(rv.String())
	}
	return None[string]()
}

// M_as_bytes is the value as Bytes when it is a []byte (copied).
func (a Go_Any) M_as_bytes(tk *Task) Option[Bytes] {
	a.check(tk, "as_bytes")
	rv := reflect.ValueOf(a.V)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Kind() == reflect.Uint8 {
		return Some(Bytes(rv.Bytes()))
	}
	return None[Bytes]()
}

// M_as_list is the value's elements when it is a slice or an array.
func (a Go_Any) M_as_list(tk *Task) Option[List[Go_Any]] {
	a.check(tk, "as_list")
	rv := reflect.ValueOf(a.V)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return None[List[Go_Any]]()
	}
	out := make([]Go_Any, rv.Len())
	for i := range out {
		out[i] = Go_Any{V: rv.Index(i).Interface()}
	}
	if len(out) == 0 {
		return Some(List[Go_Any]{})
	}
	return Some(ListOf(out...))
}

// M_as_map is the value's entries when it is a map with string keys, in
// key order.
func (a Go_Any) M_as_map(tk *Task) Option[Map[string, Go_Any]] {
	a.check(tk, "as_map")
	rv := reflect.ValueOf(a.V)
	if rv.Kind() != reflect.Map || rv.Type().Key().Kind() != reflect.String {
		return None[Map[string, Go_Any]]()
	}
	m := make(map[string]any, rv.Len())
	it := rv.MapRange()
	for it.Next() {
		m[it.Key().String()] = it.Value().Interface()
	}
	return Some(GoMapOf(m, func(k string) string { return k }, func(v any) Go_Any { return Go_Any{V: v} }))
}

// Go_Any_as_handle is `a.as_handle[T]()`: the handle of type T holding the value when
// its dynamic type is T's Go type.
func Go_Any_as_handle[T any](tk *Task, a Go_Any) Option[T] {
	a.check(tk, "as_handle")
	var zero T
	if f, ok := any(zero).(GoFromer); ok {
		if h, ok := f.XoFromGo(a.V); ok {
			return Some(h.(T))
		}
		return None[T]()
	}
	if t, ok := a.V.(T); ok {
		return Some(t)
	}
	return None[T]()
}

// XoEq is Go's == on the interface; an uncomparable dynamic type (a slice
// or map) is equal only to the same go.Any value's own contents by
// identity, never a Go panic (GoEqIface).
func (a Go_Any) XoEq(o any) bool {
	b, ok := o.(Go_Any)
	if !ok {
		return false
	}
	if a.isReplay || b.isReplay {
		return a.isReplay && b.isReplay && a.replayed == b.replayed
	}
	return GoEqIface(a.V, b.V)
}

// XoHash agrees with XoEq.
func (a Go_Any) XoHash() uint64 {
	if a.isReplay {
		return maphash.String(hashSeed, a.replayed)
	}
	return GoHashIface(a.V)
}

// XoDebug writes the value as Go's %#v.
func (a Go_Any) XoDebug(b *strings.Builder) {
	if a.isReplay {
		b.WriteString(a.replayed)
		return
	}
	fmt.Fprintf(b, "go.Any(%#v)", a.V)
}

// Go_Out is `go.Out[T]` (decision 0412 4): a Go *T a Go function fills
// through `any` (Scan(&x), Unmarshal(&v)); P holds the Go value of T's
// Go type.
type Go_Out[T any] struct{ P any }

// Go_out is `go.out[T]()`: a new Go pointer to the zero value of T's Go
// type.
func Go_out[T any](_ *Task) Go_Out[T] {
	var zero T
	t := reflect.TypeOf(goValueOf(any(zero)))
	if _, isAny := any(zero).(Go_Any); isAny || t == nil {
		return Go_Out[T]{P: new(any)}
	}
	return Go_Out[T]{P: reflect.New(t).Interface()}
}

// M_any is the pointer as the go.Any a Go function fills.
func (o Go_Out[T]) M_any(*Task) Go_Any { return Go_Any{V: o.P} }

// M_get reads the value Go wrote, as T.
func (o Go_Out[T]) M_get(tk *Task) T {
	v := reflect.ValueOf(o.P).Elem().Interface()
	var zero T
	switch any(zero).(type) {
	case Go_Any:
		return any(Go_Any{V: v}).(T)
	case Bytes:
		b, _ := v.([]byte)
		return any(Bytes(b)).(T)
	case Duration:
		d, _ := v.(time.Duration)
		return any(Duration(d)).(T)
	case Time:
		t, _ := v.(time.Time)
		return any(Time(t.UnixNano())).(T)
	}
	if f, ok := any(zero).(GoFromer); ok {
		h, ok := f.XoFromGo(v)
		if !ok {
			FaultHere("go.Out.get: Go wrote a %T", v)
		}
		return h.(T)
	}
	t, ok := v.(T)
	if !ok {
		FaultHere("go.Out.get: Go wrote a %T", v)
	}
	return t
}

// GoAnyTo is a go.Any as a named empty interface type of a Go package.
func GoAnyTo[T any](a Go_Any) T {
	t, _ := a.V.(T)
	return t
}
