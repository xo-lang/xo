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
	dir, err := testtmp.Dir("xo-grpcinterop-")
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
	prog := filepath.Join(dir, "interop")
	if out, err := exec.Command(xo, "build", "-o", prog, filepath.Join(root, "internal", "stdtest", "testdata", "grpcinterop")).CombinedOutput(); err != nil {
		t.Fatalf("xo build: %v\n%s", err, out)
	}
	want, err := os.ReadFile(filepath.Join(root, "internal", "stdtest", "testdata", "grpcinterop", "want.txt"))
	if err != nil {
		t.Fatal(err)
	}

	// serve starts a server binary and returns its address; the cleanup stops it.
	serve := func(bin string) string {
		cmd := exec.Command(bin, "serve")
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
	run := func(bin, addr string) string {
		out, err := exec.Command(bin, "client", addr).CombinedOutput()
		if err != nil {
			t.Fatalf("%s client: %v\n%s", bin, err, out)
		}
		return string(out)
	}

	t.Run("xo client, grpc-go server", func(t *testing.T) {
		if got := run(prog, serve(ref)); got != string(want) {
			t.Errorf("got:\n%swant:\n%s", got, want)
		}
	})
	t.Run("grpc-go client, xo server", func(t *testing.T) {
		if got := run(ref, serve(prog)); got != string(want) {
			t.Errorf("got:\n%swant:\n%s", got, want)
		}
	})
}
