// Command htmltokoracle prints the std/htmltok corpus: golang.org/x/net
// v0.62.0's html Tokenizer (std/htmltok's reference) on curated tricky
// inputs and seeded random documents built from markup pieces, in each
// mode (plain, AllowCDATA, and the fragment contexts of
// NewTokenizerFragment). scripts/stdcorpus runs it; it is its own module so
// the repository's go.mod has no third-party requirement.
package main

import (
	"fmt"
	"io"
	"math/rand"
	"os"
	"strings"

	"golang.org/x/net/html"
)

// q renders s with the escapes the Xo side of the corpus writes: backslash,
// newline, carriage return, tab and the NUL and other controls as \xHH.
func q(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\':
			b.WriteString(`\\`)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '"':
			b.WriteString(`\"`)
		case c == '$':
			b.WriteString(`\$`)
		case c < 0x20 || c == 0x7f:
			fmt.Fprintf(&b, `\x%02x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// modes: 0 plain, 1 AllowCDATA, 2 title, 3 textarea, 4 script, 5 plaintext,
// 6 style (a fragment context)
var ctx = []string{"", "", "title", "textarea", "script", "plaintext", "style"}

// render is the token stream as one string: one line per token, the type,
// the data and the attributes, then the raw text.
func render(in string, mode int, maxBuf int) string {
	z := html.NewTokenizerFragment(strings.NewReader(in), ctx[mode])
	if mode == 1 {
		z.AllowCDATA(true)
	}
	if maxBuf > 0 {
		z.SetMaxBuf(maxBuf)
	}
	var b strings.Builder
	for {
		tt := z.Next()
		raw := string(z.Raw())
		if tt == html.ErrorToken {
			fmt.Fprintf(&b, "E %s\n", z.Err())
			break
		}
		t := z.Token()
		fmt.Fprintf(&b, "%s %s", tt, q(t.Data))
		for _, a := range t.Attr {
			fmt.Fprintf(&b, " [%s=%s]", q(a.Key), q(a.Val))
		}
		fmt.Fprintf(&b, " <%s>\n", q(raw))
	}
	_ = io.EOF
	return b.String()
}

var curated = []string{
	"", "foo  bar", "one &lt; two", "<a>b<c/>d</e>", "<", "</", "</>", "a</>b", "</ >", "</.", "</.>", "<!", "<!>", "<!--", "<!-->", "<!--->", "<!---->",
	"<!--x-->", "<!-- a -- b -->", "<!--a--!>", "<!--a--!b-->", "<!--a---->", "<!--<!--a-->-->", "<!--a-", "<!--a--", "<!--a--!",
	"<!DOCTYPE html>", "<!doctype html>", "<!DOCTYPE", "<!DOCTYP", "<!DOCTYPE  x y z >", "<!DOCTYPEhtml>", "<!x>", "<?xml version=\"1.0\"?>", "<?x", "<![CDATA[x]]>", "<![CDATA[x]]", "<![CDATA[", "<![CDAT", "<![CDATA[a]]b]]>", "<![CDATA[a\x00b]]>",
	"<a b=c>", "<a b='c'>", "<a b=\"c\">", "<a b>", "<a b=>", "<a =b>", "<a b c d>", "<a b=c d=e>", "<a b=c/>", "<a b='c'/>", "<a b=c />", "<a/>", "<a />", "<a/ b>", "<a b=/>", "<a B=1 b=2>", "<a b=&lt;&amp;&quot;&#65;&#x42;&notanentity;&copy>",
	"<a b=&amp=c>", "<a b=&ampc>", "<a b=&amp;c>", "<a b=\"x\ny\r\nz\rw\">", "<a\x00b c\x00d=e\x00f>", "<A\tB\nC>", "<a b=\"unterminated", "<a b='x", "<a b=", "<a b", "<a ", "<a", "<a>",
	"<script>a<b>c</script>d", "<script>a</scriptx>b</script>", "<SCRIPT>x</SCRIPT >", "<script><!--<script></script>--></script>x", "<script><!--</script>-->", "<script><!-- <script>x</script> --></script>y", "<script><!--a--></script>b", "<script><!-- <SCRIPT >a</script>b</script>c",
	"<script>", "<script>a", "<script>a</", "<script>a</scr", "<script>a</script", "<script>a</script>", "<script type=x>a&amp;b</script>",
	"<style>a<b>&amp;</style>c", "<style>x</style", "<textarea>a<b>&amp;c</textarea>d", "<TEXTAREA>x</textarea>", "<title>a<b>&lt;</title>c", "<title>x</titlex></title>", "<title></title>", "<title>",
	"<iframe>a<b>&amp;</iframe>c", "<noembed>x<b></noembed>", "<noframes>x<b></noframes>", "<noscript>x<b></noscript>", "<xmp>x<b>&amp;</xmp>", "<plaintext>a<b>&amp;</b>", "<plaintext>", "<p>a<plaintext>b</plaintext>c",
	"a\r\nb\rc\nd", "a\x00b", "<!--a\x00b-->", "<!DOCTYPE a\x00b>", "<title>a\x00b</title>", "<script>a\x00b</script>", "<textarea>a\r\nb</textarea>",
	"&", "&amp", "&amp;", "&#", "&#x", "&#65", "&#65;", "&#x41;", "&#X41;", "&#0;", "&#xD800;", "&#x110000;", "&#128;", "&#x9f;", "&#99999999999;", "&notin;", "&notit;", "&copy2", "&copy=", "&not", "&nots", "&NotEqualTilde;", "&lt;&gt;&amp;&quot;&apos;",
	"é<b>ü</b>日本", "<日本 x=é>", "a<1b", "a< b", "a<!b", "a<?b>c", "x<y", "<<a>", "<a<b>", "<a b=c<d>", "</a b=c>", "</a/>", "</a b", "</a", "</a>", "<br/><br /><br>",
}

var pieces = []string{
	"<a>", "</a>", "<b c=d>", "<p>", "</p>", "<br/>", "<br />", "<img src=x alt='y'>", "<a href=\"u?a=1&b=2\">", "<a href='x'/>", "text", " ", "\n", "\r\n", "\r", "&lt;", "&amp;", "&", "&#65;", "&#x;", "&copy", "&notin;",
	"<!--", "-->", "--", "-", "!", "<!-- c -->", "<!>", "<!DOCTYPE html>", "<!doctype", "<![CDATA[", "]]>", "]", "<?pi?>", "<?", "<script>", "</script>", "<style>", "</style>", "<title>", "</title>", "<textarea>", "</textarea>",
	"<iframe>", "</iframe>", "<xmp>", "<plaintext>", "<noscript>", "</noscript>", "<", ">", "/", "=", "'", "\"", "\x00", "é", "日", "<script type=a>", "<!--<script>", "</script >", "</script/>", "<SCRIPT>", "</SCRIPT>",
	"a=b", "a", "b=\"c d\"", "<a ", "<a b", "<a b=", "<a b='", "</", "</a", "</a ", "<a/", "<a />", "< ", "<1", "<!-", "<!-x",
}

func main() {
	r := rand.New(rand.NewSource(548))
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from golang.org/x/net v0.62.0's html Tokenizer (" + os.Args[1] + ") on curated and seeded random inputs in each mode (0 plain, 1 AllowCDATA, 2..6 fragment contexts title, textarea, script, plaintext, style).\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/htmltok\n\n")
	o.WriteString("/// (input, mode, max buffer, Go's token stream)\nconst CASES: List[(Str, Int, Int, Str)] = [\n")
	n := 0
	add := func(in string, mode, maxBuf int) {
		if maxBuf > 0 {
			for i := 0; i < len(in); i++ {
				if in[i] >= 0x80 {
					return // a cut inside a rune is not valid text in an Xo string
				}
			}
		}
		fmt.Fprintf(&o, "  (\"%s\", %d, %d, \"%s\"),\n", q(in), mode, maxBuf, q(render(in, mode, maxBuf)))
		n++
	}
	for _, in := range curated {
		for mode := 0; mode < len(ctx); mode++ {
			add(in, mode, 0)
		}
		for _, mb := range []int{1, 2, 3, 5, 8} {
			add(in, 0, mb)
		}
	}
	for i := 0; i < 1500; i++ {
		var b strings.Builder
		for j, k := 0, 1+r.Intn(9); j < k; j++ {
			b.WriteString(pieces[r.Intn(len(pieces))])
		}
		in := b.String()
		mode := 0
		if r.Intn(4) == 0 {
			mode = r.Intn(len(ctx))
		}
		mb := 0
		if r.Intn(10) == 0 {
			mb = 1 + r.Intn(24)
		}
		add(in, mode, mb)
	}
	o.WriteString("]\n\n")
	fmt.Fprintf(&o, "const COUNT: Int = %d\n", n)
	o.WriteString(xoTests)
	os.Stdout.WriteString(o.String())
}

const xoTests = `
fn type_name(t: htmltok.TokenType) -> Str {
  match t {
    Error => "Error"
    Text => "Text"
    StartTag => "StartTag"
    EndTag => "EndTag"
    SelfClosingTag => "SelfClosingTag"
    Comment => "Comment"
    Doctype => "Doctype"
  }
}

/// The escapes of the oracle's q: backslash, quote, dollar, newline, carriage return, tab, controls as \xHH.
fn q(s: Str) -> Str {
  var o = BytesBuilder.new()
  let hex = "0123456789abcdef".bytes()
  for c in s.bytes() {
    if c == 92 {
      o.write_str("\\\\")
    } else if c == 10 {
      o.write_str("\\n")
    } else if c == 13 {
      o.write_str("\\r")
    } else if c == 9 {
      o.write_str("\\t")
    } else if c == 34 {
      o.write_str("\\\"")
    } else if c == 36 {
      o.write_str("\\$")
    } else if c < 32 || c == 127 {
      o.write_str("\\x")
      o.write_byte(hex[Int(c) / 16])
      o.write_byte(hex[Int(c) % 16])
    } else {
      o.write_byte(c)
    }
  }
  o.freeze().to_str_lossy()
}

fn render(input: Str, mode: Int, max_buf: Int) -> (Str, Str) {
  let ctx = ["", "", "title", "textarea", "script", "plaintext", "style"]
  var z = htmltok.tokenizer_fragment(input, ctx[mode])
  if mode == 1 {
    z.allow_cdata(true)
  }
  if max_buf > 0 {
    z.set_max_buf(max_buf)
  }
  var out = ""
  var raws = ""
  while true {
    let tt = z.next()
    let raw = z.raw()
    if tt == htmltok.TokenType.Error {
      out += "E ${z.err() ?? "?"}\n"
      break
    }
    raws += raw
    let t = z.token()
    out += "${type_name(tt)} ${q(t.data)}"
    for a in t.attrs {
      out += " [${q(a.key)}=${q(a.val)}]"
    }
    out += " <${q(raw)}>\n"
  }
  (out, raws)
}

test "token streams equal Go's x/net/html Tokenizer" {
  for (input, mode, max_buf, want) in CASES {
    let (got, _) = render(input, mode, max_buf)
    if got != want {
      // the first difference shows in the failure
      expect("input ${q(input)} mode ${mode} max ${max_buf}\\n${got}") == "input ${q(input)} mode ${mode} max ${max_buf}\\n${want}"
    }
  }
}

test "the raw texts of the tokens partition the input" {
  for (input, mode, max_buf, _) in CASES {
    if max_buf == 0 {
      let (_, raws) = render(input, mode, 0)
      expect(raws) == input
    }
  }
}

test "the corpus has its cases" {
  expect(CASES.len()) == COUNT
}
`
