package rt

import (
	"fmt"
	"strings"
	"testing"
)

func BenchmarkJsonStream(b *testing.B) {
	var sb strings.Builder
	for i := 0; i < 10000; i++ {
		fmt.Fprintf(&sb, "{\"id\":%d,\"name\":\"name number %d\",\"tags\":[\"alpha\",\"beta\",\"gamma%d\"],\"score\":%v,\"nested\":{\"a\":%d,\"b\":%v},\"note\":null}\n", i, i, i%10, float64(i)/4.0, i*3, i%2 == 0)
	}
	doc := []byte(sb.String())
	b.ResetTimer()
	for k := 0; k < b.N; k++ {
		d := Json_decoder(nil, 67108864)
		n := 0
		for at := 0; at < len(doc); at += 4096 {
			e := at + 4096
			if e > len(doc) {
				e = len(doc)
			}
			r := d.M_write(nil, Bytes(doc[at:e]))
			n += len(r.V.s)
		}
		r := d.M_finish(nil)
		n += len(r.V.s)
		if n != 10000 {
			b.Fatal(n)
		}
	}
}
