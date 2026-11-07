// The Go twin of bench/perf/jsonmap/main.xo: encoding/json of a map[string]int built in descending key order (Go
// sorts the keys), 10 keys (20000 passes), 1000 keys (200 passes), and a struct holding both.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Doc struct {
	A map[string]int `json:"a"`
	B map[string]int `json:"b"`
}

func build(n int) map[string]int {
	m := map[string]int{}
	for i := n - 1; i >= 0; i-- {
		m["key"+strconv.Itoa(i)] = i
	}
	return m
}

func main() {
	w := "small"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	small := build(10)
	large := build(1000)
	sum := 0
	if w == "small" {
		for p := 0; p < 20000; p++ {
			b, _ := json.Marshal(small)
			sum += len(b)
		}
	} else if w == "large" {
		d := Doc{A: small, B: large}
		for p := 0; p < 200; p++ {
			b1, _ := json.Marshal(large)
			b2, _ := json.Marshal(d)
			sum += len(b1) + len(b2)
		}
	}
	fmt.Println(w, sum)
}
