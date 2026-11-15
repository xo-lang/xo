package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

// optsMode answers std/cbor's decode_with cases (decision 0655 features
// line): each line is "SET HEX", SET an index into optSets below (the Xo
// test's opts_of(SET) builds the same DecodeOptions). It writes {"err":
// TEXT}: "" when the input decodes, the error text when it is one of the
// option errors, "?" for another error (compared only as an error).
func optsMode() {
	sets := []cbor.DecOptions{}
	for k := 0; k < 8; k++ {
		sets = append(sets, optSet(k))
	}
	var modes []cbor.DecMode
	for _, o := range sets {
		m, err := o.DecMode()
		if err != nil {
			panic(err)
		}
		modes = append(modes, m)
	}
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 1<<20), 1<<26)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		k, _ := strconv.Atoi(f[0])
		data, err := hex.DecodeString(f[1])
		if err != nil {
			panic(err)
		}
		var v any
		err = modes[k].Unmarshal(data, &v)
		out := ""
		if err != nil {
			out = "?"
			var e1 *cbor.MaxArrayElementsError
			var e2 *cbor.MaxMapPairsError
			var e3 *cbor.IndefiniteLengthError
			var e4 *cbor.TagsMdError
			var e5 *cbor.UnacceptableDataItemError
			var e6 cbor.UnacceptableDataItemError
			if errors.As(err, &e1) || errors.As(err, &e2) || errors.As(err, &e3) || errors.As(err, &e4) || errors.As(err, &e5) || errors.As(err, &e6) {
				out = err.Error()
			}
		}
		b, _ := json.Marshal(map[string]string{"err": out})
		fmt.Fprintln(w, string(b))
	}
}

// optSet is the Xo test's opts_of(k); every set keeps the oracle's usual
// nesting, counts and duplicate key mode besides what it tests.
func optSet(k int) cbor.DecOptions {
	o := cbor.DecOptions{MaxNestedLevels: 128, MaxArrayElements: maxInt, MaxMapPairs: maxInt, DupMapKey: cbor.DupMapKeyQuiet}
	if k == 0 || k == 7 {
		o.MaxArrayElements = 16
	}
	if k == 1 || k == 7 {
		o.MaxMapPairs = 16
	}
	if k == 2 || k == 7 {
		o.IndefLength = cbor.IndefLengthForbidden
	}
	if k == 3 || k == 7 {
		o.TagsMd = cbor.TagsForbidden
	}
	if k == 4 || k == 7 {
		o.NaN = cbor.NaNDecodeForbidden
	}
	if k == 5 || k == 7 {
		o.Inf = cbor.InfDecodeForbidden
	}
	if k == 6 || k == 7 {
		r, err := cbor.NewSimpleValueRegistryFromDefaults(cbor.WithRejectedSimpleValue(0), cbor.WithRejectedSimpleValue(20), cbor.WithRejectedSimpleValue(255))
		if err != nil {
			panic(err)
		}
		o.SimpleValues = r
	}
	return o
}
