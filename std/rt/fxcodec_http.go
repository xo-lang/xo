//go:build !xo_nohttp

package rt

import "reflect"

// std/http's error payloads among fxPayloads (fxcodec.go), in a program
// that links std/http (task #472)
func init() {
	for _, v := range []any{Http_ClientErr_InvalidUrl{}, Http_ClientErr_Connect{}, Http_ClientErr_Protocol{}, Http_HttpErr_Io{}} {
		t := reflect.TypeOf(v)
		fxPayloads[t.Name()] = t
	}
}
