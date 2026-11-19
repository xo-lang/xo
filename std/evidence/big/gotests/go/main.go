// Command go is the Go side of std/big's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of package big's result as text;
// std/evidence/big/gotests/main.xo prints std/big's the same way. See
// scripts/stdproven/gotests.go.
package main

import (
	"bufio"
	"encoding/base64"
	"fmt"
	"math"
	"math/big"
	"os"
	"strconv"
	"strings"
)

func z(s string) (*big.Int, bool) {
	return new(big.Int).SetString(s, 0)
}

func q(s string) (*big.Rat, bool) {
	return new(big.Rat).SetString(s)
}

func rs(x *big.Rat) string {
	return x.String() + " " + x.RatString()
}

func ints(in []string, kind string) string {
	x, ok := z(in[0])
	if !ok {
		return "parse err " + in[0]
	}
	y := new(big.Int)
	if len(in) > 1 && in[1] != "" {
		if y, ok = z(in[1]); !ok {
			return "parse err " + in[1]
		}
	}
	return big2(x, y, kind)
}

func big2(x, y *big.Int, kind string) string {
	switch kind {
	case "quo":
		if y.Sign() == 0 {
			return "div0"
		}
		a, b := new(big.Int).QuoRem(x, y, new(big.Int))
		c, d := new(big.Int).DivMod(x, y, new(big.Int))
		return fmt.Sprintf("%s %s %s %s", a, b, c, d)
	case "bitlen":
		return fmt.Sprint(x.BitLen())
	case "gcd":
		return new(big.Int).GCD(nil, nil, x, y).String()
	case "bitwise":
		return fmt.Sprintf("%s %s %s %s", new(big.Int).And(x, y), new(big.Int).Or(x, y), new(big.Int).Xor(x, y), new(big.Int).AndNot(x, y))
	case "tzb":
		return fmt.Sprint(x.TrailingZeroBits())
	case "not":
		return new(big.Int).Not(x).String()
	case "modinv":
		r := new(big.Int).ModInverse(x, y)
		if r == nil {
			return "none"
		}
		return r.String()
	}
	return "?"
}

func exp(in []string) string {
	x, ok1 := z(in[0])
	y, ok2 := z(in[1])
	if !ok1 || !ok2 {
		return "parse err"
	}
	var m *big.Int
	if in[2] != "" {
		var ok bool
		if m, ok = z(in[2]); !ok {
			return "parse err"
		}
	}
	r := new(big.Int).Exp(x, y, m)
	if r == nil {
		return "nil"
	}
	return r.String()
}

func shift(in []string, left bool) string {
	x, ok := z(in[0])
	if !ok {
		return "parse err"
	}
	n, _ := strconv.Atoi(in[1])
	if left {
		return new(big.Int).Lsh(x, uint(n)).String()
	}
	return new(big.Int).Rsh(x, uint(n)).String()
}

func intstr(in []string) string {
	b, _ := strconv.Atoi(in[1])
	x, ok := new(big.Int).SetString(in[0], b)
	if !ok {
		return "err"
	}
	if b == 0 {
		b = 10
	}
	return "ok " + x.Text(b)
}

func ratcmp(in []string) string {
	x, ok1 := q(in[0])
	y, ok2 := q(in[1])
	if !ok1 || !ok2 {
		return "parse err"
	}
	return fmt.Sprint(x.Cmp(y))
}

func ratbin(in []string) string {
	x, ok1 := q(in[0])
	y, ok2 := q(in[1])
	if !ok1 || !ok2 {
		return "parse err"
	}
	s := rs(new(big.Rat).Add(x, y)) + "; " + rs(new(big.Rat).Mul(x, y)) + "; " + rs(new(big.Rat).Sub(x, y))
	if y.Sign() != 0 {
		s += "; " + rs(new(big.Rat).Quo(x, y))
	}
	return s
}

func ratset(in []string) string {
	x, ok := q(in[0])
	if !ok {
		return "err"
	}
	return "ok " + rs(x)
}

