package xotool

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The classes of a setting the driver or the code generation reads
// (decision 0275, "Audit of the inputs outside the closure"). keyed: a
// key header holds it (the token names where); bypass: the backend caches
// are not used when it is set (the token is in the bypass condition);
// content: it changes which functions or modules exist, and every entry
// is keyed by its function's or module's content; link: the cc and link
// steps, whose object and binary keys hold their flags and inputs (the
// binary cache keeps self_id); none: no effect on an entry's bytes
// (paths, jobs, reporting, cache and test switches).
type inputClass struct {
	class string
	token string
}

// codegenEnv classifies every environment variable compiler/xo and the
// code generation closure read. A new one fails TestCodegenInputsClassified
// until it is added here: if it can change generated code, it goes into a
// key header (or the bypass) first.
var codegenEnv = map[string]inputClass{
	"XO_ARM64_REGS":           {"keyed", "regs=${m.arm64_regs}"},
	"XO_POISON_FRAMES":        {"keyed", "poison=${m.poison_frames}"},
	"XO_HEAP_GUARD":           {"keyed", "guard=${m.heap_guard}"},
	"XO_NOLIBC":               {"keyed", "nolibc=${m.nolibc}"},
	"XO_INLINE_DENY":          {"keyed", "deny ${m.inline_deny"},
	"XO_PASSES":               {"bypass", "!gm.passes_off.is_empty()"},
	"XO_NATIVE_RACE":          {"bypass", "gm.race"},
	"XO_MAP_LOG":              {"keyed", "maplog=${m.map_log}"},
	"XO_LOWER":                {"keyed", "lower=${c.lower_off"}, // the front end key (bcache.xo); the SSA bodies the backend keys hash differ too
	"XO_PREEMPT_POLLS":        {"keyed", "nopreempt=${m.no_preempt}"},
	"XO_ASYNC_PREEMPT":        {"keyed", "async=${m.async_preempt}"},
	"XO_INLINE_BREAK":         {"none", ""}, // breaks the inlined module on purpose; verify then fails the build
	"XO_DCE":                  {"content", ""},
	"XO_LLRT_C":               {"content", ""},
	"XO_CHECK_EAGER":          {"content", ""},
	"XO_CHECK_NO_PARSE_CACHE": {"none", ""},
	"XO_STD":                  {"content", ""},
	"XO_ZONEINFO":             {"content", ""},
	"XO_CBIND_DIR":            {"content", ""},
	"XO_GOBIND":               {"content", ""},
	"XO_GOBIND_DIR":           {"content", ""},
	"XO_MODCACHE":             {"content", ""},
	"XO_OFFLINE":              {"none", ""},
	"XO_WORK":                 {"content", ""},
	"XO_LLVM_OPT":             {"link", ""},
	"XO_LLVM_LTO":             {"link", ""},
	"XO_LLVM_SPLIT":           {"link", ""},
	"XO_LLVM_TEST_SPLIT":      {"link", ""},
	"XO_LLVM_JOBS":            {"none", ""},
	"XO_SYSROOT":              {"link", ""},
	"XO_LINUXSYSROOT":         {"link", ""},
	"XO_WINSYSROOT":           {"link", ""},
	"XO_WIN_NOCRT":            {"link", ""},
	"XO_TSAN_RUNTIME":         {"link", ""},
	"XO_RACE_RUNTIME":         {"link", ""}, // the runtime's path, in macho_bin_key
	"XO_CC":                   {"link", ""},
	"XO_CLANG":                {"link", ""},
	"XO_GO":                   {"link", ""},
	"XO_GOCACHE":              {"link", ""},
	"XO_GOTIMERS":             {"link", ""},
	"XO_EMIT_JOBS":            {"none", ""}, // the parts never depend on the task count
	"XO_BACKEND_CACHE":        {"none", ""},
	"XO_BACKEND_CACHE_DIR":    {"none", ""},
	"XO_BIN_CACHE":            {"none", ""},
	"XO_CHK_CACHE":            {"none", ""},
	"XO_FE_CACHE":             {"none", ""},
	"XO_FE_STATS":             {"none", ""},
	"XO_SHARED_PACK":          {"none", ""},
	"XO_FE_BREAK":             {"none", ""},
	"XO_FE_KEEP_DIR":          {"none", ""},
	"XO_BUILD_DIR":            {"none", ""},
	"XO_INLINE_REPORT":        {"none", ""},
	"XO_DUMP_FN":              {"none", ""}, // writes a function's SSA and MIR to a file; the code is the same
	"XO_RC_CHECK":             {"none", ""}, // decision 0322's checks: marks Reuse values in the SSA bodies every cache key hashes
	"XO_DUMP_DIR":             {"none", ""}, // where those files go
	"XO_INLINE_STRICT":        {"none", ""},
	"XO_ASSUME_CHECK":         {"none", ""}, // decision 0342's check, off for its --ab
	"XO_ASSUME_BREAK":         {"none", ""}, // its test's damage
	"XO_TEST_BATCH":           {"none", ""},
	"XO_TEST_JOBS":            {"none", ""},
	"XO_TEST_ONLY":            {"none", ""},
	"XO_TRACEBACK":            {"none", ""},
	"XO_MAX_MEM":              {"none", ""},
	"XO_WARNINGS":             {"none", ""},
	"XO_LAZY_INLINE":          {"none", ""}, // off decodes every inline hit at once; the module is the same
	"XO_BORROW_CHECK":         {"none", ""}, // runs the full inference too and compares; the flags are the same
	"XO_KEEP_ASM":             {"none", ""}, // writes and keeps the assembly files only
	"XO_CACHE_PRUNE_BUDGET":   {"none", ""}, // how long a build prunes its caches
	"XO_PRUNE_DETACH":         {"cache", ""},
	"XO_PRUNE_FORCE":          {"none", ""}, // allows a back-end prune of the user cache below 64 MiB
	"XO_PROF_SELF":            {"none", ""}, // xo build --profile's own run of xo (no re-run)
	"XO_PARSE_STATS":          {"none", ""}, // --phase-times lists the parses the prefetch missed
	"XO_OP_VERIFY":            {"none", ""}, // only fails a build (decision 0274 step 3); a cache hit skips the check: verify with XO_BACKEND_CACHE=off
}

