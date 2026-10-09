package gen_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestReplayGoCalls records a program that calls Go packages (decision
// 0093, core 11.3) and replays it offline: os.getpid and os.read_file are
// answered from the trace (the replay prints the recorded pid and reads
// no file), strconv and regexp run again without being recorded, and
// exec.command, whose result is a Go handle, runs again and is reported.
// A trace written before decision 0093 (no "foreign" header field, no
// go.call effects) still replays, running every Go call; a changed
// argument diverges; an xo.mod `replay rerun go` line stops the recording
// of the functions it names.
func TestReplayGoCalls(t *testing.T) {
	if testing.Short() {
		t.Skip("builds Go")
	}
	src := filepath.Join("testdata", "replay", "gocalls")
	bin := buildReplayProg(t, src, false)
	work := t.TempDir()
	if err := os.WriteFile(filepath.Join(work, "input.txt"), []byte("41\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	trace := filepath.Join(t.TempDir(), "run.xotrace")
	recd := runProg(t, bin, work, "", "XO_RECORD="+trace)
	if recd.code != 0 || !strings.Contains(recd.stdout, "input 42\nerror: *fs.PathError\nmatch true\ncommand 2\n") {
		t.Fatalf("recording: exit %d\n%s\n%s", recd.code, recd.stdout, recd.stderr)
	}
	data, err := os.ReadFile(trace)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`"foreign":1`, `"op":"go.call","a":["os.getpid",`, `"op":"go.call","a":["os.read_file",`, `"r":{"Ok":true,"V":"41\n"}`, `"type":"*fs.PathError"`, `"a":["exec.command"],"at":"main.xo:23","x":"handle"`} {
		if !strings.Contains(text, want) {
			t.Errorf("trace lacks %s:\n%s", want, text)
		}
	}
	for _, not := range []string{"strconv", "regexp"} {
		if strings.Contains(text, `"`+not+`.`) {
			t.Errorf("trace records pure %s calls:\n%s", not, text)
		}
	}
	// offline: no input.txt, another pid
	rep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+trace)
	if rep.code != 0 || rep.stdout != recd.stdout || !strings.Contains(rep.stderr, "xo replay: re executed foreign call `exec.command` (") || !strings.Contains(rep.stderr, "xo replay: identical run") {
		t.Fatalf("replay: exit %d\n--- stdout\n%s--- recorded\n%s--- stderr\n%s", rep.code, rep.stdout, recd.stdout, rep.stderr)
	}
	// an old trace: Go calls run again (this one needs input.txt)
	var old []string
	for _, l := range strings.Split(strings.TrimSpace(text), "\n") {
		if strings.Contains(l, `"op":"go.call"`) {
			continue
		}
		old = append(old, strings.Replace(l, `"foreign":1,`, "", 1))
	}
	oldTrace := filepath.Join(t.TempDir(), "old.xotrace")
	os.WriteFile(oldTrace, []byte(strings.Join(old, "\n")+"\n"), 0o644)
	orep := runProg(t, bin, work, "", "XO_REPLAY="+oldTrace)
	if orep.code != 0 || orep.stdout == recd.stdout || !strings.Contains(orep.stdout, "input 42\n") || strings.Contains(orep.stderr, "re executed") || !strings.Contains(orep.stderr, "identical run") {
		t.Fatalf("old trace: exit %d\n--- stdout\n%s--- stderr\n%s", orep.code, orep.stdout, orep.stderr)
	}
	// a changed argument diverges at the call
	hashRE := regexp.MustCompile(`("os.read_file",")[0-9a-f]+`)
	bad := hashRE.ReplaceAllString(text, "${1}0000000000000000")
	badTrace := filepath.Join(t.TempDir(), "bad.xotrace")
	os.WriteFile(badTrace, []byte(bad), 0o644)
	brep := runProg(t, bin, t.TempDir(), "", "XO_REPLAY="+badTrace)
	if brep.code != 3 || !strings.Contains(brep.stderr, "effect 2 of main differs:\n  the program calls  go.call(\"os.read_file\", ") {
		t.Fatalf("changed argument: exit %d\n%s", brep.code, brep.stderr)
	}
	// xo.mod `replay rerun go os getpid`: getpid runs again
	prog := filepath.Join(t.TempDir(), "gocalls")
	os.MkdirAll(prog, 0o755)
	srcText, _ := os.ReadFile(filepath.Join(src, "main.xo"))
	os.WriteFile(filepath.Join(prog, "main.xo"), srcText, 0o644)
	os.WriteFile(filepath.Join(prog, "xo.mod"), []byte("module gocalls\n\nreplay rerun go os getpid\n"), 0o644)
	rbin := buildReplayProg(t, prog, false)
	rtrace := filepath.Join(t.TempDir(), "rerun.xotrace")
	rrec := runProg(t, rbin, work, "", "XO_RECORD="+rtrace)
	rdata, _ := os.ReadFile(rtrace)
	if rrec.code != 0 || strings.Contains(string(rdata), "os.getpid") || !strings.Contains(string(rdata), "os.read_file") {
		t.Fatalf("rerun recording: exit %d\n%s\n%s", rrec.code, rrec.stderr, rdata)
	}
	rrep := runProg(t, rbin, t.TempDir(), "", "XO_REPLAY="+rtrace)
	if rrep.code != 0 || rrep.stdout == rrec.stdout || !strings.Contains(rrep.stdout, "input 42\n") || !strings.Contains(rrep.stderr, "identical run") {
		t.Fatalf("rerun replay: exit %d\n--- stdout\n%s--- stderr\n%s", rrep.code, rrep.stdout, rrep.stderr)
	}
}
