// Go twin of bench/perf/ui: x/net/html node trees and html.Render.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/net/html"
)

func el(tag string, attrs []html.Attribute, kids ...*html.Node) *html.Node {
	n := &html.Node{Type: html.ElementNode, Data: tag, Attr: attrs}
	for _, k := range kids {
		n.AppendChild(k)
	}
	return n
}

func text(s string) *html.Node { return &html.Node{Type: html.TextNode, Data: s} }

func page(n int) *html.Node {
	rows := make([]*html.Node, 0, n)
	for i := 0; i < n; i++ {
		is := strconv.Itoa(i)
		rows = append(rows, el("tr", nil,
			el("td", nil, text("row "+is+" & co <b>")),
			el("td", nil, el("a", []html.Attribute{{Key: "href", Val: "/item/" + is}, {Key: "class", Val: "link"}}, text("item "+is))),
			el("td", nil, el("span", []html.Attribute{{Key: "data-id", Val: is}}, text("\"quoted\" 'text'"))),
		))
	}
	return el("div", []html.Attribute{{Key: "id", Val: "root"}}, el("table", []html.Attribute{{Key: "class", Val: "grid"}}, el("tbody", nil, rows...)))
}

func render(n *html.Node) string {
	var b bytes.Buffer
	html.Render(&b, n)
	return b.String()
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	p := page(200)
	sum := 0
	switch w {
	case "render":
		for i := 0; i < 5000; i++ {
			sum += len(render(p))
		}
	case "build":
		for i := 0; i < 20000; i++ {
			n := 0
			for c := page(10 + i%3).FirstChild; c != nil; c = c.NextSibling {
				n++
			}
			sum += n
		}
		sum += len(render(p))
	}
	fmt.Printf("%s %d\n", w, sum)
}
