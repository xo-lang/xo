// Go twin of bench/perf/metrics: client_golang's CounterVec, HistogramVec
// and the text exposition.
package main

import (
	"bytes"
	"fmt"
	"os"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"
)

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	r := prometheus.NewRegistry()
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "req_total", Help: "Requests."}, []string{"path"})
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: "lat_seconds", Help: "Latency.", Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}}, []string{"path"})
	big := prometheus.NewCounterVec(prometheus.CounterOpts{Name: "big_total", Help: "Many series."}, []string{"id"})
	r.MustRegister(c, h, big)
	names := make([]string, 50)
	for i := range names {
		names[i] = "p" + strconv.Itoa(i)
	}
	sum := 0
	switch w {
	case "counter":
		for i := 0; i < 3000000; i++ {
			c.WithLabelValues(names[i%50]).Inc()
		}
		sum = 3000000
	case "histogram":
		x := 1
		for i := 0; i < 2000000; i++ {
			x = (x * 7) % 1000003
			h.WithLabelValues(names[i%20]).Observe(float64(x) / 100000.0)
		}
		sum = 2000000
	case "scrape":
		for i := 0; i < 500; i++ {
			big.WithLabelValues("s" + strconv.Itoa(i)).Inc()
		}
		for i := 0; i < 50; i++ {
			h.WithLabelValues(names[i]).Observe(0.01 * float64(i+1))
		}
		for k := 0; k < 300; k++ {
			mfs, _ := r.Gather()
			var b bytes.Buffer
			for _, mf := range mfs {
				expfmt.MetricFamilyToText(&b, mf)
			}
			sum += b.Len()
		}
	}
	fmt.Println(w, sum)
}
