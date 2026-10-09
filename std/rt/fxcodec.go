package rt

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"strconv"
	"strings"
	"unicode/utf8"
)

// The value codec of record and replay (core 11.3, replay.go): effect
// arguments and results become JSON trees (bool, json.Number, string,
// []any, map[string]any) and back. Strings that are not valid UTF-8 are
// {"b64": ...} (encoding/json would replace their bytes). Struct fields
// are named without the F_ prefix and left out when zero. A List is an
// array, a Map an array of [key, value] pairs in insertion order, and an
// enum of the {Tag, P} shape (FsErr, NetErr, ...) is {"tag": n, "p":
// {"$": payload type, "v": fields}}.

// fxSeq is implemented by List: the codec reads and builds its elements.
type fxSeq interface {
	fxSlice() reflect.Value
	fxFrom(reflect.Value) any
}

// fxMap is implemented by Map.
type fxMap interface {
	fxPairs() (reflect.Value, reflect.Value)
	fxFrom2(ks, vs reflect.Value) any
}

func (l List[T]) fxSlice() reflect.Value { return reflect.ValueOf(l.s) }

func (List[T]) fxFrom(v reflect.Value) any { return ListOf(v.Interface().([]T)...) }

func (m Map[K, V]) fxPairs() (reflect.Value, reflect.Value) {
	ks := []K{}
	vs := []V{}
	for k, v := range Map_All(m) {
		ks = append(ks, k)
		vs = append(vs, v)
	}
	return reflect.ValueOf(ks), reflect.ValueOf(vs)
}

func (Map[K, V]) fxFrom2(ks, vs reflect.Value) any {
	var m Map[K, V]
	kk, vv := ks.Interface().([]K), vs.Interface().([]V)
	for i := range kk {
		Map_put(nil, &m, kk[i], vv[i])
	}
	return m
}

// fxPayloads are the payload types of the std error enums, by Go name.
var fxPayloads = func() map[string]reflect.Type {
	m := map[string]reflect.Type{}
	for _, v := range []any{
		FsErr_NotFound{}, FsErr_Exists{}, FsErr_NotEmpty{}, FsErr_NotDir{}, FsErr_IsDir{}, FsErr_Denied{}, FsErr_Io{},
		NetErr_InvalidAddr{}, NetErr_Io{}, IoErr_Io{}, EnvErr_Missing{}, ProcErr_NotFound{}, ProcErr_Io{},
		Http_ClientErr_InvalidUrl{}, Http_ClientErr_Connect{}, Http_ClientErr_Protocol{}, Http_HttpErr_Io{},
	} {
		t := reflect.TypeOf(v)
		m[t.Name()] = t
	}
	return m
}()

var (
	seqType      = reflect.TypeFor[fxSeq]()
	mapType      = reflect.TypeFor[fxMap]()
	listenerType = reflect.TypeFor[Listener]()
	connType     = reflect.TypeFor[Conn]()
)

func isEnum(t reflect.Type) bool {
	return t.Kind() == reflect.Struct && t.NumField() == 2 && t.Field(0).Name == "Tag" && t.Field(1).Name == "P" && t.Field(1).Type.Kind() == reflect.Interface
}

func encStr(s string) any {
	if utf8.ValidString(s) {
		return s
	}
	return map[string]any{"b64": base64.StdEncoding.EncodeToString([]byte(s))}
}

// fxEncode encodes a Go value of the runtime.
func fxEncode(x any) any {
	if x == nil {
		return nil
	}
	return fxEnc(reflect.ValueOf(x))
}