// codegenFields classifies the module fields compiler/xo sets.
var codegenFields = map[string]inputClass{
	"comp_dir":      {"none", ""}, // only in the .debug_str path slots, filled after the caches (arm64 debug.xo dw_slots_obj, 2026-10-22)
	"std_root":      {"none", ""}, // only in the .debug_str path slots, filled after the caches (arm64 debug.xo dw_slots_obj, 2026-10-22)
	"debug_info":    {"keyed", "debug=${m.debug_info}"},
	"dw_data_addrs": {"keyed", "dwaddr=${m.dw_data_addrs}"},
	"nolibc":        {"keyed", "nolibc=${m.nolibc}"},
	"nolibc_start":  {"keyed", "nolibc=${m.nolibc}"}, // nolibc and not wasm
	"poison_frames": {"keyed", "poison=${m.poison_frames}"},
	"race":          {"keyed", "race=${m.race}"},
	"inline_deny":   {"keyed", "deny ${m.inline_deny"},
	"passes_off":    {"keyed", "passes ${m.passes_off"},
	"dump_fn":       {"keyed", "dumpfn=${m.dump_fn}"}, // XO_DUMP_FN: the code is the same, but a cached function would not be generated and dumped
	"rc_check":      {"none", ""},                     // XO_RC_CHECK: marks Reuse values in the SSA bodies every cache key hashes
	"jobs":          {"none", ""},                     // the tasks of may_fault's scan; its result does not depend on them
	"op_verify":     {"none", ""},                     // XO_OP_VERIFY: checks, never changes, the emitter's output
}

