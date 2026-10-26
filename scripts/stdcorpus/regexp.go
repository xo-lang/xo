package main

import (
	"errors"
	"fmt"
	"math/rand"
	"regexp"
	"regexp/syntax"
	"strconv"
	"strings"
)

// regexpPatterns are the corpus's patterns: curated ones (anchors, classes,
// repeats, alternation, groups, flags, folding, empty matches, bad syntax)
// and seeded random ones built from a small grammar.
func regexpPatterns(r *rand.Rand) []string {
	ps := []string{
		`a`, `abc`, `a|b`, `a+`, `a*`, `a?`, `a*?`, `a+?`, `a??`, `a{2}`, `a{2,}`, `a{1,3}`, `a{0}`, `(a)`, `(a)(b)`, `(a|b)*c`,
		`(?:ab)+`, `^abc`, `abc$`, `^$`, `^`, `$`, `\b`, `\B`, `\bfoo\b`, `.`, `.+`, `(?s).+`, `(?i)abc`, `(?i)ǅ`, `[a-c]+`, `[^a-c]+`,
		`[[:alpha:]]+`, `[[:^digit:]]+`, `\d+`, `\D+`, `\w+`, `\W+`, `\s+`, `\S+`, `\pL+`, `\p{Greek}+`, `\PL+`, `(?m)^b$`, `(?m)^`, `(?U)a+`,
		`(?P<x>a)(?P<y>b)?`, `(a*)*`, `(a*)+`, `(a|ab)(c|bcd)(d*)`, `x*`, `\.`, `\Qa.b\E`, `[\d\-]+`, `[a-z]+@[a-z]+\.com`, `(\d{4})-(\d{2})-(\d{2})`,
		`(\w+)=(\w+)`, `[é日]+`, `é+`, `日本`, `(?:a|b|c)+d`, `a.c`, `^(a+)+$`, `(a)|(b)|(c)`, `\x41`, `\x{65e5}`, `\n`, `[\n]`, `\z`, `\A`,
		`(`, `a)`, `[a`, `*a`, `a**`, `a{2,1}`, `(?P<n>a)(?P<n>b)`, `\8`, `(?z)`, `[z-a]`, `\p{Foo}`, `a{1001}`, ``,
	}
	atoms := []string{"a", "b", "c", ".", "[ab]", "[^a]", `\d`, `\w`, `\s`, "x", "é", `\b`, "^", "$"}
	for len(ps) < 260 {
		var b strings.Builder
		for i, n := 0, 1+r.Intn(4); i < n; i++ {
			a := atoms[r.Intn(len(atoms))]
			switch r.Intn(8) {
			case 0:
				a = "(" + a + ")"
			case 1:
				a = "(?:" + a + "|" + atoms[r.Intn(len(atoms))] + ")"
			case 2:
				a = "(" + a + atoms[r.Intn(len(atoms))] + ")"
			}
			b.WriteString(a)
			b.WriteString([]string{"", "", "", "*", "+", "?", "*?", "+?", "{2}", "{1,2}"}[r.Intn(10)])
		}
		p := b.String()
		if r.Intn(6) == 0 {
			p = "(?i)" + p
		}
		ps = append(ps, p)
	}
	return ps
}

func regexpInputs(r *rand.Rand) []string {
	in := []string{"", "a", "abc", "aaa", "ab", "abcabc", "foo bar", "xfoo foo foox", "a\nb\nc", "2026-10-21", "k=v one=two", "bob@example.com x", "é日本", "ǆ", "aXbXc"}
	alpha := []string{"a", "b", "c", "x", "A", "B", " ", "\n", "1", "2", "é", "日", "_", "-", "@", ".", "=", "ǅ"}
	// a long text: the Pike VM, which the backtracker leaves to big inputs
	in = append(in, strings.Repeat("abc xfoo 2026-10-21 k=v \u00e9\n", 2000))
	for len(in) < 40 {
		var b strings.Builder
		for i, n := 0, r.Intn(10); i < n; i++ {
			b.WriteString(alpha[r.Intn(len(alpha))])
		}
		in = append(in, b.String())
	}
	return in
}

