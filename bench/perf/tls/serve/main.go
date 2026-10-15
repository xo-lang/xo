// The server of bench/perf/tls: crypto/tls listeners, one per
// configuration measured, from -port on (see listeners), each answering
// HTTP/1.1 requests raw and closing the connection: /hs a 16 byte body,
// /big?n=N N bytes. Every certificate is signed by one CA (ECDSA P-256)
// made at start, whose PEM it writes to -ca; the leaves name 127.0.0.1.
// Only http/1.1 is offered (ALPN), so the Xo client opens a connection,
// and makes a full handshake, per request. Records are 16 KiB from the
// start (DynamicRecordSizingDisabled). Prints "ready" once it listens.
package main

import (
	"bufio"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"flag"
	"fmt"
	"math/big"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

type listener struct {
	name   string
	cert   string // p256, rsa, ed25519
	curve  tls.CurveID
	max    uint16
	suites []uint16
}

// listeners are served on -port, -port+1, ... in this order (main.xo
// and measure.sh name them by their offset).
var listeners = []listener{
	{"x25519", "p256", tls.X25519, tls.VersionTLS13, nil},
	{"p256", "p256", tls.CurveP256, tls.VersionTLS13, nil},
	{"p384", "p256", tls.CurveP384, tls.VersionTLS13, nil},
	{"rsa2048", "rsa", tls.X25519, tls.VersionTLS13, nil},
	{"ed25519", "ed25519", tls.X25519, tls.VersionTLS13, nil},
	{"tls12", "p256", tls.X25519, tls.VersionTLS12, []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}},
	{"aes128", "p256", tls.X25519, tls.VersionTLS12, []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256}},
	{"aes256", "p256", tls.X25519, tls.VersionTLS12, []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384}},
	{"chacha", "p256", tls.X25519, tls.VersionTLS12, []uint16{tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256}},
	{"aes128tls13", "p256", tls.X25519, tls.VersionTLS13, nil},
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	port := flag.Int("port", 18120, "the first listener's port")
	caFile := flag.String("ca", "ca.pem", "the file for the CA (PEM)")
	flag.Parse()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	now := time.Now()
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "bench tls CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	must(err)
	ca, _ := x509.ParseCertificate(caDER)
	must(os.WriteFile(*caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER}), 0o644))
	keys := map[string]crypto.Signer{}
	k1, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	must(err)
	keys["p256"] = k1
	k2, err := rsa.GenerateKey(rand.Reader, 2048)
	must(err)
	keys["rsa"] = k2
	_, k3, err := ed25519.GenerateKey(rand.Reader)
	must(err)
	keys["ed25519"] = k3
	certs := map[string]tls.Certificate{}
	for i, name := range []string{"p256", "rsa", "ed25519"} {
		t := &x509.Certificate{SerialNumber: big.NewInt(int64(i + 2)), Subject: pkix.Name{CommonName: "127.0.0.1"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}}
		der, err := x509.CreateCertificate(rand.Reader, t, ca, keys[name].Public(), caKey)
		must(err)
		certs[name] = tls.Certificate{Certificate: [][]byte{der, caDER}, PrivateKey: keys[name]}
	}
	for i, l := range listeners {
		cfg := &tls.Config{
			Certificates:                []tls.Certificate{certs[l.cert]},
			CurvePreferences:            []tls.CurveID{l.curve},
			MaxVersion:                  l.max,
			CipherSuites:                l.suites,
			NextProtos:                  []string{"http/1.1"},
			SessionTicketsDisabled:      true,
			DynamicRecordSizingDisabled: true,
		}
		ln, err := tls.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port+i), cfg)
		must(err)
		go serve(ln)
	}
	fmt.Println("ready")
	select {}
}

var zeros = make([]byte, 1<<20)

func serve(ln net.Listener) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func() {
			defer c.Close()
			br := bufio.NewReader(c)
			line, err := br.ReadString('\n')
			if err != nil {
				return
			}
			for {
				h, err := br.ReadString('\n')
				if err != nil || h == "\r\n" {
					break
				}
			}
			n := 16
			if f := strings.Fields(line); len(f) >= 2 && strings.HasPrefix(f[1], "/big?n=") {
				n, _ = strconv.Atoi(strings.TrimPrefix(f[1], "/big?n="))
			}
			fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n", n)
			for n > 0 {
				k := min(n, len(zeros))
				if _, err := c.Write(zeros[:k]); err != nil {
					return
				}
				n -= k
			}
		}()
	}
}
