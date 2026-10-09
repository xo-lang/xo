package rt

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// TestGoErrorCodec round trips a go.Error result through the effect codec
// (decision 0093): the text and the Go type survive, as JSON too.
func TestGoErrorCodec(t *testing.T) {
	_, err := os.ReadFile("/no/such/file")
	r := Result[Bytes, Go_Error]{E: GoErr(err)}
	text := fxText(fxEncode(r))
	var tree any
	dec := json.NewDecoder(strings.NewReader(text))
	dec.UseNumber()
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	v, derr := fxDecode(tree, reflect.TypeFor[Result[Bytes, Go_Error]]())
	if derr != nil {
		t.Fatal(derr)
	}
	got := v.Interface().(Result[Bytes, Go_Error])
	if got.Ok || got.E.M_display(nil) != err.Error() || got.E.M_go_type(nil) != "*fs.PathError" {
		t.Fatalf("decoded %s as %+v (type %s)", text, got, got.E.M_go_type(nil))
	}
	if !got.E.XoEq(r.E) {
		t.Error("a replayed go.Error is not equal to the recorded one")
	}
	ok := Result[int64, Go_Error]{Ok: true, V: 7}
	v, _ = fxDecode(roundTrip(t, fxEncode(ok)), reflect.TypeFor[Result[int64, Go_Error]]())
	if v.Interface().(Result[int64, Go_Error]) != ok {
		t.Errorf("ok result: %+v", v.Interface())
	}
}

// TestGoArgHash: equal arguments hash equal, others differ.
func TestGoArgHash(t *testing.T) {
	a := goArgHash([]any{"x", int64(1)})
	if a != goArgHash([]any{"x", int64(1)}) || a == goArgHash([]any{"x", int64(2)}) || len(a) != 16 {
		t.Errorf("hash %q", a)
	}
}

func roundTrip(t *testing.T, x any) any {
	t.Helper()
	var tree any
	dec := json.NewDecoder(strings.NewReader(fxText(x)))
	dec.UseNumber()
	if err := dec.Decode(&tree); err != nil {
		t.Fatal(err)
	}
	return tree
}
