package rt

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGoMapConversions(t *testing.T) {
	m := GoMapOf(map[string]int{"b": 2, "a": 1, "c": 3}, func(k string) string { return k }, func(v int) int64 { return int64(v) })
	var keys []string
	for k, v := range Map_All(m) {
		keys = append(keys, k)
		if v != map[string]int64{"a": 1, "b": 2, "c": 3}[k] {
			t.Errorf("value of %s = %d", k, v)
		}
	}
	if strings.Join(keys, "") != "abc" {
		t.Errorf("keys in order %v, want abc", keys)
	}
	back := GoMapToGo(m, func(k string) string { return k }, func(v int64) int { return int(v) })
	if len(back) != 3 || back["c"] != 3 {
		t.Errorf("back = %v", back)
	}
}

func TestGoListCopies(t *testing.T) {
	s := []string{"x", "y"}
	l := GoListCopy(s)
	s[0] = "changed"
	if List_Slice(l)[0] != "x" {
		t.Error("List shares the Go slice")
	}
	out := GoSliceCopy(l)
	out[1] = "changed"
	if List_Slice(l)[1] != "y" {
		t.Error("Go slice shares the List")
	}
	if n := GoListOf([]int{1, 2}, func(x int) int64 { return int64(x) * 10 }); List_Slice(n)[1] != 20 {
		t.Errorf("GoListOf = %v", List_Slice(n))
	}
}

func TestGoBoundary(t *testing.T) {
	recovered := func(f func()) (r any) {
		defer func() { r = recover() }()
		f()
		return nil
	}
	r := recovered(func() {
		defer GoBoundary("pkg.F")
		panic("boom")
	})
	f, ok := r.(*FaultErr)
	if !ok || !strings.Contains(f.Msg, "Go panic in pkg.F: boom") {
		t.Errorf("Go panic became %#v", r)
	}
	orig := &FaultErr{Msg: "xo fault"}
	r = recovered(func() {
		defer GoBoundary("pkg.F")
		panic(orig)
	})
	if r != orig {
		t.Errorf("an Xo fault was rewrapped: %#v", r)
	}
}

func TestGoContextAndError(t *testing.T) {
	tk := NewRootTask()
	ctx := GoContext(tk)
	if ctx.Err() != nil {
		t.Fatal("context done before cancel")
	}
	tk.node.cancel()
	<-ctx.Done()
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Errorf("Err = %v", ctx.Err())
	}
	e := GoErr(errors.New("bad thing"))
	if e.M_display(nil) != "bad thing" || e.M_go_type(nil) != "*errors.errorString" || Debug(e) != `go.Error("bad thing")` {
		t.Errorf("Go_Error: %q %q %q", e.M_display(nil), e.M_go_type(nil), Debug(e))
	}
	if !EqAny(e, GoErr(errors.New("bad thing"))) || EqAny(e, GoErr(errors.New("other"))) {
		t.Error("Go_Error equality")
	}
}
