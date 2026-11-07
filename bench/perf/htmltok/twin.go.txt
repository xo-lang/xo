package main

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/net/html"
)

func page() string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><title>Bench &amp; page</title><style>p{color:red}</style></head><body>")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "<!-- item %d --><div class=\"post\" id=p%d><h2>Post %d</h2><p>Tom &amp; Jerry's <a href=\"/x?a=1&amp;b=2\" title='show %d'>show</a> &copy; text &lt;b&gt; more words here.</p><img src=a.png alt=\"\"><br/></div>\n", i, i, i, i)
	}
	b.WriteString("<script>if (a < b && c > d) { document.write('<p>x</p>'); }</script></body></html>")
	return b.String()
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	t := page()
	sum := 0
	switch w {
	case "next":
		for i := 0; i < 2000; i++ {
			z := html.NewTokenizer(strings.NewReader(t))
			for z.Next() != html.ErrorToken {
				sum++
			}
			sum += i % 3
		}
	case "token":
		for i := 0; i < 2000; i++ {
			z := html.NewTokenizer(strings.NewReader(t))
			for z.Next() != html.ErrorToken {
				k := z.Token()
				sum += 1 + len(k.Data) + len(k.Attr)
			}
			sum += i % 3
		}
	default:
		for i := 0; i < 2000; i++ {
			sum += len(t) + i%3
		}
	}
	fmt.Println(w, sum)
}