func fxEnc(v reflect.Value) any {
	if !v.IsValid() {
		return nil
	}
	t := v.Type()
	switch {
	case t == listenerType:
		l := v.Interface().(Listener)
		return map[string]any{"id": json.Number(strconv.FormatInt(l.id, 10)), "addr": l.addr}
	case t == connType:
		return map[string]any{}
	case t.Implements(seqType) && v.CanInterface():
		s := v.Interface().(fxSeq).fxSlice()
		out := make([]any, s.Len())
		for i := range out {
			out[i] = fxEnc(s.Index(i))
		}
		return out
	case t.Implements(mapType) && v.CanInterface():
		ks, vs := v.Interface().(fxMap).fxPairs()
		out := make([]any, ks.Len())
		for i := range out {
			out[i] = []any{fxEnc(ks.Index(i)), fxEnc(vs.Index(i))}
		}
		return out
	}
	switch v.Kind() {
	case reflect.Bool:
		return v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return json.Number(strconv.FormatInt(v.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return json.Number(strconv.FormatUint(v.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
		return json.Number(strconv.FormatFloat(f, 'g', -1, 64))
	case reflect.String:
		return encStr(v.String())
	case reflect.Slice, reflect.Array:
		out := make([]any, v.Len())
		for i := range out {
			out[i] = fxEnc(v.Index(i))
		}
		return out
	case reflect.Interface:
		if v.IsNil() {
			return nil
		}
		return fxEnc(v.Elem())
	case reflect.Struct:
		if isEnum(t) {
			out := map[string]any{"tag": json.Number(strconv.FormatInt(v.Field(0).Int(), 10))}
			if p := v.Field(1); !p.IsNil() {
				pe := p.Elem()
				out["p"] = map[string]any{"$": pe.Type().Name(), "v": fxEnc(pe)}
			}
			return out
		}
		m := map[string]any{}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			switch f.Type.Kind() {
			case reflect.Func, reflect.Pointer, reflect.Chan, reflect.UnsafePointer:
				continue
			}
			fv := v.Field(i)
			if fv.IsZero() {
				continue
			}
			m[strings.TrimPrefix(f.Name, "F_")] = fxEnc(fv)
		}
		return m
	}
	return nil
}

// fxDecode decodes x into a value of type t.
func fxDecode(x any, t reflect.Type) (v reflect.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("cannot decode %v as %s: %v", x, t, r)
		}
	}()
	return fxDec(x, t), nil
}

func fxDec(x any, t reflect.Type) reflect.Value {
	out := reflect.New(t).Elem()
	if x == nil {
		return out
	}
	switch {
	case t == listenerType:
		m := x.(map[string]any)
		id, _ := m["id"].(json.Number).Int64()
		addr, _ := m["addr"].(string)
		out.Set(reflect.ValueOf(Listener{id: id, replay: true, addr: addr}))
		return out
	case t == connType:
		return out
	case t.Implements(seqType):
		proto := reflect.Zero(t).Interface().(fxSeq)
		st := proto.fxSlice().Type()
		arr := x.([]any)
		s := reflect.MakeSlice(st, len(arr), len(arr))
		for i, e := range arr {
			s.Index(i).Set(fxDec(e, st.Elem()))
		}
		out.Set(reflect.ValueOf(proto.fxFrom(s)))
		return out
	case t.Implements(mapType):
		proto := reflect.Zero(t).Interface().(fxMap)
		pk, pv := proto.fxPairs()
		arr := x.([]any)
		ks := reflect.MakeSlice(pk.Type(), len(arr), len(arr))
		vs := reflect.MakeSlice(pv.Type(), len(arr), len(arr))
		for i, e := range arr {
			kv := e.([]any)
			ks.Index(i).Set(fxDec(kv[0], pk.Type().Elem()))
			vs.Index(i).Set(fxDec(kv[1], pv.Type().Elem()))
		}
		out.Set(reflect.ValueOf(proto.fxFrom2(ks, vs)))
		return out
	}
	switch t.Kind() {
	case reflect.Bool:
		out.SetBool(x.(bool))
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := strconv.ParseInt(string(x.(json.Number)), 10, 64)
		if err != nil {
			panic(err)
		}
		out.SetInt(n)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		n, err := strconv.ParseUint(string(x.(json.Number)), 10, 64)
		if err != nil {
			panic(err)
		}
		out.SetUint(n)
	case reflect.Float32, reflect.Float64:
		var s string
		switch y := x.(type) {
		case json.Number:
			s = string(y)
		case string:
			s = y
		}
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			panic(err)
		}
		out.SetFloat(f)
	case reflect.String:
		switch y := x.(type) {
		case string:
			out.SetString(y)
		case map[string]any:
			b, err := base64.StdEncoding.DecodeString(y["b64"].(string))
			if err != nil {
				panic(err)
			}
			out.SetString(string(b))
		}
	case reflect.Slice:
		arr := x.([]any)
		s := reflect.MakeSlice(t, len(arr), len(arr))
		for i, e := range arr {
			s.Index(i).Set(fxDec(e, t.Elem()))
		}
		out.Set(s)
	case reflect.Struct:
		m := x.(map[string]any)
		if isEnum(t) {
			tag, _ := m["tag"].(json.Number).Int64()
			out.Field(0).SetInt(tag)
			if p, ok := m["p"].(map[string]any); ok {
				name, _ := p["$"].(string)
				pt, ok := fxPayloads[name]
				if !ok {
					panic("unknown error payload " + name)
				}
				out.Field(1).Set(fxDec(p["v"], pt))
			}
			return out
		}
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			if e, ok := m[strings.TrimPrefix(f.Name, "F_")]; ok {
				out.Field(i).Set(fxDec(e, f.Type))
			}
		}
	}
	return out
}

// fxText is the compact JSON text of an encoded tree.
func fxText(x any) string {
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.Encode(x)
	return strings.TrimSuffix(b.String(), "\n")
}

// fxCall renders an effect call for messages: `fs.read("a.txt")`.
func fxCall(op string, args []any) string {
	parts := make([]string, len(args))
	for i, a := range args {
		s := fxText(a)
		if len(s) > 80 {
			s = s[:77] + "..."
		}
		parts[i] = s
	}
	return op + "(" + strings.Join(parts, ", ") + ")"
}
