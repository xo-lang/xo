package rt

import "testing"

type adaptIface interface {
	M_name(*Task) Result[string, IError]
}

type adaptPlain struct{ s string }

func (p adaptPlain) M_name(*Task) string { return p.s }

type adaptWrap struct{ V adaptPlain }

func (a adaptWrap) M_name(tk *Task) Result[string, IError] {
	return Ok[string, IError](a.V.M_name(tk))
}

// TestIfaceAdapter checks that Iface wraps a value whose Go methods differ
// from the interface's in the adapter registered for it.
func TestIfaceAdapter(t *testing.T) {
	RegAdapter[adaptIface](func(v any) (adaptIface, bool) {
		x, ok := v.(adaptPlain)
		if !ok {
			return nil, false
		}
		return adaptWrap{V: x}, true
	})
	got := Iface[adaptIface](adaptPlain{s: "x"}).M_name(nil)
	if !got.Ok || got.V != "x" {
		t.Fatalf("got %+v", got)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("no fault for a value without an adapter")
		}
	}()
	Iface[adaptIface](3)
}
