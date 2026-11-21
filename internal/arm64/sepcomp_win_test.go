package arm64

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestSepcompLinksWindows (decision 0388 stage 3 on windows/amd64 and
// windows/arm64): std's windows store and facts from the std cache (xo
// dev stdcache --target=windows/ARCH), and a program built against them (XO_SEPCOMP:
// its objects whole, std through pelink's COFF store link) links with its
// PDB, before and after an edit. The program instantiates a generic std
// type std also has (a weak definition each side, the store's kept). The
// executables run under Wine in the gate, not here. windows/arm64's std
// object linked the stub program instead of writing its store until
// 2026-11-10 (the link failed on xo_entry; winarm VM).
func TestSepcompLinksWindows(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	for _, arch := range []string{"amd64", "arm64"} {
		t.Run(arch, func(t *testing.T) {
			t.Parallel()
			sepcompLinksWindows(t, arch)
		})
	}
}

func sepcompLinksWindows(t *testing.T, arch string) {
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepcomp-win-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--target=windows/"+arch, "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := stdRun(cmd.Output)
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	src := `fn main(os: Os) {
  var m: Map[Str, Int] = {}
  for w in "edit 0 of the program".split(" ") {
    m[w] = w.len()
  }
  var xs: List[Int] = []
  for i in 0..20 {
    xs.push((i * 7) % 13)
  }
  os.stdio.println("${xs.sort().len()} ${m.len()} ${m.get("program") ?? 0}")
}
`
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	for edit := 0; edit < 2; edit++ {
		if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(strings.Replace(src, "edit 0", "edit "+string(rune('0'+edit)), 1)), 0o644); err != nil {
			t.Fatal(err)
		}
		out := filepath.Join(work, "sep", "prog")
		os.Remove(out + ".exe")
		os.Remove(out + ".pdb")
		b := testtmp.Guard(exec.Command(xo, "build", "--backend="+arch, "--targets=windows/"+arch, "-o", out, "."))
		b.Dir = prog
		b.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-sep"), "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
		if log, err := b.CombinedOutput(); err != nil {
			t.Fatalf("edit %d: xo build (XO_SEPCOMP, windows/%s): %v\n%s", edit, arch, err, log)
		}
		for _, f := range []string{out + ".exe", out + ".pdb"} {
			if fi, err := os.Stat(f); err != nil || fi.Size() == 0 {
				t.Fatalf("edit %d: %s missing or empty (%v)", edit, f, err)
			}
		}
	}
}

// TestSepcompWindowsProgramThunks (task #597): a program built against
// std's windows/amd64 store links when it calls a platform function
// through a std function the store leaves out (none of std's own
// functions calls it): the program writes that call's System V to Win64
// thunk, as a weak definition beside the store's thunks. Until
// 2026-11-21 a program against a store wrote no thunks (generate_parallel
// left them all to std's object), so every program using os.secrets'
// Dpapi failed with an undefined xo_w64_CryptProtectData on windows/amd64
// (secrets_dpapi, secrets_keychain and secrets_keyring on the Windows
// host); a whole program build linked.
func TestSepcompWindowsProgramThunks(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-sepcomp-thunks-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	cmd := testtmp.Guard(exec.Command(xo, "dev", "stdcache", "--target=windows/amd64", "--conc=0"))
	cmd.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-std"))
	paths, err := stdRun(cmd.Output)
	if err != nil {
		t.Fatalf("xo dev stdcache: %v", err)
	}
	lines := strings.Fields(string(paths))
	if len(lines) != 2 {
		t.Fatalf("xo dev stdcache printed %q, want the store and the facts", paths)
	}
	store, facts := lines[0], lines[1]
	if b, err := os.ReadFile(store); err != nil || strings.Contains(string(b), "xo_w64_CryptProtectData") {
		t.Fatalf("std's store %s has CryptProtectData's thunk (%v): the test needs a call only the program makes", store, err)
	}
	src := `fn main(os: Os) {
  let n = match os.secrets.dpapi() {
    Some(d) => (d.protect(b"token").ok() ?? b"").len()
    None => -1
  }
  os.stdio.println("${n}")
}
`
	prog := filepath.Join(work, "prog")
	if err := os.MkdirAll(prog, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(prog, "main.xo"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(work, "sep", "prog")
	b := testtmp.Guard(exec.Command(xo, "build", "--backend=amd64", "--targets=windows/amd64", "-o", out, "."))
	b.Dir = prog
	b.Env = append(os.Environ(), "XO_BACKEND_CACHE=on", "XO_BACKEND_CACHE_DIR="+filepath.Join(work, "c-sep"), "XO_SEPCOMP=1", "XO_SEPCOMP_FACTS="+facts, "XO_SEPCOMP_STORE="+store)
	if log, err := b.CombinedOutput(); err != nil {
		t.Fatalf("xo build (XO_SEPCOMP, windows/amd64): %v\n%s", err, log)
	}
	if fi, err := os.Stat(out + ".exe"); err != nil || fi.Size() == 0 {
		t.Fatalf("%s.exe missing or empty (%v)", out, err)
	}
}
