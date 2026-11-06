// DRAFT oracle for std/timego (own module internal/stdtest/timeref, stdlib only, so no go.sum):
// reads lines "op<TAB>layout<TAB>arg" from stdin and prints Go's answer.
//
//	fmt  layout unix_ns zone_offset_secs  -> time.Unix(0,ns).In(FixedZone("ZZ", off)).Format(layout)
//	parse layout text                      -> time.Parse(layout, text): "ok <unix_ns> <offset>" or "err"
package main

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		switch f[0] {
		case "fmt":
			ns, _ := strconv.ParseInt(f[2], 10, 64)
			off, _ := strconv.Atoi(f[3])
			z := time.FixedZone("ZZ", off)
			if off == 0 {
				z = time.UTC
			}
			fmt.Println(time.Unix(0, ns).In(z).Format(f[1]))
		case "parse":
			t, err := time.Parse(f[1], f[2])
			if err != nil {
				fmt.Println("err")
				continue
			}
			// a Time of the Xo side holds int64 nanoseconds since 1970 (decision 0364): 1677 to 2262
			if t.Before(time.Unix(0, math.MinInt64)) || t.After(time.Unix(0, math.MaxInt64)) {
				fmt.Println("range")
				continue
			}
			_, off := t.Zone()
			fmt.Printf("ok %d %d\n", t.UnixNano(), off)
		}
	}
}