// codegenFS are the closure's files that use the file system, with why
// that is not an input of the code generation's entries. A new one fails
// the test until it is classified here.
var codegenFS = map[string]string{
	"arm64/main.xo":             "the standalone tool's main, not linked into xo's builds",
	"llvm/main.xo":              "the standalone tool's main",
	"macho/main.xo":             "the standalone tool's main",
	"ssa/main.xo":               "the standalone tool's main",
	"wasm/main.xo":              "the standalone tool's main",
	"wasm/play.xo":              "the playground's own build",
	"wasm/rt4.xo":               "the wasm runtime, read as program input",
	"ssa/build.xo":              "reads the program's sources (program input, in the entries' keys)",
	"ssa/expr.xo":               "reads the program's sources",
	"parser/main.xo":            "reads the program's sources",
	"parser/ast.xo":             "reads the program's sources",
	"lexer/main.xo":             "reads the program's sources",
	"check/bindmiss.xo":         "module loading: program input",
	"check/cli.xo":              "module loading: program input",
	"check/call.xo":             "a diagnostic note only (an unimported std module's file exists): no code",
	"check/cimport.xo":          "C bindings: program input",
	"check/decl.xo":             "module loading: program input",
	"check/deps.xo":             "module loading: program input",
	"check/effects.xo":          "module loading: program input",
	"check/goimport.xo":         "Go bindings: program input",
	"check/load.xo":             "module loading: program input",
	"check/main.xo":             "module loading: program input",
	"check/modcache.xo":         "the check cache itself",
	"check/modfile.xo":          "xo.mod: program input",
	"check/locate.xo":           "finds std: program input",
	"check/stmt.xo":             "module loading: program input",
	"check/query.xo":            "module loading: program input",
	"check/vouch.xo":            "module loading: program input",
	"cli/bind/cbind.xo":         "C bindings: program input",
	"cli/bind/cli.xo":           "C bindings: program input",
	"cli/bind/clang.xo":         "C bindings: program input",
	"cli/core/format/main.xo":   "the formatter's own files",
	"tzsel/tzsel.xo":            "time zone data the program embeds: program input",
	"check/embed.xo":            "embedded files (decision 0304): their digest is in the module's interface closure (iface_closures embed_digest), which keys the module check cache and front-end entries",
	"cli/cmdhelp/cmdhelp.xo":    "help text",
	"cli/cmdhelp/main.xo":       "help text",
	"macho/pelink.xo":           "the link (binary cache, self_id)",
	"macho/link.xo":             "the link (binary cache, self_id)",
	"macho/elflink.xo":          "the link (binary cache, self_id)",
	"llvm/gen.xo":               "program input",
	"wasm/gen.xo":               "program input",
	"arm64/race.xo":             "program input",
	"ssa/fecache.xo":            "the front end cache itself",
	"ssa/rccache.xo":            "the rc cache itself",
	"cli/core/format/format.xo": "the formatter's own files",
}

var (
	envRe   = regexp.MustCompile(`env\.get\("([A-Z_0-9]+)"\)`)
	fieldRe = regexp.MustCompile(`\b(?:gm|lm|mi|m)\.([a-z_]+) = `)
	fsRe    = regexp.MustCompile(`os\.fs\b|\bfs\.read|uses fs\.`)
)