func mf(s string) (*big.Float, bool) {
	x, _, err := big.ParseFloat(s, 0, 1000, big.ToNearestEven)
	return x, err == nil
}

var modes = map[string]big.RoundingMode{"ToNearestEven": big.ToNearestEven, "ToNearestAway": big.ToNearestAway, "ToZero": big.ToZero, "AwayFromZero": big.AwayFromZero, "ToNegativeInf": big.ToNegativeInf, "ToPositiveInf": big.ToPositiveInf}

func floats(in []string, kind string) string {
	if kind == "fisint" {
		in = []string{strings.Fields(in[0])[0]}
	}
	if kind == "fsqrt" {
		var out []string
		for _, p := range []uint{24, 53, 64, 100, 256} {
			x := new(big.Float).SetPrec(p)
			if _, _, err := x.Parse(in[0], 10); err != nil {
				return "parse err"
			}
			out = append(out, new(big.Float).SetPrec(p).Sqrt(x).Text('p', 0))
		}
		return strings.Join(out, " ")
	}
	if kind == "fset64" {
		var x big.Float
		x.SetPrec(53)
		if _, ok := x.SetString(in[0]); !ok {
			return "err"
		}
		return fmt.Sprintf("ok %s %v", x.Text('p', 0), x.Signbit())
	}
	if kind == "ftext" || kind == "ftextr" {
		mode := big.ToNearestEven
		if kind == "ftextr" {
			mode = modes[in[1]]
			in = append([]string{in[0]}, in[2:]...)
		}
		prec, _ := strconv.Atoi(in[1])
		f, _, err := big.ParseFloat(in[0], 0, uint(prec), big.ToNearestEven)
		if err != nil {
			return "parse err"
		}
		f.SetMode(mode)
		format, _ := strconv.Atoi(in[2])
		digits, _ := strconv.Atoi(in[3])
		return f.Text(byte(format), digits)
	}
	x, ok := mf(in[0])
	if !ok {
		return "parse err"
	}
	switch kind {
	case "fminprec":
		return fmt.Sprint(x.SetPrec(100).MinPrec())
	case "fsign":
		return fmt.Sprint(x.Sign())
	case "fisint":
		return fmt.Sprint(x.IsInt())
	case "fint64":
		v, acc := x.Int64()
		return fmt.Sprintf("%d %s", v, acc)
	case "ffloat64":
		v, acc := x.Float64()
		return fmt.Sprintf("%d %s", int64(math.Float64bits(v)), acc)
	case "fint":
		v, acc := x.Int(nil)
		if v == nil {
			return "nil " + acc.String()
		}
		return v.String() + " " + acc.String()
	case "frat":
		v, acc := x.SetPrec(64).Rat(nil)
		if v == nil {
			return "nil " + acc.String()
		}
		return v.String() + " " + acc.String()
	case "fmantexp":
		m := new(big.Float)
		e := x.MantExp(m)
		return fmt.Sprintf("%s %d", m.Text('p', 0), e)
	}
	return "?"
}

func floatstr(in []string) string {
	x, ok := q(in[0])
	if !ok {
		return "err"
	}
	p, _ := strconv.Atoi(in[1])
	return x.FloatString(p)
}

func run(kind string, in []string) string {
	switch kind {
	case "quo", "bitlen", "gcd", "bitwise", "tzb", "not", "modinv":
		return ints(in, kind)
	case "exp":
		return exp(in)
	case "lsh":
		return shift(in, true)
	case "rsh":
		return shift(in, false)
	case "intstr":
		return intstr(in)
	case "ratcmp":
		return ratcmp(in)
	case "ratbin":
		return ratbin(in)
	case "ratset":
		return ratset(in)
	case "floatstr":
		return floatstr(in)
	case "fminprec", "fsign", "fisint", "fint64", "ffloat64", "fint", "frat", "fmantexp", "fsqrt", "fset64", "ftext", "ftextr":
		return floats(in, kind)
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
