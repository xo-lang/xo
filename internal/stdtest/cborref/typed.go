package main

import (
	"bufio"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"

	"github.com/fxamacker/cbor/v2"
)

// The structs of the derive Cbor corpus (scripts/stdcorpus/cbor.go keeps
// the Xo side in step), decision 0527.
type typedAddr struct {
	City string `cbor:"city"`
	Zip  int    `cbor:"zip"`
}

type typedPerson struct {
	Name  string         `cbor:"name"`
	Age   int            `cbor:"7,keyasint"`
	Tags  []string       `cbor:"tags,omitempty"`
	Addr  *typedAddr     `cbor:"addr"`
	Ratio float64        `cbor:"ratio"`
	Ok    bool           `cbor:"ok"`
	Raw   []byte         `cbor:"raw"`
	Attrs map[string]int `cbor:"attrs"`
	Skip  string         `cbor:"-"`
}

type typedDeep struct {
	Addrs  []typedAddr          `cbor:"addrs"`
	By     map[string]typedAddr `cbor:"by"`
	Opt    *int                 `cbor:"opt"`
	Grid   [][]int              `cbor:"grid"`
	Maybe  *float64             `cbor:"maybe"`
	Flag   *bool                `cbor:"flag"`
	Nested *typedPerson         `cbor:"nested"`
	Count  int                  `cbor:"count,omitempty"`
}

type typedResult struct {
	Err bool   `json:"err"`
	Out string `json:"out"` // hex of the deterministic marshal of what decoded ("" when Err)
}

// typedMain reads hex inputs a line and writes the outcome of decoding each
// into typedPerson (strict: false) and marshaling the result.
func typedMain(mode string) {
	dec, err := cbor.DecOptions{MaxNestedLevels: 128, MaxArrayElements: maxInt, MaxMapPairs: maxInt, DupMapKey: cbor.DupMapKeyQuiet}.DecMode()
	if err != nil {
		panic(err)
	}
	eo := cbor.CoreDetEncOptions()
	eo.NilContainers = cbor.NilContainerAsEmpty
	enc, err := eo.EncMode()
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
		var res typedResult
		var p any = &typedPerson{}
		if mode == "typed2" {
			p = &typedDeep{}
		}
		if err := dec.Unmarshal(data, p); err != nil {
			res.Err = true
		} else {
			out, err := enc.Marshal(p)
			if err != nil {
				res.Err = true
			} else {
				res.Out = hex.EncodeToString(out)
			}
		}
		b, _ := json.Marshal(res)
		w.Write(b)
		w.WriteByte('\n')
	}
}
