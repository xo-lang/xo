package stdtest

import (
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestGrpcInterop crosses std/grpc with grpc-go (decision 0535): an Xo client calls a grpc-go server and a grpc-go
// client calls an Xo server, each making the calls of testdata/grpcinterop (a unary call with metadata, a status
// error, an unknown method, server, client and bidirectional streams) and printing what it saw; both must print
// testdata/grpcinterop/want.txt. The oracle (internal/stdtest/grpcref) is a Go module built offline (GOPROXY=off); the
// test skips when it cannot be built. Heavy (two builds): skipped with -short.
func TestGrpcInterop(t *testing.T) {
	grpcCross(t, "grpcinterop", "serve", "client", false, true)
}

// TestGrpcHealthInterop crosses std/grpchealth with grpc-go's health package (decision 0546 item 5): every pairing of
// an Xo or grpc-go client with an Xo or grpc-go server makes the calls of testdata/grpchealth (Check, List, Watch
// through status changes, Shutdown and Resume) and must print want.txt; grpc-go with itself checks want.txt.
func TestGrpcHealthInterop(t *testing.T) {
	grpcCross(t, "grpchealth", "healthserve", "healthclient", true, false)
}

// grpcCross builds testdata/NAME and the grpc-go oracle, then runs a client of one against a server of the other
// (the oracle's modes refServe and refClient; the Xo program's serve and client), and with all, each with itself too.
// TestGrpcReflectInterop crosses std/grpcreflect with grpc-go's reflection server and client (task #236): every
// pairing of an Xo or grpc-go client with an Xo or grpc-go server lists the services and asks for files by symbol,
// name and extension, and for extension numbers, as testdata/grpcreflect's want.txt says.
func TestGrpcReflectInterop(t *testing.T) {
	grpcCross(t, "grpcreflect", "reflectserve", "reflectclient", true, false)
}

// tls: the mutual TLS pairings too (serve-tls and client-tls), which only testdata/grpcinterop and grpcref's
// interop modes have (they failed TestGrpcHealthInterop and TestGrpcReflectInterop at 2c5618c53, run for every caller)
func grpcCross(t *testing.T, name, refServe, refClient string, all, tls bool) {
	if testing.Short() {
		t.Skip("builds an Xo program and the grpc-go oracle")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := testtmp.Dir("xo-" + name + "-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	ref := filepath.Join(dir, "grpcref")
	b := exec.Command("go", "build", "-o", ref, ".")
	b.Dir = filepath.Join(root, "internal", "stdtest", "grpcref")
	b.Env = append(os.Environ(), "GOFLAGS=-mod=mod", "GOPROXY=off")
	if out, err := b.CombinedOutput(); err != nil {
		t.Skipf("the grpc-go oracle cannot be built here: %v\n%s", err, out)
	}
	prog := filepath.Join(dir, name)
	if out, err := exec.Command(xo, "build", "-o", prog, filepath.Join(root, "internal", "stdtest", "testdata", name)).CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	want, err := os.ReadFile(filepath.Join(root, "internal", "stdtest", "testdata", name, "want.txt"))
	if err != nil {
		t.Fatal(err)
	}

	// serve starts a server binary and returns its address; the cleanup stops it.
	serve := func(t *testing.T, bin string, args ...string) string {
		cmd := exec.Command(bin, args...)
		out, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cmd.Process.Kill()
			cmd.Wait()
		})
		addr := make(chan string, 1)
		go func() {
			line, _ := bufio.NewReader(out).ReadString('\n')
			addr <- strings.TrimSpace(line)
		}()
		select {
		case a := <-addr:
			return a
		case <-time.After(30 * time.Second):
			t.Fatal("the server printed no address")
			return ""
		}
	}
	run := func(t *testing.T, bin string, args ...string) string {
		out, err := exec.Command(bin, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", bin, strings.Join(args, " "), err, out)
		}
		return string(out)
	}
	type side struct{ who, bin, serve, client string }
	xs := side{"xo", prog, "serve", "client"}
	gs := side{"grpc-go", ref, refServe, refClient}
	pairs := [][2]side{{xs, gs}, {gs, xs}}
	if all {
		pairs = append(pairs, [2]side{gs, gs}, [2]side{xs, xs})
	}
	for _, p := range pairs {
		c, s := p[0], p[1]
		t.Run(c.who+" client, "+s.who+" server", func(t *testing.T) {
			if got := run(t, c.bin, c.client, serve(t, s.bin, s.serve)); got != string(want) {
				t.Errorf("got:\n%swant:\n%s", got, want)
			}
		})
	}

	if !tls {
		return
	}
	// mutual TLS with per-RPC credentials that require it (map.txt grpc.behavior.mtls)
	certs := filepath.Join(dir, "certs")
	writeGrpcCerts(t, certs)
	wantTLS := "who: auth tls PrivacyAndIntegrity TLS 1.3 client=grpc client chains=1 authorization=Bearer tls-token\n" +
		"creds: /t.S/Who https://ADDR/t.S tls PrivacyAndIntegrity\n"
	for _, c := range []struct{ name, client, server string }{
		{"tls: xo client, grpc-go server", prog, ref},
		{"tls: grpc-go client, xo server", ref, prog},
		{"tls: xo client, xo server", prog, prog},
	} {
		t.Run(c.name, func(t *testing.T) {
			addr := serve(t, c.server, "serve-tls", certs)
			if got := run(t, c.client, "client-tls", addr, certs); got != wantTLS {
				t.Errorf("got:\n%swant:\n%s", got, wantTLS)
			}
		})
	}
}

// writeGrpcCerts writes a CA, a server pair for 127.0.0.1 and a client pair ("grpc client", clientAuth) into dir.
func writeGrpcCerts(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	write := func(name, kind string, der []byte) {
		if err := os.WriteFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: kind, Bytes: der}), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTmpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "grpc test CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature}
	caDER, err := x509.CreateCertificate(rand.Reader, caTmpl, caTmpl, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	write("ca.pem", "CERTIFICATE", caDER)
	leaf := func(name string, serial int64, cn string, eku x509.ExtKeyUsage, ips []net.IP) {
		k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		tmpl := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: cn}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(24 * time.Hour),
			KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{eku}, IPAddresses: ips}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &k.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		kd, err := x509.MarshalPKCS8PrivateKey(k)
		if err != nil {
			t.Fatal(err)
		}
		write(name+".pem", "CERTIFICATE", der)
		write(name+".key", "PRIVATE KEY", kd)
	}
	leaf("server", 2, "grpc server", x509.ExtKeyUsageServerAuth, []net.IP{net.ParseIP("127.0.0.1")})
	leaf("client", 3, "grpc client", x509.ExtKeyUsageClientAuth, nil)
}
