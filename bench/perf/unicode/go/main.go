// Go twin of bench/perf/unicode: the same workloads with Go's unicode,
// strings, and strconv packages, so a timing row can compare the native
// Xo runtime's Unicode operations with Go's. Usage: unicodeperfgo
// <workload>; each prints "<workload> <checksum> <elapsed microseconds>"
// and the checksum must equal the Xo program's.
package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	ascii = "The Quick Brown Fox Jumps Over The Lazy Dog; HTTP/1.1 200 OK, Content-Type: text/plain. 0123456789 mixed CASE words\n"
	mixed = "Ελληνικά ΚΕΙΜΕΝΟ Кириллица ТЕКСТ Ünïcödé ÀÉÎÕÜ straße İstanbul ǅemal ﬁne 日本語 ﾃｷｽﾄ Ⅻ ⓐⓑ 𐐀𐐨 ascii Words\n"
	reps  = 200
	mod   = 1000000007
)

// sample is a checksum of s: its length and every 61st byte.
func sample(s string) int64 {
	h := int64(len(s))
	for i := 0; i < len(s); i += 61 {
		h = (h*31 + int64(s[i])) % mod
	}
	return h
}

func lower(s string) int64 {
	var sum int64
	for i := 0; i < reps; i++ {
		sum = (sum + sample(strings.ToLower(s))) % mod
	}
	return sum
}

func upper(s string) int64 {
	var sum int64
	for i := 0; i < reps; i++ {
		sum = (sum + sample(strings.ToUpper(s))) % mod
	}
	return sum
}

func eqLower(a, b string) int64 {
	var n int64
	for i := 0; i < reps; i++ {
		if strings.ToLower(a) == strings.ToLower(b) {
			n += int64(i)
		}
	}
	return n
}

func classify(rs []rune) int64 {
	var sum int64
	for k := 0; k < 10; k++ {
		for _, r := range rs {
			if unicode.IsLetter(r) {
				sum += 1
			}
			if unicode.IsDigit(r) {
				sum += 3
			}
			if unicode.IsSpace(r) {
				sum += 7
			}
			if strconv.IsPrint(r) {
				sum += 11
			}
		}
	}
	return sum
}

func runeCase(rs []rune) int64 {
	var sum int64
	for k := 0; k < 10; k++ {
		for _, r := range rs {
			sum = (sum + int64(unicode.ToLower(r)) + 3*int64(unicode.ToUpper(r))) % mod
		}
	}
	return sum
}

// classBits is the weighted sum of every Rune class, in the order and
// with the weights of the Xo program.
func classBits(r rune) int64 {
	var n int64
	if unicode.IsLetter(r) {
		n += 1
	}
	if unicode.IsDigit(r) {
		n += 3
	}
	if unicode.IsSpace(r) {
		n += 7
	}
	if strconv.IsPrint(r) {
		n += 11
	}
	if unicode.IsUpper(r) {
		n += 13
	}
	if unicode.IsLower(r) {
		n += 17
	}
	if unicode.IsTitle(r) {
		n += 19
	}
	if unicode.IsPunct(r) {
		n += 23
	}
	if unicode.IsControl(r) {
		n += 29
	}
	if unicode.IsGraphic(r) {
		n += 31
	}
	if unicode.IsMark(r) {
		n += 37
	}
	if unicode.IsSymbol(r) {
		n += 41
	}
	if unicode.IsNumber(r) {
		n += 43
	}
	return n
}

func classes(rs []rune) int64 {
	var sum int64
	for k := 0; k < 10; k++ {
		for _, r := range rs {
			sum += classBits(r)
		}
	}
	return sum
}

func latin1() int64 {
	var sum int64
	for k := 0; k < reps; k++ {
		for i := 0; i < 256; i++ {
			sum += classBits(rune(i))
		}
	}
	return sum
}

func titleSum(rs []rune) int64 {
	var sum int64
	for k := 0; k < 10; k++ {
		for _, r := range rs {
			sum = (sum + int64(unicode.ToTitle(r))) % mod
		}
	}
	return sum
}