func regexpResult(re *regexp.Regexp, s string) string {
	var parts []string
	if loc := re.FindStringIndex(s); loc != nil {
		parts = append(parts, fmt.Sprintf("%d,%d", loc[0], loc[1]))
	} else {
		parts = append(parts, "-")
	}
	if len(s) > 10000 {
		parts = append(parts, "")
	} else {
		parts = append(parts, strings.Join(re.FindAllString(s, -1), "\x01"))
	}
	if m := re.FindStringSubmatchIndex(s); m != nil {
		var cs []string
		for i := 0; i+1 < len(m); i += 2 {
			if m[i] < 0 {
				cs = append(cs, "\x02")
			} else {
				cs = append(cs, s[m[i]:m[i+1]])
			}
		}
		parts = append(parts, strings.Join(cs, "\x01"))
	} else {
		parts = append(parts, "-")
	}
	if len(s) > 10000 {
		// long text: the counts only
		parts = append(parts, fmt.Sprint(len(re.FindAllString(s, -1))))
		return strings.Join(parts, "\x03")
	}
	parts = append(parts, re.ReplaceAllString(s, "<$1>"), re.ReplaceAllLiteralString(s, "<$1>"), strings.Join(re.Split(s, -1), "\x01"))
	return strings.Join(parts, "\x03")
}

func regexpCorpus() string {
	r := rand.New(rand.NewSource(21))
	var o strings.Builder
	o.WriteString(header("regexp's Compile, MatchString, FindStringIndex, FindAllString, FindStringSubmatchIndex, ReplaceAllString, ReplaceAllLiteralString and Split on curated and seeded random patterns against curated and random texts"))
	o.WriteString("use std/regexp\n\n/// (pattern, whether it compiles, then per input in INPUTS the joined results)\nconst CASES: List[(Str, Bool, List[Str])] = [\n")
	inputs := regexpInputs(r)
	// errs is what a bad pattern's display must say: Xo's own wording
	// ("invalid regexp "P": CODE: `EXPR`", std/native/regexp.xo) around
	// the code and the expression Go's syntax.Error names.
	var errs []string
	for _, p := range regexpPatterns(r) {
		re, err := regexp.Compile(p)
		if err != nil {
			fmt.Fprintf(&o, "  (%s, false, []),\n", xq(p))
			var se *syntax.Error
			if errors.As(err, &se) {
				errs = append(errs, fmt.Sprintf("  (%s, %s),\n", xq(p),
					xq(fmt.Sprintf("invalid regexp %s: %s: `%s`", strconv.Quote(p), se.Code, se.Expr))))
			}
			continue
		}
		var rs []string
		for _, s := range inputs {
			rs = append(rs, xq(regexpResult(re, s)))
		}
		fmt.Fprintf(&o, "  (%s, true, [%s]),\n", xq(p), strings.Join(rs, ", "))
	}
	o.WriteString("]\n\nconst INPUTS: List[Str] = [")
	for i, s := range inputs {
		if i > 0 {
			o.WriteString(", ")
		}
		o.WriteString(xq(s))
	}
	o.WriteString("]\n\n/// (pattern, what its RegexErr displays: Xo's wording around Go's code and expression)\nconst ERRS: List[(Str, Str)] = [\n")
	for _, e := range errs {
		o.WriteString(e)
	}
	o.WriteString("]\n")
	o.WriteString(regexpTests)
	return o.String()
}

const regexpTests = `
fn joined(l: List[Str]) -> Str { l.join("\u{1}") }

fn result(re: regexp.Regex, s: Str) -> Str {
  var parts: List[Str] = []
  parts.push(match re.find_index(s) {
    Some((a, b)) => "${a},${b}"
    None => "-"
  })
  parts.push(if s.len() > 10000 { "" } else { joined(re.find_all(s)) })
  parts.push(match re.captures(s) {
    Some(cs) => joined(cs.map(fn(c) { c ?? "\u{2}" }))
    None => "-"
  })
  if s.len() > 10000 {
    parts.push("${re.find_all(s).len()}")
    return parts.join("\u{3}")
  }
  parts.push(re.replace_all(s, "<$1>"))
  parts.push(re.replace_all_literal(s, "<$1>"))
  parts.push(joined(re.split(s)))
  parts.join("\u{3}")
}

test "regexp is Go's on the corpus" {
  var bad: List[(Str, Str, Str, Str)] = []
  for (pat, ok, want) in CASES {
    match regexp.compile(pat) {
      Ok(re) => {
        if !ok {
          bad.push((pat, "compiled", "", ""))
        } else {
          for (i, s) in INPUTS.indexed() {
            let got = result(re, s)
            if got != want[i] {
              bad.push((pat, s, got, want[i]))
            }
          }
        }
      }
      Err(_) => {
        if ok {
          bad.push((pat, "did not compile", "", ""))
        }
      }
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}

test "a bad pattern displays Xo's wording around Go's code and expression" {
  var bad: List[(Str, Str, Str)] = []
  for (pat, want) in ERRS {
    let got = match regexp.compile(pat) {
      Ok(_) => "compiled"
      Err(e) => e.display()
    }
    if got != want {
      bad.push((pat, got, want))
    }
  }
  expect(bad[0..(if bad.len() < 10 { bad.len() } else { 10 })]) == []
}
`
