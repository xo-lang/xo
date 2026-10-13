package arm64

import (
	"fmt"
	"strings"
)

// Translation of the backend's assembly text (GNU syntax, the subset
// instruction selection and emission produce) to Go assembler syntax
// (Plan 9 style, `go doc cmd/asm`), for Go mode (goasm.go). The operand
// order is reversed (the destination last), registers are R0 to R30, ZR,
// RSP, and F0 to F31, memory is `off(Rn)` or `(Rn)(Rm<<k)`, immediates
// are `$n`. TestGoSyntaxEncoding assembles both forms and compares the
// machine code.

// goCond maps condition codes to Go's names.
var goCond = map[string]string{"eq": "EQ", "ne": "NE", "lt": "LT", "ge": "GE", "gt": "GT", "le": "LE",
	"lo": "LO", "hs": "HS", "hi": "HI", "ls": "LS", "mi": "MI", "pl": "PL", "vs": "VS", "vc": "VC"}

// goText translates a function's assembly text line by line.
func goText(text string) (string, error) {
	var b strings.Builder
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		switch {
		case t == "":
			continue
		case strings.HasPrefix(t, "//"):
			b.WriteString(t + "\n")
			continue
		case strings.HasSuffix(t, ":") && !strings.ContainsAny(t, " \t,"):
			b.WriteString(t + "\n")
			continue
		}
		g, err := goInstr(t)
		if err != nil {
			return "", err
		}
		b.WriteString("\t" + g + "\n")
	}
	return b.String(), nil
}

// splitOps splits operands at commas outside brackets.
func splitOps(s string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if t := strings.TrimSpace(s[start:]); t != "" {
		out = append(out, t)
	}
	return out
}

// goReg translates a register name; w reports a 32 bit name.
func goReg(r string) (name string, w, fp, ok bool) {
	switch r {
	case "xzr":
		return "ZR", false, false, true
	case "wzr":
		return "ZR", true, false, true
	case "sp":
		return "RSP", false, false, true
	case "x28":
		// the goroutine (Go mode's preemption polls read g.stackguard0)
		return "g", false, false, true
	}
	if len(r) < 2 {
		return "", false, false, false
	}
	n := r[1:]
	for _, c := range n {
		if c < '0' || c > '9' {
			return "", false, false, false
		}
	}
	switch r[0] {
	case 'x':
		return "R" + n, false, false, true
	case 'w':
		return "R" + n, true, false, true
	case 'd':
		return "F" + n, false, true, true
	}
	return "", false, false, false
}

func goImm(s string) (string, bool) {
	if !strings.HasPrefix(s, "#") {
		return "", false
	}
	v := s[1:]
	if v == "0.0" {
		return "$(0.0)", true
	}
	return "$" + v, true
}

var goShift = map[string]string{"lsl": "<<", "lsr": ">>", "asr": "->"}

// goOperand translates one value operand (a register, an immediate, or
// a register with a shift given by the next operand).
func goOperand(ops []string, i int) (string, int, error) {
	o := ops[i]
	if v, ok := goImm(o); ok {
		return v, i + 1, nil
	}
	r, _, _, ok := goReg(o)
	if !ok {
		return "", 0, fmt.Errorf("operand %q", o)
	}
	if i+1 < len(ops) {
		f := strings.Fields(ops[i+1])
		if len(f) == 2 && goShift[f[0]] != "" && strings.HasPrefix(f[1], "#") {
			return r + goShift[f[0]] + f[1][1:], i + 2, nil
		}
	}
	return r, i + 1, nil
}

