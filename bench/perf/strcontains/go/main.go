package main

import (
	"fmt"
	"os"
	"strings"
)

func main() {
	w := "byte"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	var lines []string
	s := "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJ"
	for i := 0; i < 64; i++ {
		lines = append(lines, fmt.Sprintf("line %d: ", i)+s[:18+i%28])
	}
	sub := "\x1b"
	if w == "word" {
		sub = "zzz"
	}
	n := 0
	if w != "none" {
		for k := 0; k < 20000000; k++ {
			if w == "strip" {
				l := lines[k&63]
				if strings.Contains(l, "\x1b") {
					l = ""
				}
				n += len(l) & 1
				continue
			}
			if strings.Contains(lines[k&63], sub) {
				n++
			}
			n += len(lines[k&63]) & 1
		}
	}
	fmt.Printf("%s %d\n", w, n)
}
