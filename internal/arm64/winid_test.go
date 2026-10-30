package arm64

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

// winSubset is the windows/amd64 per-step subset of the goldens
// (decision 0254 6b; scripts/wine-goldens-subset.txt runs the same ones
// under Wine): platform calls, files, network, threads and tasks, thread
// locals, faults and unwinding, callbacks, processes, floats, zone data.
var winSubset = []string{"abs_fault", "arith", "clock_timers", "cli_fault", "conc_channels", "conc_mutex", "conc_scopes", "conc_select", "conc_stress", "div_zero", "env_program", "fault_defer_chain", "fault_tasks", "fault_unwind", "float_libm", "fs_cwd_path", "fs_handles", "fs_native", "http_client", "http_server", "index_oob", "map_key_fault", "net_echo", "overflow_add", "proc_set_grace", "sys_files_clock", "task_budget", "task_native", "term_stdio", "tls13_server", "tz_table"}

// winIdentityAllowed are the functions whose code differs by target for
// a reason of the target's (register choices after a value one OS's
// runtime makes known; a spill Linux keeps in the red zone below rsp,
// which Windows does not have, so its frame is larger), not 6b's:
// checked 2026-10-19.
var winIdentityAllowed = map[string]bool{"byref_struct/merged": true, "cli_parse/attempt": true, "method_values/main": true}

// TestWindowsAmd64Identity: the amd64 backend's windows/amd64 code of a
// golden's own functions and the compiler's instances (not std's, whose
// runtime differs by OS) is linux/amd64's, as decision 0254 6c makes Xo's
// calls System V on both. Shrink wrapping is off for Linux too (Windows
// has it off: one prologue per function), and the red zone frames
// (x86_red_zone, 0136b3ce8: a System V leaf keeps its frame below rsp,
// which Windows has not), the fault stubs' register saves off for both
// (x86_stub_saves: below rsp on System V, in frame words on Windows),
// and inlining off for both (the runtime inlined
// differs by OS). Normalized: directives (.seh against
// .cfi), labels, the export calls' targets (Windows calls the function,
// Linux the export's alias of it), the C runtime's functions Windows
// with no C runtime calls as Xo's (cabi_wincrt.fmod for fmod), thunk names, a leaf's fault stub
// (Windows jumps, Linux makes a frame record and calls), and literal
// label numbers. The subset by default; XO_TEST_WIN_IDENTITY=all every
// golden.
func TestWindowsAmd64Identity(t *testing.T) {
	if testing.Short() {
		t.Skip("emits every golden twice")
	}
	names := winSubset
	if os.Getenv("XO_TEST_WIN_IDENTITY") == "all" {
		dirs := goldenDirs(t)
		names = nil
		for _, d := range dirs {
			names = append(names, filepath.Base(d))
		}
	}
	for _, name := range names {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join("..", "llvm", "testdata", "golden", name)
			lin := winAsm(t, dir, "linux/amd64", "-shrink_split,-inline,-x86_red_zone,-x86_stub_saves")
			win := winAsm(t, dir, "windows/amd64", "-inline,-x86_stub_saves")
			alias := exportAliases(lin)
			a := ownFuncsNorm(lin, alias)
			b := ownFuncsNorm(win, alias)
			var bad []string
			for n, la := range a {
				wb, ok := b[n]
				if !ok || winIdentityAllowed[name+"/"+n] {
					continue
				}
				if la != wb {
					bad = append(bad, n)
				}
			}
			sort.Strings(bad)
			for _, n := range bad {
				t.Errorf("%s: windows/amd64 differs from linux/amd64\n%s", n, firstDiff(a[n], b[n]))
			}
		})
	}
}

// winAsm is `xo asm --backend=amd64 --target=target` of dir with
// XO_PASSES=passes.
func winAsm(t *testing.T, dir, target, passes string) string {
	t.Helper()
	cli, err := xotool.Command()
	if err != nil {
		t.Fatal(err)
	}
	args := append(append([]string{}, cli[1:]...), "asm", "--backend=amd64", "--target="+target, dir)
	cmd := testtmp.Guard(exec.Command(cli[0], args...))
	cmd.Env = append(os.Environ(), "XO_PASSES="+passes, "XO_NOLIBC=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("xo %s: %v\n%s", strings.Join(args, " "), err, stderr.String())
	}
	return string(out)
}

var (
	reAliasJmp = regexp.MustCompile(`^\s*jmp (xo_f_\S+)$`)
	reThunk    = regexp.MustCompile(`\bxo_w64e?_`)
	reLocal    = regexp.MustCompile(`\.Lp\w+?_f\d+_`)
	reLiteral  = regexp.MustCompile(`\.Lxo_p\w+?_(str|lit|k)\d+`)
	reExport   = regexp.MustCompile(`\bxo_x_\w+`)
	reWinCRT   = regexp.MustCompile(`\bxo_f_cabi__wincrt_2e(\w+)`)
)

