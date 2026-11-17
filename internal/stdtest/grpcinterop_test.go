package stdtest

import (
	"bufio"
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
	grpcCross(t, "grpcinterop", "serve", "client", false)
}

// TestGrpcHealthInterop crosses std/grpchealth with grpc-go's health package (decision 0546 item 5): every pairing of
// an Xo or grpc-go client with an Xo or grpc-go server makes the calls of testdata/grpchealth (Check, List, Watch
// through status changes, Shutdown and Resume) and must print want.txt; grpc-go with itself checks want.txt.
func TestGrpcHealthInterop(t *testing.T) {
	grpcCross(t, "grpchealth", "healthserve", "healthclient", true)
}

// grpcCross builds testdata/NAME and the grpc-go oracle, then runs a client of one against a server of the other
// (the oracle's modes refServe and refClient; the Xo program's serve and client), and with all, each with itself too.
// TestGrpcReflectInterop crosses std/grpcreflect with grpc-go's reflection server and client (task #236): every
// pairing of an Xo or grpc-go client with an Xo or grpc-go server lists the services and asks for files by symbol,
// name and extension, and for extension numbers, as testdata/grpcreflect's want.txt says.
func TestGrpcReflectInterop(t *testing.T) {
	grpcCross(t, "grpcreflect", "reflectserve", "reflectclient", true)
}

func grpcCross(t *testing.T, name, refServe, refClient string, all bool) {
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
	serve := func(t *testing.T, bin, mode string) string {
		cmd := exec.Command(bin, mode)
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
	run := func(t *testing.T, bin, mode, addr string) string {
		out, err := exec.Command(bin, mode, addr).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %s: %v\n%s", bin, mode, err, out)
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
}
