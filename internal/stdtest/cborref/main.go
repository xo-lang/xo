// cborref is the oracle for the std/cbor tests (decision 0468): it reads
// one hex encoded CBOR input a line from standard input and writes one
// JSON object a line with what github.com/fxamacker/cbor/v2 (MIT) makes of
// it. It is its own module so that the main module keeps no dependency;
// the tests run it with `go run .` here, and skip when the module cannot
// be fetched.
package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"os"
	"regexp"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

type result struct {
	WF    bool   `json:"wf"`
	Diag  string `json:"diag"`  // the diagnostic notation; "ERR" when it fails, "UTF8" when text is not UTF-8, "SKIP" when an indefinite item is present
	Canon string `json:"canon"` // hex of the deterministic re-encoding, "ERR" or "SKIP"
	Dup   string `json:"dup"`   // a map with a repeated key: "yes", "no", or "?" when the oracle cannot tell (a key it cannot hold)
}

const maxInt = math.MaxInt32

var lowTag = regexp.MustCompile(`(^|[^0-9A-Za-z"'])([0-3]|55799)\(`)

// hasIndef reports whether a well formed item holds an indefinite length
// string, array or map.
func hasIndef(data []byte) bool {
	found := false
	var item func(i int) int
	item = func(i int) int {
		ib := data[i]
		major, ai := ib>>5, ib&31
		i++
		var arg uint64
		switch {
		case ai < 24:
			arg = uint64(ai)
		case ai == 24:
			arg = uint64(data[i])
			i++
		case ai == 25:
			arg = uint64(data[i])<<8 | uint64(data[i+1])
			i += 2
		case ai == 26:
			arg = uint64(data[i])<<24 | uint64(data[i+1])<<16 | uint64(data[i+2])<<8 | uint64(data[i+3])
			i += 4
		case ai == 27:
			for k := 0; k < 8; k++ {
				arg = arg<<8 | uint64(data[i+k])
			}
			i += 8
		}
		if ai == 31 {
			switch major {
			case 2, 3, 4, 5:
				found = true
				for data[i] != 0xff {
					i = item(i)
					if major == 5 {
						i = item(i)
					}
				}
				return i + 1
			}
			return i
		}
		switch major {
		case 2, 3:
			return i + int(arg)
		case 4:
			for k := uint64(0); k < arg; k++ {
				i = item(i)
			}
		case 5:
			for k := uint64(0); k < 2*arg; k++ {
				i = item(i)
			}
		case 6:
			return item(i)
		}
		return i
	}
	item(0)
	return found
}

func main() {
	if len(os.Args) > 1 && strings.HasPrefix(os.Args[1], "typed") {
		typedMain(os.Args[1])
		return
	}
	dec, err := cbor.DecOptions{
		MaxNestedLevels:  128,
		MaxArrayElements: maxInt,
		MaxMapPairs:      maxInt,
		DupMapKey:        cbor.DupMapKeyQuiet,
		DefaultMapType:   nil,
		BigIntDec:        cbor.BigIntDecodePointer,
	}.DecMode()
	if err != nil {
		panic(err)
	}
	decDup, err := cbor.DecOptions{
		MaxNestedLevels:  128,
		MaxArrayElements: maxInt,
		MaxMapPairs:      maxInt,
		DupMapKey:        cbor.DupMapKeyEnforcedAPF,
	}.DecMode()
	if err != nil {
		panic(err)
	}
	diagMode, err := cbor.DiagOptions{}.DiagMode()
	if err != nil {
		panic(err)
	}
	enc, err := cbor.CoreDetEncOptions().EncMode()
	if err != nil {
		panic(err)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		data, err := hex.DecodeString(strings.TrimSpace(sc.Text()))
		if err != nil {
			panic(err)
		}
		r := result{Diag: "ERR", Canon: "ERR", Dup: "no"}
		skipCanon := false
		func() {
			defer func() {
				if x := recover(); x != nil {
					r.Diag = fmt.Sprint("PANIC ", x)
				}
			}()
			r.WF = dec.Wellformed(data) == nil
			if !r.WF {
				return
			}
			indef := hasIndef(data)
			s, err := diagMode.Diagnose(data)
			if err != nil {
				if err.Error() == "cbor: invalid UTF-8 string" {
					r.Diag = "UTF8"
				}
			} else if indef {
				r.Diag = "SKIP"
			} else {
				r.Diag = s
			}
			// tags 0 to 3 and 55799 and undefined change meaning in the interface decode
			if err == nil && (lowTag.MatchString(s) || strings.Contains(s, "undefined") || strings.Contains(s, "NaN")) {
				skipCanon = true
			}
			var v any
			if err := decDup.Unmarshal(data, &v); err != nil {
				if _, ok := err.(*cbor.DupMapKeyError); ok {
					r.Dup = "yes"
				} else {
					r.Dup = "?"
				}
			}
			var iv any
			if err := dec.Unmarshal(data, &iv); err == nil && r.Dup == "no" && !hasBig(iv) {
				if skipCanon {
					r.Canon = "SKIP"
				} else if b, err := enc.Marshal(iv); err == nil {
					r.Canon = hex.EncodeToString(b)
				}
			}
		}()
		j, _ := json.Marshal(r)
		w.Write(j)
		w.WriteByte('\n')
	}
}

// hasBig reports whether the value holds a number past 64 bits, which the
// interface decode turns into a big.Int the corpus does not compare.
func hasBig(v any) bool {
	switch x := v.(type) {
	case *big.Int, big.Int:
		return true
	case []any:
		for _, e := range x {
			if hasBig(e) {
				return true
			}
		}
	case map[any]any:
		for k, e := range x {
			if hasBig(k) || hasBig(e) {
				return true
			}
		}
	case cbor.Tag:
		return hasBig(x.Content)
	}
	return false
}
