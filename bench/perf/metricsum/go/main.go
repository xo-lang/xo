// Go twin of bench/perf/metricsum: client_golang's Summary (beorn7/perks).
package main

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func main() {
	r := prometheus.NewRegistry()
	s := prometheus.NewSummary(prometheus.SummaryOpts{Name: "lat", Help: "Latency.", Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}})
	r.MustRegister(s)
	x := 1
	for i := 0; i < 2000000; i++ {
		x = (x * 7) % 1000003
		s.Observe(float64(x))
	}
	mfs, _ := r.Gather()
	var b bytes.Buffer
	for _, mf := range mfs {
		expfmt.MetricFamilyToText(&b, mf)
	}
	n := 0
	for _, line := range strings.Split(b.String(), "\n") {
		if strings.HasPrefix(line, "lat{") {
			n++
		}
	}
	fmt.Println("metricsum", n, b.Len() > 0)
}
