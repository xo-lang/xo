// Go twin of bench/perf/embed: embed.FS over the same 32 files.
package main

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
)

//go:embed static
var site embed.FS

var names = []string{"css/f00.txt", "js/f01.txt", "img/icons/f02.txt", "f03.txt", "css/f04.txt", "js/f05.txt", "img/icons/f06.txt", "f07.txt", "css/f28.txt", "js/f29.txt", "img/icons/f30.txt", "f31.txt", "missing.txt", "css/nope.txt", "js/f13.txt", "f15.txt"}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	root, _ := fs.Sub(site, "static")
	sum := 0
	switch w {
	case "read":
		for r := 0; r < 100000; r++ {
			for _, n := range names {
				b, _ := fs.ReadFile(root, n)
				sum += len(b)
			}
		}
	case "text":
		for r := 0; r < 100000; r++ {
			for _, n := range names {
				b, err := fs.ReadFile(root, n)
				if err == nil {
					sum += len(string(b))
				}
			}
		}
	case "sub":
		for i := 0; i < 300000; i++ {
			d, _ := fs.Sub(root, "css")
			n := "f08.txt"
			if i%2 == 0 {
				n = "f00.txt"
			}
			b, _ := fs.ReadFile(d, n)
			sum += len(b)
		}
	default:
		n := 0
		fs.WalkDir(root, ".", func(p string, e fs.DirEntry, err error) error {
			if err == nil && !e.IsDir() {
				n++
			}
			return nil
		})
		sum = n
	}
	fmt.Printf("%s %d\n", w, sum)
}
