package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// otlpOracle runs internal/stdtest/otlpref (google.golang.org/protobuf marshaling the opentelemetry-proto trace
// messages) over the JSON lines of in and returns its hex lines.
func otlpOracle(in string) []string {
	cmd := exec.Command("go", "run", ".")
	cmd.Dir = filepath.Join("internal", "stdtest", "otlpref")
	cmd.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	cmd.Stdin = strings.NewReader(in)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stdcorpus: the otlp oracle (internal/stdtest/otlpref) failed: %v\n", err)
		os.Exit(1)
	}
	return strings.Split(strings.TrimRight(string(out), "\n"), "\n")
}

type otlpSpan struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	StartMs int64  `json:"start_ms"`
	Micros  int64  `json:"micros"`
	Status  int64  `json:"status"`
}

type otlpCase struct {
	Service string      `json:"service"`
	Attrs   [][2]string `json:"attrs"`
	Scope   string      `json:"scope"`
	Version string      `json:"version"`
	Spans   []otlpSpan  `json:"spans"`
}

func otlpCorpus() string {
	r := rand.New(rand.NewSource(539 + protoFuzzSeed()))
	hexd := "0123456789abcdef"
	rid := func() string {
		switch r.Intn(4) {
		case 0:
			b := make([]byte, 32)
			for i := range b {
				b[i] = hexd[r.Intn(16)]
			}
			return string(b)
		case 1: // 32 characters that are not all hex
			b := make([]byte, 32)
			for i := range b {
				b[i] = hexd[r.Intn(16)]
			}
			b[r.Intn(32)] = 'g'
			return string(b)
		case 2:
			return ""
		}
		b := make([]byte, 1+r.Intn(20))
		for i := range b {
			b[i] = "abcXYZ019-_"[r.Intn(11)]
		}
		return string(b)
	}
	words := []string{"db", "cache", "render", "auth", "queue", "request", "", "héllo", "x y"}
	var cases []otlpCase
	for i := 0; i < 120; i++ {
		c := otlpCase{Service: []string{"api", "", "svc-é", "a.b"}[r.Intn(4)], Scope: []string{"std/trace", "", "lib"}[r.Intn(3)], Version: []string{"", "1.0", "v2"}[r.Intn(3)]}
		for n := r.Intn(3); n > 0; n-- {
			c.Attrs = append(c.Attrs, [2]string{words[r.Intn(len(words))] + "k", words[r.Intn(len(words))]})
		}
		for n := r.Intn(6); n > 0; n-- {
			s := otlpSpan{ID: rid(), Name: words[r.Intn(len(words))], StartMs: r.Int63n(4e12), Micros: r.Int63n(1e7)}
			if r.Intn(5) == 0 {
				s.StartMs = 0
			}
			if s.Name == "request" {
				s.Status = []int64{0, 200, 404, 499, 500, 503, 599}[r.Intn(7)]
			}
			c.Spans = append(c.Spans, s)
		}
		cases = append(cases, c)
	}
	var in bytes.Buffer
	for _, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			panic(err)
		}
		in.Write(b)
		in.WriteByte('\n')
	}
	out := otlpOracle(in.String())
	var o strings.Builder
	o.WriteString(header("google.golang.org/protobuf marshaling the opentelemetry-proto trace messages (internal/stdtest/otlpref)"))
	o.WriteString("use std/metrics\nuse std/otlp\nuse std/trace\n\n")
	o.WriteString("/// (service, resource attributes, scope, scope version, spans, the ExportTraceServiceRequest bytes in hex)\n")
	o.WriteString("const CASES: List[(Str, List[(Str, Str)], Str, Str, List[trace.Span], Str)] = [\n")
	for i, c := range cases {
		var at, sp []string
		for _, a := range c.Attrs {
			at = append(at, fmt.Sprintf("(%s, %s)", xq(a[0]), xq(a[1])))
		}
		for _, s := range c.Spans {
			sp = append(sp, fmt.Sprintf("trace.Span{id: %s, name: %s, start_ms: %d, micros: %d, status: %d}", xq(s.ID), xq(s.Name), s.StartMs, s.Micros, s.Status))
		}
		fmt.Fprintf(&o, "  (%s, [%s], %s, %s, [%s], %s),\n", xq(c.Service), strings.Join(at, ", "), xq(c.Scope), xq(c.Version), strings.Join(sp, ", "), xq(out[i]))
	}
	o.WriteString("]\n")
	o.WriteString(otlpMetricsCorpus(r))
	o.WriteString(otlpCheckXo)
	return o.String()
}

type otlpPoint struct {
	Values []string  `json:"values"`
	Obs    []float64 `json:"obs"`
}

type otlpFamily struct {
	Kind   int         `json:"kind"`
	Name   string      `json:"name"`
	Help   string      `json:"help"`
	Labels []string    `json:"labels"`
	Bounds []float64   `json:"bounds"`
	Series []otlpPoint `json:"series"`
}

type otlpMetricsCase struct {
	Service  string       `json:"service"`
	Attrs    [][2]string  `json:"attrs"`
	Scope    string       `json:"scope"`
	Version  string       `json:"version"`
	StartMs  int64        `json:"start_ms"`
	NowMs    int64        `json:"now_ms"`
	Families []otlpFamily `json:"families"`
}


func xfl(fs []float64) string {
	var p []string
	for _, f := range fs {
		p = append(p, xf(f))
	}
	return "[" + strings.Join(p, ", ") + "]"
}