// goMem translates a memory operand.
func goMem(o string) (string, error) {
	if strings.HasPrefix(o, "@") {
		return o[1:], nil // already Go syntax: name+off(FP)
	}
	if !strings.HasPrefix(o, "[") || !strings.HasSuffix(o, "]") {
		return "", fmt.Errorf("memory operand %q", o)
	}
	parts := splitOps(o[1 : len(o)-1])
	base, _, _, ok := goReg(parts[0])
	if !ok {
		return "", fmt.Errorf("memory base %q", o)
	}
	switch len(parts) {
	case 1:
		return "(" + base + ")", nil
	case 2:
		if v, ok := goImm(parts[1]); ok {
			off := v[1:]
			if off == "0" {
				return "(" + base + ")", nil
			}
			return off + "(" + base + ")", nil
		}
		idx, _, _, ok := goReg(parts[1])
		if !ok {
			return "", fmt.Errorf("memory index %q", o)
		}
		return "(" + base + ")(" + idx + ")", nil
	case 3:
		idx, _, _, ok := goReg(parts[1])
		f := strings.Fields(parts[2])
		if !ok || len(f) != 2 || f[0] != "lsl" {
			return "", fmt.Errorf("memory index %q", o)
		}
		return "(" + base + ")(" + idx + "<<" + strings.TrimPrefix(f[1], "#") + ")", nil
	}
	return "", fmt.Errorf("memory operand %q", o)
}

