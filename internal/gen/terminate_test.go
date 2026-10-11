package gen

import "testing"

func TestTerminates(t *testing.T) {
	for _, c := range []struct {
		body string
		want bool
	}{
		{"return 1", true},
		{"panic(\"x\")", true},
		{"x := 1\n_ = x", false},
		{"if a {\nreturn 1\n} else {\nreturn 2\n}", true},
		{"if a {\nreturn 1\n}", false},
		{"if a {\nreturn 1\n} else if b {\nreturn 2\n} else {\npanic(0)\n}", true},
		{"if a {\nreturn 1\n} else if b {\nreturn 2\n}", false},
		{"for {\n}", true},
		{"for {\nbreak\n}", false},
		{"for {\nswitch {\ndefault:\nbreak\n}\n}", true},
		{"L:\nfor {\nswitch {\ndefault:\nbreak L\n}\n}", false},
		{"for {\nf := func() {\nfor {\n}\n}\n_ = f\n}", true},
		{"switch x {\ncase 1:\nreturn 1\ndefault:\nreturn 2\n}", true},
		{"switch x {\ncase 1:\nreturn 1\n}", false},
		{"switch x {\ncase 1:\nfallthrough\ndefault:\nreturn 2\n}", true},
		{"L:\nswitch {\ndefault:\nreturn 1\nbreak L\n}", false},
		{"select {\ncase <-c:\nreturn 1\n}", true},
		{"//line main.xo:3:1\nif a {\n//line main.xo:4:1\nreturn 1\n} else {\nreturn 2\n}\n", true},
		{"{ not go", false},
	} {
		if got := terminates(c.body); got != c.want {
			t.Errorf("terminates(%q) = %v, want %v", c.body, got, c.want)
		}
	}
}

func TestNeverFallsThrough(t *testing.T) {
	for _, c := range []struct {
		body string
		want bool
	}{
		{"return 1", true},
		{"continue L", true},
		{"break", true},
		{"{\nx := 1\n_ = x\ncontinue L\n}", true},
		{"if a {\ncontinue\n} else {\nbreak L\n}", true},
		{"if a {\ncontinue\n}", false},
		{"switch {\ndefault:\nif a {\nreturn 1\n}\nbreak\n}", false},
		{"switch {\ndefault:\nif a {\ncontinue L\n}\npanic(0)\n}", true},
		{"for {\nbreak L\n}", true},
		{"for {\nbreak\n}", false},
		{"x := 1\n_ = x", false},
	} {
		if got := neverFallsThrough(c.body); got != c.want {
			t.Errorf("neverFallsThrough(%q) = %v, want %v", c.body, got, c.want)
		}
	}
	// the spec's rule, for a missing return, does not count break and
	// continue
	if terminates("continue L") || terminates("{\nbreak\n}") {
		t.Error("terminates counts break or continue")
	}
}
