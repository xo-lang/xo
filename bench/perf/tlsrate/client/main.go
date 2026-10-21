// The load client of bench/perf/tlsrate: TLS handshakes per second, bulk
// throughput over one connection, and HTTP/2 requests per second, against
// a Go or Xo server (gosrv/, xosrv/) on loopback, with the server
// process's CPU time (from /proc, -pid) per unit of work.
//
//	client -gencert DIR [-key ec|rsa]      write cert.pem and key.pem, exit
//	client -mode hs   -url ADDR [-curve x25519|p256] [-resume] [-tls 12|13] [-c N] [-d D]
//	client -mode bulk -url ADDR -size BYTES [-tls 12|13] [-cipher aes|chacha] [-reps N]
//	client -mode h2   -url ADDR -c STREAMS [-d D]
//	client -mode h2big -url ADDR -size BYTES [-reps N]
//
// ADDR is host:port. The certificate is not verified (loopback bench).
package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

func gencert(dir, kind string) error {
	var priv any
	var pub any
	switch kind {
	case "rsa":
		k, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return err
		}
		priv, pub = k, &k.PublicKey
	default:
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return err
		}
		priv, pub = k, &k.PublicKey
	}
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, pub, priv)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return err
	}
	if err := os.WriteFile(dir+"/cert.pem", pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return err
	}
	return os.WriteFile(dir+"/key.pem", pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}), 0o600)
}

// cpuTicks is the server process's user+system CPU ticks from /proc.
func cpuTicks(pid int) int64 {
	if pid == 0 {
		return 0
	}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return 0
	}
	s := string(b)
	s = s[strings.LastIndexByte(s, ')')+2:]
	f := strings.Fields(s)
	var u, k int64
	fmt.Sscan(f[11], &u)
	fmt.Sscan(f[12], &k)
	return u + k
}

func tlsConfig(curve, ver, cipher string, resume bool) *tls.Config {
	c := &tls.Config{InsecureSkipVerify: true}
	switch curve {
	case "p256":
		c.CurvePreferences = []tls.CurveID{tls.CurveP256}
	case "x25519":
		c.CurvePreferences = []tls.CurveID{tls.X25519}
	}
	switch ver {
	case "12":
		c.MinVersion, c.MaxVersion = tls.VersionTLS12, tls.VersionTLS12
		switch cipher {
		case "chacha":
			c.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256, tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256}
		case "aes":
			c.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}
		}
	default:
		c.MinVersion, c.MaxVersion = tls.VersionTLS13, tls.VersionTLS13
	}
	if resume {
		c.ClientSessionCache = tls.NewLRUClientSessionCache(256)
	}
	return c
}