func utf8LenSum(rs []rune) int64 {
	var sum int64
	for k := 0; k < 10; k++ {
		for _, r := range rs {
			sum += int64(utf8.RuneLen(r))
		}
	}
	return sum
}

func quote(s string) int64 {
	var sum int64
	for i := 0; i < 20; i++ {
		sum = (sum + sample(strconv.Quote(s))) % mod
	}
	return sum
}

// The predicate forms of Str (Go's *Func family), the same workloads as
// the Xo program.

func marker(r rune) bool { return r == 0x10ffff }

func indexFunc(a, b string) int64 {
	var sum int64
	for i := 0; i < reps; i++ {
		sum = (sum + int64(strings.IndexFunc(a, marker))) % mod
		sum = (sum + int64(strings.LastIndexFunc(b, marker))) % mod
		if strings.ContainsFunc(a, marker) {
			sum++
		}
	}
	return sum
}

func trimFunc(s string) int64 {
	space := func(r rune) bool { return unicode.IsSpace(r) }
	asciiSpace := func(r rune) bool { return r == ' ' }
	var sum int64
	for i := 0; i < reps; i++ {
		sum = (sum + sample(strings.TrimFunc(s, space))) % mod
		sum = (sum + int64(len(strings.TrimLeftFunc(s, space)))) % mod
		sum = (sum + int64(len(strings.TrimRightFunc(s, asciiSpace)))) % mod
	}
	return sum
}

func fieldsFunc(s string) int64 {
	space := func(r rune) bool { return unicode.IsSpace(r) }
	var sum int64
	for i := 0; i < reps; i++ {
		ps := strings.FieldsFunc(s, space)
		sum = (sum + int64(len(ps))) % mod
		for _, p := range ps {
			sum = (sum + int64(len(p))) % mod
		}
	}
	return sum
}

func mapFunc(s string) int64 {
	up := func(r rune) rune { return unicode.ToUpper(r) }
	dropDigits := func(r rune) rune {
		if unicode.IsDigit(r) {
			return -1
		}
		return r
	}
	var sum int64
	for i := 0; i < reps; i++ {
		sum = (sum + sample(strings.Map(up, s))) % mod
		sum = (sum + int64(len(strings.Map(dropDigits, s)))) % mod
	}
	return sum
}

func main() {
	w := "lower"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	a := strings.Repeat(ascii, 4000)
	u := strings.Repeat(mixed, 2000)
	rs := []rune(strings.Repeat(ascii+mixed, 1000))
	t := time.Now()
	var sum int64
	switch w {
	case "lower":
		sum = lower(a)
	case "upper":
		sum = upper(a)
	case "lower_u":
		sum = lower(u)
	case "upper_u":
		sum = upper(u)
	case "eqlower":
		sum = eqLower(a, strings.ToUpper(a)) + eqLower(u, strings.ToUpper(u))
	case "classify":
		sum = classify(rs)
	case "runecase":
		sum = runeCase(rs)
	case "classes":
		sum = classes(rs)
	case "latin1":
		sum = latin1()
	case "title":
		sum = titleSum(rs)
	case "utf8len":
		sum = utf8LenSum(rs)
	case "quote":
		sum = quote(strings.Repeat(ascii+mixed+"\t\u0007\u0000", 400))
	case "indexfunc":
		sum = indexFunc(strings.Repeat(ascii+mixed, 2000)+"\U0010FFFF", "\U0010FFFF"+strings.Repeat(ascii+mixed, 2000))
	case "trimfunc":
		sum = trimFunc(strings.Repeat(" ", 2000) + ascii + strings.Repeat(" ", 2000))
	case "fieldsfunc":
		sum = fieldsFunc(strings.Repeat(ascii, 200))
	case "mapfunc":
		sum = mapFunc(strings.Repeat(ascii+mixed, 500))
	default:
		sum = -1
	}
	us := time.Since(t).Microseconds()
	fmt.Printf("%s %d %d\n", w, sum, us)
}