// goInstr translates one instruction.
func goInstr(t string) (string, error) {
	mn, rest, _ := strings.Cut(t, " ")
	ops := splitOps(rest)
	bad := func() (string, error) { return "", fmt.Errorf("go asm: cannot translate %q", t) }
	reg := func(i int) string {
		r, _, _, ok := goReg(ops[i])
		if !ok {
			return "?"
		}
		return r
	}
	isW := func(i int) bool {
		_, w, _, _ := goReg(ops[i])
		return w
	}
	isFP := func(i int) bool {
		_, _, fp, _ := goReg(ops[i])
		return fp
	}
	cond := func(i int) string {
		if c, ok := goCond[ops[i]]; ok {
			return c
		}
		return "?"
	}
	var out string
	switch mn {
	case "ret":
		out = "RET"
	case "brk":
		out = "UNDEF"
	case "b":
		out = "JMP " + ops[0]
	case "bl":
		out = "CALL " + ops[0]
	case "cbz", "cbnz":
		m := strings.ToUpper(mn)
		if isW(0) {
			m += "W"
		}
		out = fmt.Sprintf("%s %s, %s", m, reg(0), ops[1])
	case "tbz", "tbnz":
		v, _ := goImm(ops[1])
		out = fmt.Sprintf("%s %s, %s, %s", strings.ToUpper(mn), v, reg(0), ops[2])
	case "add", "sub", "and", "orr", "eor", "mul", "sdiv", "smulh", "umulh", "adds", "subs", "lsl", "lsr", "asr":
		if len(ops) < 3 {
			return bad()
		}
		src, n, err := goOperand(ops, 2)
		if err != nil || n != len(ops) {
			return bad()
		}
		m := strings.ToUpper(mn)
		if isW(0) {
			return bad()
		}
		out = fmt.Sprintf("%s %s, %s, %s", m, src, reg(1), reg(0))
	case "madd", "msub":
		// d = a +- n*m
		out = fmt.Sprintf("%s %s, %s, %s, %s", strings.ToUpper(mn), reg(2), reg(3), reg(1), reg(0))
	case "neg", "negs", "mvn":
		src, n, err := goOperand(ops, 1)
		if err != nil || n != len(ops) {
			return bad()
		}
		out = fmt.Sprintf("%s %s, %s", strings.ToUpper(mn), src, reg(0))
	case "mov":
		src, n, err := goOperand(ops, 1)
		if err != nil || n != len(ops) || isW(0) {
			return bad()
		}
		out = fmt.Sprintf("MOVD %s, %s", src, reg(0))
	case "movz", "movk", "movn":
		v, ok := goImm(ops[1])
		if !ok {
			return bad()
		}
		sh := "0"
		if len(ops) == 3 {
			f := strings.Fields(ops[2])
			if len(f) != 2 || f[0] != "lsl" {
				return bad()
			}
			sh = strings.TrimPrefix(f[1], "#")
		}
		out = fmt.Sprintf("%s $(%s<<%s), %s", strings.ToUpper(mn), v[1:], sh, reg(0))
	case "cmp", "cmn", "tst":
		src, n, err := goOperand(ops, 1)
		if err != nil || n != len(ops) {
			return bad()
		}
		m := strings.ToUpper(mn)
		if isW(0) {
			m += "W"
		}
		out = fmt.Sprintf("%s %s, %s", m, src, reg(0))
	case "csel":
		out = fmt.Sprintf("CSEL %s, %s, %s, %s", cond(3), reg(1), reg(2), reg(0))
	case "fcsel":
		out = fmt.Sprintf("FCSELD %s, %s, %s, %s", cond(3), reg(1), reg(2), reg(0))
	case "cset":
		out = fmt.Sprintf("CSET %s, %s", cond(1), reg(0))
	case "cneg":
		out = fmt.Sprintf("CNEG %s, %s, %s", cond(2), reg(1), reg(0))
	case "ccmn", "ccmp":
		a, _ := goImm(ops[2])
		src, _, err := goOperand(ops, 1)
		if err != nil || a == "" {
			return bad()
		}
		out = fmt.Sprintf("%s %s, %s, %s, %s", strings.ToUpper(mn), cond(3), reg(0), src, a)
	case "fccmp":
		a, _ := goImm(ops[2])
		out = fmt.Sprintf("FCCMPD %s, %s, %s, %s", cond(3), reg(1), reg(0), a)
	case "fadd", "fsub", "fmul", "fdiv":
		out = fmt.Sprintf("%sD %s, %s, %s", strings.ToUpper(mn), reg(2), reg(1), reg(0))
	case "fneg", "fabs", "fsqrt", "frintz", "frintm", "frintp", "frinta":
		out = fmt.Sprintf("%sD %s, %s", strings.ToUpper(mn), reg(1), reg(0))
	case "fcmp":
		src, _, err := goOperand(ops, 1)
		if err != nil {
			return bad()
		}
		out = fmt.Sprintf("FCMPD %s, %s", src, reg(0))
	case "fmov":
		if !isFP(0) && !isFP(1) {
			return bad()
		}
		out = fmt.Sprintf("FMOVD %s, %s", reg(1), reg(0))
	case "scvtf":
		out = fmt.Sprintf("SCVTFD %s, %s", reg(1), reg(0))
	case "fcvtzs":
		out = fmt.Sprintf("FCVTZSD %s, %s", reg(1), reg(0))
	case "ldr", "str", "ldrb", "strb":
		if len(ops) != 2 {
			return bad()
		}
		mem, err := goMem(ops[1])
		if err != nil {
			return bad()
		}
		var m string
		switch {
		case mn == "ldrb" && isW(0):
			m = "MOVBU"
		case mn == "strb" && isW(0):
			m = "MOVB"
		case (mn == "ldr" || mn == "str") && isFP(0):
			m = "FMOVD"
		case (mn == "ldr" || mn == "str") && !isW(0):
			m = "MOVD"
		default:
			return bad()
		}
		if mn[0] == 'l' {
			out = fmt.Sprintf("%s %s, %s", m, mem, reg(0))
		} else {
			out = fmt.Sprintf("%s %s, %s", m, reg(0), mem)
		}
	case "ldp", "stp":
		if len(ops) != 3 || isW(0) || isFP(0) != isFP(1) {
			return bad()
		}
		mem, err := goMem(ops[2])
		if err != nil {
			return bad()
		}
		m := strings.ToUpper(mn)
		if isFP(0) {
			m = "F" + m + "D"
		}
		if mn == "ldp" {
			out = fmt.Sprintf("%s %s, (%s, %s)", m, mem, reg(0), reg(1))
		} else {
			out = fmt.Sprintf("%s (%s, %s), %s", m, reg(0), reg(1), mem)
		}
	default:
		if strings.HasPrefix(mn, "b.") {
			c, ok := goCond[mn[2:]]
			if !ok {
				return bad()
			}
			out = fmt.Sprintf("B%s %s", c, ops[0])
			break
		}
		return bad()
	}
	if strings.Contains(out, "?") {
		return bad()
	}
	return out, nil
}
