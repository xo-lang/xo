package llvm

import (
	"bytes"
	"strings"
	"testing"
)

// SplitTestIR is splitIR for the external tests.
const SplitTestIR = splitIR

// splitIR is a program in the shape Generate writes: header, constants
// (one referring to a function), helpers, and functions whose names hold
// quotes, parentheses, and commas.
const splitIR = `; generated (a comment with @"x.not_a_ref" and "quotes)
%xo.str = type { ptr, i64 }
%"s.(A, B)" = type { i64, i64 }

declare i32 @printf(ptr, ...)

@.s.0 = private unnamed_addr constant [12 x i8] c"total @%ld\0A\00"
@.fp = private constant { ptr } { ptr @"x.twice" }

define internal i64 @"xo.dup.(A, B)"(%"s.(A, B)" %x) #4 {
  %a = extractvalue %"s.(A, B)" %x, 0
  %b = call i64 @"xo.eq.Int"(i64 %a, i64 %a)
  ret i64 %b
}

define internal i64 @"xo.eq.Int"(i64 %a, i64 %b) #4 {
  %c = icmp eq i64 %a, %b
  %r = zext i1 %c to i64
  ret i64 %r
}

define internal i64 @"x.twice"(i64 %v0) #1 personality ptr @xo_personality {
entry:
  ; a comment naming @"x.nothing"
  %r = mul i64 %v0, 2
  ret i64 %r
}

define internal i64 @"x.sum((A, B))"(%"s.(A, B)" %v0, ptr noalias readonly %p1) #1 {
entry:
  %a = extractvalue %"s.(A, B)" %v0, 0
  %b = extractvalue %"s.(A, B)" %v0, 1
  %d = call i64 @"xo.dup.(A, B)"(%"s.(A, B)" %v0)
  %fp = load ptr, ptr @.fp
  %t = call i64 %fp(i64 %a)
  %s = add i64 %t, %b
  %s2 = add i64 %s, %d
  ret i64 %s2
}

define internal i64 @"x.unused_elsewhere"(i64 %v0) #1 {
  ret i64 %v0
}

define i32 @xo_personality(...) #1 {
  ret i32 0
}

define i32 @main() #1 {
entry:
  %p = insertvalue %"s.(A, B)" zeroinitializer, i64 20, 0
  %q = insertvalue %"s.(A, B)" %p, i64 1, 1
  %s = call i64 @"x.sum((A, B))"(%"s.(A, B)" %q, ptr null)
  %u = call i64 @"x.twice"(i64 %s)
  %k = call i64 @"x.unused_elsewhere"(i64 %u)
  call i32 (ptr, ...) @printf(ptr @.s.0, i64 %k)
  ret i32 0
}

attributes #1 = { uwtable }
attributes #4 = { nounwind alwaysinline }
`

func TestSplitShape(t *testing.T) {
	if got := Split([]byte(splitIR), 1); len(got) != 1 || string(got[0]) != splitIR {
		t.Fatalf("Split(ir, 1) changed the IR")
	}
	mods := Split([]byte(splitIR), 3)
	if len(mods) != 3 {
		t.Fatalf("got %d modules, want 3", len(mods))
	}
	defs := map[string]int{}
	for i, m := range mods {
		s := string(m)
		for _, want := range []string{"%xo.str = type", "declare i32 @printf", "attributes #4"} {
			if !strings.Contains(s, want) {
				t.Errorf("module %d lacks the header line %q", i, want)
			}
		}
		for _, line := range strings.Split(s, "\n") {
			if strings.HasPrefix(line, "define ") && !strings.Contains(line, `@"xo.`) {
				defs[defName([]byte(line))]++
			}
		}
		if strings.Contains(s, `"x.sum((A, B))"(`) && strings.Contains(s, `define internal i64 @"x.sum`) {
			if !strings.Contains(s, `define internal i64 @"xo.dup.(A, B)"`) || !strings.Contains(s, `define internal i64 @"xo.eq.Int"`) {
				t.Errorf("module %d defines x.sum without its helpers:\n%s", i, s)
			}
			if !strings.Contains(s, "@.fp = private") {
				t.Errorf("module %d defines x.sum without its constant", i)
			}
		}
		// names only mentioned in comments are not references
		if strings.Contains(s, `declare hidden i64 @"x.nothing"`) || strings.Contains(s, `@"x.not_a_ref"(`) {
			t.Errorf("module %d declares a name from a comment", i)
		}
	}
	for _, n := range []string{`@"x.twice"`, `@"x.sum((A, B))"`, `@"x.unused_elsewhere"`, "@main"} {
		if defs[n] != 1 {
			t.Errorf("%s defined %d times, want once", n, defs[n])
		}
	}
	// the same IR splits into the same bytes
	again := Split([]byte(splitIR), 3)
	for i := range mods {
		if !bytes.Equal(mods[i], again[i]) {
			t.Errorf("module %d differs between two splits", i)
		}
	}
}
