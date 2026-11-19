// A fuzz harness for std/ui (task #377, decision 0616's fuzz kind), two parts.
//
// Render: random views of text and elements whose text and attribute values are built from hostile pieces
// (markup, quotes, '&', schemes in every case, tabs, newlines, C0 controls, Unicode spaces, ':' '/' '?' '#').
// ui.render must give exactly what Go computes: html.EscapeString of every text and value, a URL attribute
// (href, src, action, formaction) kept or replaced by about:invalid#xo-unsafe by std/ui's rule, void elements
// with no close tag. Go also checks the rule itself against a model of how a browser reads a URL's scheme
// (the WHATWG URL parser: leading and trailing C0 controls and spaces stripped, tabs and newlines removed, a
// scheme an ASCII letter then letters, digits, '+', '-', '.' up to ':'): a value the rule keeps never has a
// scheme other than http, https, mailto or tel, or the generator fails.
//
// Sync: random schedules of 2 or 3 clients over a ui.Server with a small max_batch: Msgs sent, syncs, replies
// lost (the server took the Up, the client never saw the Down), reloads (a new client at version 0, its unsent
// Msgs gone) and compactions. After every step no replica is past the server; at the end every client syncs
// until it has nothing pending and is at the server's version, and the server's model, every replica's model
// and confirmed model must equal what Go computes by replaying the schedule (each Msg once, in the order the
// server first took it).
//
// TestUiFuzz runs XO_UIFUZZ_SEED (default 1) with XO_UIFUZZ_N cases of each part (default 2000) on
// XO_UIFUZZ_BACKENDS (default arm64 and llvm).
package stdtest

import (
	"fmt"
	"html"
	"math"
	"math/rand"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/hostile"
)

var uiFuzzPieces = []string{"", "x", "a b", "javascript", "JavaScript", "JAVASCRIPT", "java\tscript", "jav\nascript",
	"vbscript", "data", "DATA", "http", "HTTP", "https", "mailto", "tel", "ftp", "file", "blob", ":", ":", ":", "/", "//",
	"?", "#", " ", "\t", "\n", "\r", "\x01", "\x00", "\x7f", " ", "　", "&", "&amp;", "&#58;", "&colon;", "<",
	">", "\"", "'", "`", "=", "alert(1)", "é", "日本", "\U0001F600", "evil.com", "%3a", "\\", "$", "{x}", "<script>",
	"</a>", "-->", "<!--", "javascript:", "http:", "data:text/html,", "："}

func uiFuzzStr(r *rand.Rand) string {
	var b strings.Builder
	for i, n := 0, r.Intn(6); i < n; i++ {
		b.WriteString(uiFuzzPieces[r.Intn(len(uiFuzzPieces))])
	}
	return b.String()
}

// uiURLKept is std/ui's rule (std/ui.xo safe_url): kept when there is no ':' or a '/', '?' or '#' comes before the
// first one, or the scheme before it, trimmed and lower cased, is http, https, mailto or tel.
func uiURLKept(v string) bool {
	t := strings.ToLower(strings.TrimSpace(v))
	i := strings.Index(t, ":")
	if i < 0 {
		return true
	}
	if j := strings.IndexAny(t, "/?#"); j >= 0 && j < i {
		return true
	}
	switch t[:i] {
	case "http", "https", "mailto", "tel":
		return true
	}
	return false
}

// uiBrowserScheme is the scheme a browser reads from an attribute's value, lower cased, "" for a relative URL.
func uiBrowserScheme(v string) string {
	s := strings.TrimFunc(v, func(r rune) bool { return r <= 0x20 })
	s = strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, s)
	alpha := func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	if s == "" || !alpha(s[0]) {
		return ""
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		switch {
		case c == ':':
			return strings.ToLower(s[:i])
		case alpha(c) || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '.':
		default:
			return ""
		}
	}
	return ""
}

var uiFuzzTags = []string{"div", "p", "span", "a", "form", "button", "ul", "li", "h1", "img", "br", "input", "hr",
	"link", "meta", "source", "td", "x-y", "h2"}