func main() {
	gen := flag.String("gencert", "", "write cert.pem and key.pem into this directory and exit")
	key := flag.String("key", "ec", "-gencert key type: ec (P-256) or rsa (2048)")
	mode := flag.String("mode", "hs", "hs, bulk, h2 or h2big")
	addr := flag.String("url", "127.0.0.1:18140", "host:port")
	conc := flag.Int("c", 16, "workers (hs) or concurrent streams (h2)")
	dur := flag.Duration("d", 8*time.Second, "measured duration (hs, h2)")
	warm := flag.Duration("warmup", 2*time.Second, "warmup (hs, h2)")
	curve := flag.String("curve", "x25519", "key exchange group: x25519 or p256")
	ver := flag.String("tls", "13", "TLS version: 12 or 13")
	cipher := flag.String("cipher", "", "with -tls 12: aes or chacha")
	resume := flag.Bool("resume", false, "hs: keep sessions so connections resume")
	size := flag.Int("size", 256<<20, "bulk/h2big: body bytes")
	reps := flag.Int("reps", 3, "bulk/h2big: repetitions")
	pid := flag.Int("pid", 0, "server pid (Linux): its CPU time is reported")
	flag.Parse()
	if *gen != "" {
		if err := gencert(*gen, *key); err != nil {
			fmt.Fprintln(os.Stderr, "client:", err)
			os.Exit(1)
		}
		return
	}
	out := map[string]any{"mode": *mode}
	cfg := tlsConfig(*curve, *ver, *cipher, *resume)
	const tick = 10000 // microseconds per clock tick (100 Hz)
	switch *mode {
	case "hs":
		var count, errs, resumed int64
		var stop atomic.Bool
		var measuring atomic.Bool
		var wg sync.WaitGroup
		for i := 0; i < *conc; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for !stop.Load() {
					c, err := tls.Dial("tcp", *addr, cfg)
					if err != nil {
						atomic.AddInt64(&errs, 1)
						continue
					}
					c.Write([]byte("GET /hs HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n"))
					io.Copy(io.Discard, c)
					if measuring.Load() {
						atomic.AddInt64(&count, 1)
						if c.ConnectionState().DidResume {
							atomic.AddInt64(&resumed, 1)
						}
					}
					c.Close()
				}
			}()
		}
		time.Sleep(*warm)
		c0 := cpuTicks(*pid)
		measuring.Store(true)
		t0 := time.Now()
		time.Sleep(*dur)
		measuring.Store(false)
		secs := time.Since(t0).Seconds()
		c1 := cpuTicks(*pid)
		stop.Store(true)
		wg.Wait()
		out["handshakes"] = count
		out["per_sec"] = float64(count) / secs
		out["resumed_pct"] = 100 * float64(resumed) / float64(max(count, 1))
		out["errors"] = errs
		out["server_cpu_us_per_conn"] = float64((c1-c0)*tick) / float64(max(count, 1))
	case "bulk":
		var rates []float64
		var cpus []float64
		for r := 0; r < *reps; r++ {
			c, err := tls.Dial("tcp", *addr, cfg)
			if err != nil {
				fmt.Fprintln(os.Stderr, "client:", err)
				os.Exit(1)
			}
			fmt.Fprintf(c, "GET /bulk?n=%d HTTP/1.1\r\nHost: x\r\nConnection: close\r\n\r\n", *size)
			c0 := cpuTicks(*pid)
			t0 := time.Now()
			n, _ := io.Copy(io.Discard, c)
			secs := time.Since(t0).Seconds()
			c1 := cpuTicks(*pid)
			c.Close()
			rates = append(rates, float64(n)/secs/1e6)
			cpus = append(cpus, float64((c1-c0)*tick)/(float64(n)/1e6))
			out["suite"] = tls.VersionName(c.ConnectionState().Version) + " " + tls.CipherSuiteName(c.ConnectionState().CipherSuite)
		}
		sort.Float64s(rates)
		sort.Float64s(cpus)
		out["MBps_best"] = rates[len(rates)-1]
		out["MBps_median"] = rates[len(rates)/2]
		out["server_cpu_us_per_MB_median"] = cpus[len(cpus)/2]
	case "h2", "h2big":
		tr := &http.Transport{
			TLSClientConfig:   &tls.Config{InsecureSkipVerify: true},
			ForceAttemptHTTP2: true,
			MaxConnsPerHost:   1,
		}
		cl := &http.Client{Transport: tr}
		base := "https://" + *addr
		if *mode == "h2big" {
			var rates []float64
			for r := 0; r < *reps; r++ {
				t0 := time.Now()
				resp, err := cl.Get(fmt.Sprintf("%s/bulk?n=%d", base, *size))
				if err != nil {
					fmt.Fprintln(os.Stderr, "client:", err)
					os.Exit(1)
				}
				if resp.ProtoMajor != 2 {
					fmt.Fprintln(os.Stderr, "client: not HTTP/2:", resp.Proto)
					os.Exit(1)
				}
				n, _ := io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				rates = append(rates, float64(n)/time.Since(t0).Seconds()/1e6)
			}
			sort.Float64s(rates)
			out["MBps_best"] = rates[len(rates)-1]
			out["MBps_median"] = rates[len(rates)/2]
			break
		}
		var count, errs int64
		var stop atomic.Bool
		var measuring atomic.Bool
		var lat sync.Mutex
		var lats []float64
		var wg sync.WaitGroup
		for i := 0; i < *conc; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				var mine []float64
				for !stop.Load() {
					t := time.Now()
					resp, err := cl.Get(base + "/hs")
					if err != nil {
						atomic.AddInt64(&errs, 1)
						continue
					}
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.ProtoMajor != 2 {
						atomic.AddInt64(&errs, 1)
					}
					if measuring.Load() {
						atomic.AddInt64(&count, 1)
						mine = append(mine, float64(time.Since(t).Microseconds()))
					}
				}
				lat.Lock()
				lats = append(lats, mine...)
				lat.Unlock()
			}()
		}
		time.Sleep(*warm)
		c0 := cpuTicks(*pid)
		measuring.Store(true)
		t0 := time.Now()
		time.Sleep(*dur)
		measuring.Store(false)
		secs := time.Since(t0).Seconds()
		c1 := cpuTicks(*pid)
		stop.Store(true)
		wg.Wait()
		sort.Float64s(lats)
		out["requests"] = count
		out["rps"] = float64(count) / secs
		out["errors"] = errs
		out["streams"] = *conc
		if len(lats) > 0 {
			out["p50_us"] = lats[len(lats)/2]
			out["p99_us"] = lats[len(lats)*99/100]
		}
		out["server_cpu_us_per_req"] = float64((c1-c0)*tick) / float64(max(count, 1))
	default:
		fmt.Fprintln(os.Stderr, "client: unknown -mode", *mode)
		os.Exit(2)
	}
	b, _ := json.Marshal(out)
	fmt.Println(string(b))
}
