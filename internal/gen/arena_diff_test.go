package gen

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// Arena differential tests on examples/notes-service (decision 0066): the
// golden programs are covered by TestGolden (arenas, the default) against
// TestGoldenNoArenas; these run the example's tests and its real server
// built both ways and require the same results.

// copyExample copies the .xo sources of examples/notes-service into a
// fresh directory: `xo test` may write regression files into the module.
func copyExample(t *testing.T) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "examples", "notes-service", "*.xo"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no example sources: %v", err)
	}
	dir := t.TempDir()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(f)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// timingRE matches a duration or a time stamp printed by a run.
var timingRE = regexp.MustCompile(`\d{4}-\d\d-\d\dT[\d:.]+Z|\d+(\.\d+)?(ns|µs|us|ms|s)\b`)

// TestExampleArenaDiff runs the tests of examples/notes-service (HTTP JSON,
// std/sql, auth middleware, a background job) built with and without
// arenas and requires the same results.
func TestExampleArenaDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go twice")
	}
	dir := copyExample(t)
	on, _ := buildAndRunOpts(t, dir, xotool.Options{Mode: xotool.ModeTest})
	off, _ := buildAndRunOpts(t, dir, xotool.Options{Mode: xotool.ModeTest, NoArenas: true})
	if on.exit != 0 {
		t.Errorf("tests failed with arenas:\n%s", on.String())
	}
	a, b := timingRE.ReplaceAllString(on.String(), "<t>"), timingRE.ReplaceAllString(off.String(), "<t>")
	if a != b {
		t.Errorf("results differ\n--- arenas\n%s\n--- no arenas\n%s", a, b)
	}
}

// TestExampleArenaServeDiff runs the notes-service server built with and
// without arenas, sends the same requests (a scripted sequence, then
// concurrent clients so arenas are reused while earlier values live on in
// the database), and requires the same responses. The /notes routes sit
// behind a mounted router's middleware, which sees every response.
func TestExampleArenaServeDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go twice")
	}
	dir := copyExample(t)
	var outs [2]string
	for i, noArenas := range []bool{false, true} {
		res, err := xotool.Build([]string{dir}, xotool.Options{NoArenas: noArenas})
		if err != nil || res.Failed {
			t.Fatalf("build: %v %+v", err, res)
		}
		outs[i] = serveNotes(t, res.Binaries[xotool.Host()], dir)
	}
	if outs[0] != outs[1] {
		t.Errorf("responses differ\n--- arenas\n%s\n--- no arenas\n%s", outs[0], outs[1])
	}
	for _, want := range []string{"GET /health 200", "POST /notes 201", "GET /notes 200", "GET /notes/1 200", "DELETE /notes/2 204", "GET /notes/2 404", "GET /notes 401", "concurrent 4x30 bad 0"} {
		if !strings.Contains(outs[0], want) {
			t.Errorf("transcript lacks %q:\n%s", want, outs[0])
		}
	}
}

// serveNotes starts the server binary, runs the requests, stops it with
// SIGINT, and returns the transcript with time stamps masked.
func serveNotes(t *testing.T, bin, dir string) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	cmd := exec.Command(bin)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "NOTES_TOKEN=tok-admin", fmt.Sprintf("NOTES_ADDR=127.0.0.1:%d", port))
	var errb strings.Builder
	cmd.Stdout, cmd.Stderr = io.Discard, &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	stopped := false
	defer func() {
		if !stopped {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	do := func(method, path, token, body string) (int, string, error) {
		req, err := http.NewRequest(method, base+path, strings.NewReader(body))
		if err != nil {
			return 0, "", err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0, "", err
		}
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b), err
	}
	ready := false
	for i := 0; i < 200 && !ready; i++ {
		if st, _, err := do("GET", "/health", "", ""); err == nil && st == 200 {
			ready = true
		} else {
			time.Sleep(25 * time.Millisecond)
		}
	}
	if !ready {
		t.Fatalf("server did not start:\n%s", errb.String())
	}
	var out strings.Builder
	step := func(method, path, token, body string) {
		st, b, err := do(method, path, token, body)
		if err != nil {
			t.Fatalf("%s %s: %v", method, path, err)
		}
		fmt.Fprintf(&out, "%s %s %d %s\n", method, path, st, b)
	}
	step("GET", "/health", "", "")
	step("POST", "/notes", "tok-admin", `{"body":"first note, rather long to fill the arena","ttl_secs":600}`)
	step("POST", "/notes", "tok-admin", `{"body":"second"}`)
	step("POST", "/notes", "tok-admin", `{"body":""}`)
	step("POST", "/notes", "tok-admin", `{"body":`)
	step("GET", "/notes", "tok-admin", "")
	step("GET", "/notes/1", "tok-admin", "")
	step("DELETE", "/notes/2", "tok-admin", "")
	step("GET", "/notes/2", "tok-admin", "")
	step("GET", "/notes", "nope", "")
	step("GET", "/notes", "", "")
	step("GET", "/missing", "", "")
	// concurrent clients: each creates notes of its own and reads them back
	const clients, per = 4, 30
	var wg sync.WaitGroup
	var mu sync.Mutex
	bad := 0
	for c := 0; c < clients; c++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < per; j++ {
				text := fmt.Sprintf("c%d-n%d-%s", c, j, strings.Repeat("x", (c*per+j)%41))
				st, b, err := do("POST", "/notes", "tok-admin", fmt.Sprintf(`{"body":%q}`, text))
				ok := err == nil && st == 201 && strings.Contains(b, fmt.Sprintf(`"body":%q`, text))
				if ok {
					var id int
					if _, err := fmt.Sscanf(b[strings.Index(b, `"id":`)+5:], "%d", &id); err == nil {
						st, b, err = do("GET", fmt.Sprintf("/notes/%d", id), "tok-admin", "")
						ok = err == nil && st == 200 && strings.Contains(b, fmt.Sprintf(`"body":%q`, text))
					} else {
						ok = false
					}
				}
				if !ok {
					mu.Lock()
					bad++
					mu.Unlock()
				}
			}
		}()
	}
	wg.Wait()
	fmt.Fprintf(&out, "concurrent %dx%d bad %d\n", clients, per, bad)
	st, b, err := do("GET", "/notes", "tok-admin", "")
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(&out, "GET /notes after: %d, %d notes\n", st, strings.Count(b, `"id":`))
	if err := cmd.Process.Signal(syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		stopped = true
		if err != nil {
			fmt.Fprintf(&out, "exit: %v\n", err)
		}
	case <-time.After(20 * time.Second):
		t.Fatalf("server did not stop:\n%s", errb.String())
	}
	return timingRE.ReplaceAllString(out.String(), "<t>")
}