var uiFuzzVoid = map[string]bool{"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true, "track": true, "wbr": true}
var uiFuzzAttrs = []string{"href", "src", "action", "formaction", "class", "id", "title", "data-x", "value", "alt",
	"name", "hreff", "srcset", "data-href", "style"}

// uiFuzzView returns an Xo expression for a random view and the HTML Go renders for it.
func uiFuzzView(t *testing.T, r *rand.Rand, depth int) (string, string) {
	if depth == 0 || r.Intn(3) == 0 {
		s := uiFuzzStr(r)
		return "ui.text(" + xoStr(s) + ")", html.EscapeString(s)
	}
	tag := uiFuzzTags[r.Intn(len(uiFuzzTags))]
	var attrs []string
	var b strings.Builder
	b.WriteString("<" + tag)
	for i, n := 0, r.Intn(4); i < n; i++ {
		name := uiFuzzAttrs[r.Intn(len(uiFuzzAttrs))]
		v := uiFuzzStr(r)
		attrs = append(attrs, "ui.attr("+xoStr(name)+", "+xoStr(v)+")")
		out := v
		if name == "href" || name == "src" || name == "action" || name == "formaction" {
			if uiURLKept(v) {
				switch uiBrowserScheme(v) {
				case "", "http", "https", "mailto", "tel":
				default:
					t.Fatalf("std/ui's URL rule keeps %q, which a browser reads with scheme %q", v, uiBrowserScheme(v))
				}
			} else {
				out = "about:invalid#xo-unsafe"
			}
		}
		b.WriteString(" " + name + "=\"" + html.EscapeString(out) + "\"")
	}
	b.WriteString(">")
	var kids []string
	n := r.Intn(4)
	for i := 0; i < n; i++ {
		e, h := uiFuzzView(t, r, depth-1)
		kids = append(kids, e)
		if !uiFuzzVoid[tag] {
			b.WriteString(h)
		}
	}
	if !uiFuzzVoid[tag] {
		b.WriteString("</" + tag + ">")
	}
	return "ui.el(" + xoStr(tag) + ", attrs: [" + strings.Join(attrs, ", ") + "], kids: [" + strings.Join(kids, ", ") + "])", b.String()
}

// uiFuzzSchedule is a random schedule as Xo (client, op, arg) triples and the server's final model as Go
// replays it. Ops: 0 send arg, 1 sync, 2 sync with the reply lost, 3 reload, 4 compact to arg entries.
func uiFuzzSchedule(r *rand.Rand, clients int) (string, string) {
	type cl struct {
		sent []int64
		last int
	}
	cs := make([]cl, clients)
	var log []int64
	deliver := func(c int) {
		log = append(log, cs[c].sent[cs[c].last:]...)
		cs[c].last = len(cs[c].sent)
	}
	var ops []string
	next := 1
	for i, n := 0, 5+r.Intn(40); i < n; i++ {
		c := r.Intn(clients)
		op, arg := 0, int64(0)
		switch k := r.Intn(20); {
		case k < 9:
			op, arg = 0, int64(c*100000+next)
			next++
			if r.Intn(4) == 0 {
				arg = hostile.Pick(r, hostile.Ints())
			}
			cs[c].sent = append(cs[c].sent, arg)
		case k < 14:
			op = 1
			deliver(c)
		case k < 17:
			op = 2
			deliver(c)
		case k < 18:
			op = 3
			cs[c] = cl{}
		default:
			op, arg = 4, int64(r.Intn(6))
		}
		a := fmt.Sprint(arg)
		if arg == math.MinInt64 {
			a = "-9223372036854775807 - 1"
		}
		ops = append(ops, fmt.Sprintf("(%d, %d, %s)", c, op, a))
	}
	for c := range cs {
		deliver(c)
	}
	want := make([]string, len(log))
	for i, v := range log {
		want[i] = fmt.Sprint(v)
	}
	return "[" + strings.Join(ops, ", ") + "]", strings.Join(want, ",")
}

func uiFuzz(t *testing.T, seed int64, n int) string {
	r := rand.New(rand.NewSource(seed))
	var b strings.Builder
	fmt.Fprintf(&b, "// Generated by internal/stdtest TestUiFuzz (seed %d, %d cases of each part): views rendered as Go's html.EscapeString and std/ui's URL rule give, schedules replayed in Go.\n\n", seed, n)
	b.WriteString("use std/ui\n\n")
	b.WriteString(`fn fz_app() -> ui.App[List[Int], Int] {
  ui.app([], fn(m: List[Int], e: Int) -> List[Int] { var n = m.clone(); n.push(e); n }, fn(_m) { ui.text("") })
}

fn fz_join(m: List[Int]) -> Str {
  m.map(fn(x) { "${x}" }).join(",")
}

fn fz_sync(var s: ui.Server[List[Int], Int, ()], var r: ui.Replica[List[Int], Int], lost: Bool) {
  let d = s.receive(r.outbox())
  if !lost {
    r.receive(d)
  }
}

/// Runs one schedule; "" when every check holds, else what failed.
fn fz_run(clients: Int, batch: Int, ops: List[(Int, Int, Int)], want: Str) -> Str {
  let app = fz_app()
  var s = ui.server(app, max_batch: batch)
  var r0 = ui.replica(app, "c0", [], 0)
  var r1 = ui.replica(app, "c1", [], 0)
  var r2 = ui.replica(app, "c2", [], 0)
  var reloads = 0
  for (c, op, arg) in ops {
    match op {
      0 => match c { 0 => r0.send(arg); 1 => r1.send(arg); _ => r2.send(arg) }
      1 | 2 => match c { 0 => fz_sync(s, r0, op == 2); 1 => fz_sync(s, r1, op == 2); _ => fz_sync(s, r2, op == 2) }
      3 => {
        reloads += 1
        let r = ui.replica(app, "c${c}r${reloads}", [], 0)
        match c { 0 => r0 = r; 1 => r1 = r; _ => r2 = r }
      }
      _ => s.compact(arg)
    }
    if r0.version() > s.version() || r1.version() > s.version() || r2.version() > s.version() {
      return "a replica is past the server"
    }
  }
  var rounds = 0
  while r0.pending() > 0 || r0.version() < s.version() {
    fz_sync(s, r0, false)
    rounds += 1
  }
  while clients > 1 && (r1.pending() > 0 || r1.version() < s.version()) {
    fz_sync(s, r1, false)
    rounds += 1
  }
  while clients > 2 && (r2.pending() > 0 || r2.version() < s.version()) {
    fz_sync(s, r2, false)
    rounds += 1
  }
  // the first drains may leave earlier clients behind the later ones' Msgs
  while r0.version() < s.version() || (clients > 1 && r1.version() < s.version()) {
    fz_sync(s, r0, false)
    fz_sync(s, r1, false)
    rounds += 1
    if rounds > 100000 {
      return "no convergence"
    }
  }
  let got = fz_join(s.model())
  if got != want {
    return "server model ${got.quote()}"
  }
  for (i, r) in [r0, r1, r2].indexed() {
    if i < clients && (fz_join(r.model()) != want || fz_join(r.confirmed()) != want) {
      return "replica ${i} model ${fz_join(r.model()).quote()} confirmed ${fz_join(r.confirmed()).quote()}"
    }
  }
  ""
}
`)
	var views, wants []string
	for i := 0; i < n; i++ {
		e, h := uiFuzzView(t, r, 1+r.Intn(3))
		views = append(views, e)
		wants = append(wants, h)
	}
	for lo := 0; lo < n; lo += 100 {
		hi := min(lo+100, n)
		fmt.Fprintf(&b, "\ntest \"ui.render escapes as Go's html.EscapeString and refuses unsafe URLs, cases %d to %d\" {\n", lo, hi-1)
		for i := lo; i < hi; i++ {
			fmt.Fprintf(&b, "  expect(\"%d \" + ui.render(%s)) == %s\n", i, views[i], xoStr(fmt.Sprintf("%d ", i)+wants[i]))
		}
		b.WriteString("}\n")
	}
	// views nested to the hostile depths: render must not exhaust its stack
	for _, d := range hostile.Depths() {
		fmt.Fprintf(&b, `
test "ui.render of a view nested %d deep" {
  var v = ui.text("x")
  var i = 0
  while i < %d {
    v = ui.el("b", kids: [v])
    i += 1
  }
  expect(ui.render(v) == %s) == true
}
`, d, d, xoStr(hostile.Nest("<b>", "", d)+"x"+hostile.Nest("", "</b>", d)))
	}
	b.WriteString("\n/// (clients, max_batch, schedule, the server's final model as Go replays it)\nconst SCHEDULES: List[(Int, Int, List[(Int, Int, Int)], Str)] = [\n")
	for i := 0; i < n; i++ {
		clients := 2 + r.Intn(2)
		ops, want := uiFuzzSchedule(r, clients)
		fmt.Fprintf(&b, "  (%d, %d, %s, %s),\n", clients, 1+r.Intn(5), ops, xoStr(want))
	}
	b.WriteString("]\n")
	for lo := 0; lo < n; lo += 100 {
		hi := min(lo+100, n)
		fmt.Fprintf(&b, `
test "replicas converge on the model Go replays, every Msg once, cases %d to %d" {
  var i = 0
  for (clients, batch, ops, want) in SCHEDULES {
    if i >= %d && i < %d {
      expect("${i} " + fz_run(clients, batch, ops, want)) == "${i} "
    }
    i += 1
  }
}
`, lo, hi-1, lo, hi)
	}
	return b.String()
}

func TestUiFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the ui fuzz builds and runs a generated test file per backend")
	}
	seed := envInt(t, "XO_UIFUZZ_SEED", 1)
	n := envInt(t, "XO_UIFUZZ_N", 2000)
	runFuzzDir(t, "uifuzz_test.xo", uiFuzz(t, int64(seed), n), envBackends("XO_UIFUZZ_BACKENDS", []string{"arm64", "llvm"}), 2*n)
}
