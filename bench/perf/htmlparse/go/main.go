package main

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"golang.org/x/net/html"
)

func page() string {
	var b strings.Builder
	b.WriteString("<!DOCTYPE html><html><head><title>Bench &amp; page</title><style>p{color:red}</style></head><body>")
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&b, "<!-- item %d --><div class=\"post\" id=p%d><h2>Post %d</h2><p>Tom &amp; Jerry's <a href=\"/x?a=1&amp;b=2\" title='show %d'>show</a> &copy; text &lt;b&gt; more words here.<p><b>bold <i>both</b> italic</i><table><tr><td>cell<td>two</table><img src=a.png alt=\"\"><br/></div>\n", i, i, i, i)
	}
	b.WriteString("<script>if (a < b && c > d) { document.write('<p>x</p>'); }</script></body></html>")
	return b.String()
}

func entPage() string {
	names := []string{"hellip", "mdash", "ndash", "eacute", "Aacute", "ouml", "szlig", "laquo", "raquo", "euro", "trade", "reg", "deg", "plusmn", "frac12", "times", "divide", "larr", "rarr", "hearts", "alpha", "beta", "Omega", "infin", "ne", "le", "ge", "sum", "radic", "bull"}
	var b strings.Builder
	b.WriteString("<html><body>")
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "<p title=\"a &%s; b\">", names[i%30])
		for j := 0; j < 6; j++ {
			fmt.Fprintf(&b, "word &%s; &#%d; text ", names[(i+j*7)%30], 65+(i+j)%26)
		}
		b.WriteString("</p>\n")
	}
	b.WriteString("</body></html>")
	return b.String()
}

func count(n *html.Node) int {
	c := 1 + len(n.Data) + len(n.Attr)
	for k := n.FirstChild; k != nil; k = k.NextSibling {
		c += count(k)
	}
	return c
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	t := page()
	sum := 0
	switch w {
	case "parse":
		for i := 0; i < 1000; i++ {
			doc, err := html.Parse(strings.NewReader(t))
			if err != nil {
				panic(err)
			}
			sum += count(doc) + i%3
		}
	case "tok":
		for i := 0; i < 1000; i++ {
			z := html.NewTokenizer(strings.NewReader(t))
			for z.Next() != html.ErrorToken {
				tk := z.Token()
				sum += len(tk.Data) + len(tk.Attr)
			}
			sum += i % 3
		}
	case "ent":
		e := entPage()
		for i := 0; i < 1000; i++ {
			z := html.NewTokenizer(strings.NewReader(e))
			for z.Next() != html.ErrorToken {
				tk := z.Token()
				sum += len(tk.Data) + len(tk.Attr)
			}
			sum += i % 3
		}
	case "render":
		doc, err := html.Parse(strings.NewReader(t))
		if err != nil {
			panic(err)
		}
		for i := 0; i < 1000; i++ {
			var b bytes.Buffer
			if err := html.Render(&b, doc); err != nil {
				panic(err)
			}
			sum += b.Len() + i%3
		}
	default:
		for i := 0; i < 1000; i++ {
			sum += len(t) + i%3
		}
	}
	fmt.Println(w, sum)
}
