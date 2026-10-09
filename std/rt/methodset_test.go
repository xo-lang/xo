package rt

import (
	"reflect"
	"testing"
)

// TestGenericMethodSets keeps the method sets of the runtime's generic
// types down to the protocol methods that runtime code reaches through
// interfaces. Every Go package that names an instantiation of a generic
// type compiles all of its methods, and the runtime's API reaches dozens
// of instantiations from Task alone, so each method here is compiled
// again in every generated package (knowledgebase/gotchas/
// go-generic-method-sets-per-package.md). Other operations are package
// level functions (List_len, Map_put, ...), compiled only where called.
func TestGenericMethodSets(t *testing.T) {
	protocol := map[string]bool{
		"XoEq": true, "XoHash": true, "XoCmp": true, "XoDebug": true,
		"XoJsonEnc": true, "XoJsonDec": true, "XoJsonRead": true,
		"XoGenAny": true, "XoMark": true, "M_display": true, "M_debug": true,
	}
	for _, v := range []any{
		List[int64]{}, Map[string, int64]{}, Set[string]{}, Option[int64]{},
		Result[int64, string]{}, Tuple2[int64, string]{}, Tuple3[int64, int64, int64]{},
		Tuple4[int64, int64, int64, int64]{}, Tuple5[int64, int64, int64, int64, int64]{},
		Tuple6[int64, int64, int64, int64, int64, int64]{}, Chan[int64]{}, Secret[int64]{},
		Json_Patch[int64]{},
	} {
		for _, typ := range []reflect.Type{reflect.TypeOf(v), reflect.PointerTo(reflect.TypeOf(v))} {
			for i := 0; i < typ.NumMethod(); i++ {
				if m := typ.Method(i).Name; !protocol[m] {
					t.Errorf("%s has method %s: make it a function (%s_%s) unless runtime code calls it through an interface", typ, m, reflect.TypeOf(v).Name(), m)
				}
			}
		}
	}
}
