package main

import (
	"bytes"
	"encoding/json"
	"testing"
)

// TestAppendPage: GET /alloc-hand sends what GET /alloc-strconv does.
func TestAppendPage(t *testing.T) {
	for _, n := range []int{0, 1, items} {
		p := pageStrconv(n)
		want, err := json.Marshal(p)
		if err != nil {
			t.Fatal(err)
		}
		if got := appendPage(nil, p); !bytes.Equal(got, want) {
			t.Errorf("n=%d:\n got %s\nwant %s", n, got, want)
		}
	}
	odd := Page{Total: -3, Items: []Item{{ID: -1, Name: "a\"<b>\u00e9\n", Score: 1e-9}, {Score: 2.5e22, Tags: []string{}}}}
	want, _ := json.Marshal(odd)
	if got := appendPage(nil, odd); !bytes.Equal(got, want) {
		t.Errorf("odd:\n got %s\nwant %s", got, want)
	}
}