func xsl(ss []string) string {
	var p []string
	for _, s := range ss {
		p = append(p, xq(s))
	}
	return "[" + strings.Join(p, ", ") + "]"
}

// otlpMetricsCorpus: registries of counters, gauges and histograms (values are multiples of 1/8, so the sums are exact).
func otlpMetricsCorpus(r *rand.Rand) string {
	val := func() float64 { return float64(r.Intn(400)) / 8 }
	words := []string{"a", "b", "é", "x y", "z"}
	var cases []otlpMetricsCase
	for i := 0; i < 60; i++ {
		c := otlpMetricsCase{Service: []string{"api", "svc"}[r.Intn(2)], Scope: []string{"std/metrics", "lib"}[r.Intn(2)], Version: []string{"", "1"}[r.Intn(2)],
			StartMs: []int64{0, 1700000000000}[r.Intn(2)], NowMs: []int64{0, 1700000060000, 1700000999123}[r.Intn(3)], Attrs: [][2]string{}}
		c.Families = []otlpFamily{}
		for n := r.Intn(4); n >= 0; n-- {
			f := otlpFamily{Kind: r.Intn(3), Name: fmt.Sprintf("m%d_%d", i, n), Help: []string{"", "help text", "é"}[r.Intn(3)], Labels: []string{}}
			for l := r.Intn(3); l > 0; l-- {
				f.Labels = append(f.Labels, fmt.Sprintf("l%d", l))
			}
			if f.Kind == 2 {
				f.Bounds = []float64{}
				b := val() / 8
				for k := r.Intn(5); k >= 0; k-- {
					f.Bounds = append(f.Bounds, b)
					b += float64(1+r.Intn(40)) / 8
				}
			}
			seen := map[string]bool{}
			for sn := 1 + r.Intn(3); sn > 0; sn-- {
				vs := []string{}
				for range f.Labels {
					vs = append(vs, words[r.Intn(len(words))])
				}
				key := strings.Join(vs, "\x1f")
				if seen[key] {
					continue
				}
				seen[key] = true
				var obs []float64
				for k := 1 + r.Intn(5); k > 0; k-- {
					obs = append(obs, val())
				}
				f.Series = append(f.Series, otlpPoint{Values: vs, Obs: obs})
			}
			c.Families = append(c.Families, f)
		}
		cases = append(cases, c)
	}
	var in bytes.Buffer
	for _, c := range cases {
		b, err := json.Marshal(c)
		if err != nil {
			panic(err)
		}
		in.Write(b)
		in.WriteByte('\n')
	}
	out := otlpOracle(in.String())
	var o strings.Builder
	o.WriteString("\n/// (service, scope, scope version, start ms, now ms, families, the ExportMetricsServiceRequest bytes in hex); a family is\n")
	o.WriteString("/// (kind 0 counter, 1 gauge, 2 histogram; name; help; labels; bounds; series), a series (label values, observations)\n")
	o.WriteString("const METRICS: List[(Str, Str, Str, Int, Int, List[(Int, Str, Str, List[Str], List[Float], List[(List[Str], List[Float])])], Str)] = [\n")
	for i, c := range cases {
		var fs []string
		for _, f := range c.Families {
			var ss []string
			for _, s := range f.Series {
				ss = append(ss, fmt.Sprintf("(%s, %s)", xsl(s.Values), xfl(s.Obs)))
			}
			fs = append(fs, fmt.Sprintf("(%d, %s, %s, %s, %s, [%s])", f.Kind, xq(f.Name), xq(f.Help), xsl(f.Labels), xfl(f.Bounds), strings.Join(ss, ", ")))
		}
		fmt.Fprintf(&o, "  (%s, %s, %s, %d, %d, [%s], %s),\n", xq(c.Service), xq(c.Scope), xq(c.Version), c.StartMs, c.NowMs, strings.Join(fs, ", "), xq(out[i]))
	}
	o.WriteString("]\n")
	return o.String()
}

const otlpCheckXo = `
test "otlp.encode_traces: the bytes protobuf-go marshals for the same trace messages" {
  for (service, attrs, scope, version, spans, want) in CASES {
    let got = otlp.encode_traces(otlp.Resource{service: service, attributes: attrs, scope: scope, version: version}, spans)
    if got.to_hex() != want {
      fault("${service} ${spans.len()} spans: got ${got.to_hex()} want ${want}")
    }
  }
}

test "otlp.encode_metrics: the bytes protobuf-go marshals for the same metrics messages" {
  for (service, scope, version, start, now, fams, want) in METRICS {
    let reg = metrics.registry()
    for (kind, name, help, labels, bounds, series) in fams {
      if kind == 0 {
        let c = reg.counter(name, help, labels)
        for (vs, obs) in series {
          for v in obs {
            c.add(v, vs)
          }
        }
      } else if kind == 1 {
        let g = reg.gauge(name, help, labels)
        for (vs, obs) in series {
          g.set(obs[obs.len() - 1], vs)
        }
      } else {
        let h = reg.histogram(name, help, labels, bounds)
        for (vs, obs) in series {
          for v in obs {
            h.observe(v, vs)
          }
        }
      }
    }
    let got = otlp.encode_metrics(otlp.Resource{service: service, scope: scope, version: version}, reg, start, now)
    if got.to_hex() != want {
      fault("${service} ${fams.len()} families: got ${got.to_hex()} want ${want}")
    }
  }
}
`