func codegenRepoRoot(t *testing.T) string {
	t.Helper()
	r, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// TestCodegenInputsClassified: every environment variable compiler/xo and
// the closure read, every module field the driver sets, and every closure
// file that touches the file system is classified above, and every keyed
// one's token is in a key header (decision 0275's guard test).
func TestCodegenInputsClassified(t *testing.T) {
	root := codegenRepoRoot(t)
	cdir := filepath.Join(root, CompilerDir(root))
	mods, err := CodegenClosure(root)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, m := range append([]string{"xo"}, mods...) {
		fs, err := moduleFiles(filepath.Join(cdir, m))
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, fs...)
	}
	var headers strings.Builder
	for _, h := range []string{"arm64/fcache.xo", "arm64/partcache.xo", "arm64/inlcache.xo", "ssa/rccache.xo", "ssa/fecache.xo"} {
		b, err := os.ReadFile(filepath.Join(cdir, h))
		if err != nil {
			t.Fatal(err)
		}
		headers.Write(b)
	}
	main, err := os.ReadFile(filepath.Join(cdir, "xo", "main.xo"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(what, name string, c inputClass, ok bool) {
		if !ok {
			t.Errorf("%s %s is not classified (internal/xotool codegen_test.go): put it in a key header or the cache bypass if it can change generated code, then classify it", what, name)
			return
		}
		switch c.class {
		case "keyed":
			if !strings.Contains(headers.String(), c.token) {
				t.Errorf("%s %s is classified keyed but no key header holds %q", what, name, c.token)
			}
		case "bypass":
			if !strings.Contains(string(main), c.token) {
				t.Errorf("%s %s is classified bypass but compiler/xo main.xo has no %q", what, name, c.token)
			}
		}
	}
	envs := map[string]bool{}
	var fsFiles []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(cdir, f)
		rel = filepath.ToSlash(rel)
		for _, m := range envRe.FindAllSubmatch(b, -1) {
			if n := string(m[1]); strings.HasPrefix(n, "XO_") {
				envs[n] = true
			}
		}
		if strings.HasPrefix(rel, "xo/") {
			for _, m := range fieldRe.FindAllSubmatch(b, -1) {
				n := string(m[1])
				c, ok := codegenFields[n]
				check("module field", n, c, ok)
			}
		} else if fsRe.Match(b) {
			fsFiles = append(fsFiles, rel)
		}
	}
	var names []string
	for n := range envs {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		c, ok := codegenEnv[n]
		check("environment variable", n, c, ok)
	}
	for _, f := range fsFiles {
		if _, ok := codegenFS[f]; !ok {
			t.Errorf("%s uses the file system and is not classified (codegenFS): say why what it reads is not an input of the code generation's cache entries, or key it", f)
		}
	}
}

// copyCodegenTree copies the closure's modules, compiler/xo, and cli/core's
// cli.xo of root's compiler into a new directory, as a tree CodegenKey
// reads.
func copyCodegenTree(t *testing.T, root string) string {
	t.Helper()
	cd := CompilerDir(root)
	mods, err := CodegenClosure(root)
	if err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	cp := func(rel string) {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dst, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range append([]string{"xo"}, mods...) {
		es, err := os.ReadDir(filepath.Join(root, cd, m))
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range es {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".xo") {
				cp(filepath.Join(cd, m, e.Name()))
			}
		}
	}
	cp(filepath.Join(cd, "cli", "core", "cli.xo"))
	return dst
}

func appendCodegen(t *testing.T, path, s string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(s); err != nil {
		t.Fatal(err)
	}
}

// TestCodegenKeyChanges: a change to any non-test file of any closure
// module, or to the version, changes the key; a change to compiler/xo or
// to a test file does not; a `use` the closure cannot resolve fails
// closed (no key, and StampArgs sets none).
func TestCodegenKeyChanges(t *testing.T) {
	root := codegenRepoRoot(t)
	cd := CompilerDir(root)
	tree := copyCodegenTree(t, root)
	base, err := CodegenKey(tree)
	if err != nil {
		t.Fatal(err)
	}
	if real, err := CodegenKey(root); err != nil || real != base {
		t.Fatalf("the copy's key differs from the tree's (%v)", err)
	}
	mods, _ := CodegenClosure(tree)
	for _, m := range mods {
		fs, err := moduleFiles(filepath.Join(tree, cd, m))
		if err != nil || len(fs) == 0 {
			t.Fatalf("%s: %v", m, err)
		}
		f := fs[len(fs)-1]
		orig, _ := os.ReadFile(f)
		appendCodegen(t, f, "\n// a change\n")
		if k, err := CodegenKey(tree); err != nil || k == base {
			t.Errorf("a change to %s/%s left the key (%v)", m, filepath.Base(f), err)
		}
		os.WriteFile(f, orig, 0o644)
	}
	same := func(what, rel, s string) {
		t.Helper()
		p := filepath.Join(tree, rel)
		orig, err := os.ReadFile(p)
		if err != nil {
			os.WriteFile(p, nil, 0o644)
		}
		appendCodegen(t, p, s)
		if k, err := CodegenKey(tree); err != nil || k != base {
			t.Errorf("%s changed the key (%v)", what, err)
		}
		if orig == nil {
			os.Remove(p)
		} else {
			os.WriteFile(p, orig, 0o644)
		}
	}
	same("a change to compiler/xo", filepath.Join(cd, "xo", "main.xo"), "\n// a change\n")
	same("a new test file", filepath.Join(cd, "arm64", "zz_test.xo"), "// a test\n")
	cli := filepath.Join(tree, cd, "cli", "core", "cli.xo")
	orig, _ := os.ReadFile(cli)
	os.WriteFile(cli, []byte(strings.Replace(string(orig), `pub const VERSION = "`, `pub const VERSION = "9`, 1)), 0o644)
	if k, err := CodegenKey(tree); err != nil || k == base {
		t.Errorf("a new version left the key (%v)", err)
	}
	os.WriteFile(cli, orig, 0o644)
	// fail closed
	appendCodegen(t, filepath.Join(tree, cd, "ssa", "ir.xo"), "\nuse example.com/elsewhere\n")
	if k, err := CodegenKey(tree); err == nil {
		t.Errorf("an unresolvable use gave a key %s", k)
	}
	for _, a := range StampArgs(tree, "seed") {
		if strings.HasPrefix(a, "XO_CODEGEN_KEY=") {
			t.Errorf("StampArgs set %s for a tree whose closure cannot be computed", a)
		}
	}
}
