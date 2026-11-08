package arm64

import (
	"bytes"
	"context"
	"debug/elf"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// TestFreeBSDCrossBuilds cross builds two goldens for freebsd/amd64 and
// freebsd/arm64 (decision 0573 stage 1: cross builds only, no VM yet) and
// reads each executable: a static ELF branded FreeBSD (EI_OSABI 9), its
// entry the nolibc _start, with a TLS segment and no interpreter.
func TestFreeBSDCrossBuilds(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("cross builds two programs for two targets")
	}
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	golden := filepath.Join("..", "llvm", "testdata", "golden")
	machines := map[string]elf.Machine{"freebsd/amd64": elf.EM_X86_64, "freebsd/arm64": elf.EM_AARCH64}
	for _, name := range []string{"arith", "json_sized_ints"} {
		for target, machine := range machines {
			t.Run(name+"/"+strings.ReplaceAll(target, "/", "_"), func(t *testing.T) {
				t.Parallel()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				bin := filepath.Join(t.TempDir(), "b")
				args := append(append([]string{}, cli[1:]...), "build", "--targets="+target, "-o", bin, ".")
				cmd := testtmp.Guard(exec.CommandContext(ctx, cli[0], args...))
				cmd.Dir = filepath.Join(golden, name)
				var stderr bytes.Buffer
				cmd.Stderr = &stderr
				if err := cmd.Run(); err != nil {
					t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
				}
				f, err := elf.Open(bin)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				if f.OSABI != elf.ELFOSABI_FREEBSD || f.Type != elf.ET_EXEC || f.Machine != machine {
					t.Errorf("header: OSABI %v, type %v, machine %v; want FreeBSD, EXEC, %v", f.OSABI, f.Type, f.Machine, machine)
				}
				tls := false
				for _, p := range f.Progs {
					switch p.Type {
					case elf.PT_INTERP:
						t.Errorf("a PT_INTERP segment: the executable is not static")
					case elf.PT_TLS:
						tls = true
					}
				}
				if !tls {
					t.Errorf("no PT_TLS segment")
				}
				syms, err := f.Symbols()
				if err != nil {
					t.Fatal(err)
				}
				start := false
				for _, s := range syms {
					if s.Name == "_start" && s.Value == f.Entry {
						start = true
					}
				}
				if !start {
					t.Errorf("the entry %#x is not _start", f.Entry)
				}
			})
		}
	}
}