// exportAliases maps each export of Linux text to the function its alias
// jumps to (`xo_x_name:` then `jmp xo_f_...`).
func exportAliases(text string) map[string]string {
	m := map[string]string{}
	prev := ""
	for _, l := range strings.Split(text, "\n") {
		if g := reAliasJmp.FindStringSubmatch(l); g != nil && strings.HasPrefix(prev, "xo_x_") {
			m[prev] = g[1]
		}
		if strings.HasSuffix(l, ":") && !strings.HasPrefix(l, ".") && !strings.HasPrefix(l, "\t") {
			prev = strings.TrimSuffix(l, ":")
		} else if strings.TrimSpace(l) != "" && !strings.HasPrefix(strings.TrimSpace(l), ".") {
			prev = ""
		}
	}
	return m
}

// ownFuncsNorm is the normalized instructions of each function of text
// (`# func NAME` comments) that is the program's or an instance's.
func ownFuncsNorm(text string, alias map[string]string) map[string]string {
	out := map[string]string{}
	cur := ""
	var body []string
	flush := func() {
		if cur != "" && (!strings.Contains(strings.SplitN(cur, "[", 2)[0], ".") || strings.HasPrefix(cur, "xo.")) {
			out[cur] = strings.Join(foldStubs(body), "\n")
		}
	}
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, "# func ") {
			flush()
			cur, body = strings.TrimSpace(l[7:]), nil
			continue
		}
		s := strings.TrimSpace(l)
		if cur == "" || s == "" || strings.HasPrefix(s, ".") || strings.HasPrefix(s, "#") || strings.HasSuffix(s, ":") {
			continue
		}
		s = reThunk.ReplaceAllString(s, "")
		s = reWinCRT.ReplaceAllString(s, "$1")
		s = reLocal.ReplaceAllString(s, ".L_")
		s = reLiteral.ReplaceAllString(s, ".L$1")
		s = reExport.ReplaceAllStringFunc(s, func(x string) string {
			if f, ok := alias[x]; ok {
				return f
			}
			return x
		})
		body = append(body, s)
	}
	flush()
	return out
}

// foldStubs writes a Linux leaf's fault stub (a frame record, the moves,
// a call, ud2) as Windows writes it (the moves, a jump). The Linux stub's
// frame for the variables a fault report shows (x86emit.xo stub_saves,
// 3eae41c99: `subq $N, %rsp` and stores of registers to it) has no
// Windows twin (a Windows leaf's stub jumps with no frame, win_leaf), so
// it is dropped too, and so is a framed function's stub's (Windows makes
// none).
func foldStubs(b []string) []string {
	var r []string
	for i := 0; i < len(b); {
		if i > 0 && b[i] == "pushq %rbp" && i+1 < len(b) && b[i+1] == "movq %rsp, %rbp" {
			j := i + 2
			for j < len(b) && j < i+16 && !strings.HasPrefix(b[j], "callq ") {
				j++
			}
			if j+1 < len(b) && strings.HasPrefix(b[j], "callq ") && b[j+1] == "ud2" {
				for _, l := range b[i+2 : j] {
					if strings.HasPrefix(l, "subq $") && strings.HasSuffix(l, ", %rsp") {
						continue
					}
					if strings.HasPrefix(l, "mov") && strings.HasSuffix(l, "(%rsp)") {
						continue
					}
					r = append(r, l)
				}
				r = append(r, "jmp "+b[j][6:])
				i = j + 2
				continue
			}
		}
		// a framed function's stub: Linux saves the variables' registers
		// below rsp first (stub_saves), Windows does not (its unwind data
		// knows only the prologue's rsp, so a moved rsp lost the fault
		// past task.isolate: fault_audit, 2026-10-30)
		if strings.HasPrefix(b[i], "subq $") && strings.HasSuffix(b[i], ", %rsp") && i+1 < len(b) && strings.HasPrefix(b[i+1], "movq %") && strings.HasSuffix(b[i+1], "(%rsp)") {
			j := i + 1
			for j < len(b) && strings.HasPrefix(b[j], "movq %") && strings.HasSuffix(b[j], "(%rsp)") {
				j++
			}
			k := j
			for k < len(b) && k < j+16 && !strings.HasPrefix(b[k], "callq ") && !strings.HasPrefix(b[k], "subq ") {
				k++
			}
			if k+1 < len(b) && strings.HasPrefix(b[k], "callq ") && b[k+1] == "ud2" {
				i = j
				continue
			}
		}
		r = append(r, b[i])
		i++
	}
	return r
}

// firstDiff shows where two normalized bodies first differ.
func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; i < len(al) || i < len(bl); i++ {
		x, y := "", ""
		if i < len(al) {
			x = al[i]
		}
		if i < len(bl) {
			y = bl[i]
		}
		if x != y {
			return fmt.Sprintf("line %d: linux %q, windows %q", i+1, x, y)
		}
	}
	return ""
}
