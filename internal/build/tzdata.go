package build

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/gen"
	"github.com/xo-lang/xo/internal/ssa"
)

// Zone data per use (std.md 13, decision 0094). gen.Program.TZ says which
// zones `time.zone` can be asked for; tzFiles writes package main's
// xo_tzdata.go, which registers them with the runtime:
//
//   - constant names: the TZif data of each zone (1 to 3 KB), extracted
//     from the zoneinfo.zip of the Go toolchain that builds the program;
//   - a name known only at run time: the whole zoneinfo.zip (about 400
//     KB, embedded with go:embed), and a note naming each such call;
//   - --tzdata=system: no data; the runtime reads the host database.

// TZDataSystem is the value of Options.TZData that reads the host's
// zone database at run time instead of embedding data.
const TZDataSystem = "system"

var (
	tzOnce sync.Once
	tzZip  []byte
	tzErr  error
)

// zoneinfoZip returns Go's lib/time/zoneinfo.zip: $XO_ZONEINFO, then the
// GOROOT of the go command the build uses.
func zoneinfoZip() ([]byte, error) {
	tzOnce.Do(func() { tzZip, tzErr = loadZoneinfo(goTool()) })
	return tzZip, tzErr
}

// tzFix is how to give a build zone data without a Go installation.
const tzFix = "set XO_ZONEINFO to a zoneinfo.zip, or build with --tzdata=system to read the host's zone database at run time"

// loadZoneinfo reads $XO_ZONEINFO, else the zoneinfo.zip under the GOROOT
// the go command goCmd reports. With no go command the error says how to
// build without Go (a native build needs no Go otherwise).
func loadZoneinfo(goCmd string) ([]byte, error) {
	path := os.Getenv("XO_ZONEINFO")
	if path == "" {
		out, err := exec.Command(goCmd, "env", "GOROOT").Output()
		if err != nil {
			if _, lerr := exec.LookPath(goCmd); lerr != nil {
				return nil, fmt.Errorf("zone data: the program names time zones and the build embeds them from Go's lib/time/zoneinfo.zip, but there is no go command; %s", tzFix)
			}
			return nil, fmt.Errorf("zone data: cannot find GOROOT (%s env GOROOT: %v); %s", goCmd, err, tzFix)
		}
		path = filepath.Join(strings.TrimSpace(string(out)), "lib", "time", "zoneinfo.zip")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("zone data: %v; %s", err, tzFix)
	}
	return data, nil
}

// zoneData returns the TZif data of the named zones; missing lists the
// names the database does not have.
func zoneData(zipData []byte, names []string) (map[string][]byte, []string, error) {
	r, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return nil, nil, fmt.Errorf("zone data: %v", err)
	}
	want := map[string]bool{}
	for _, n := range names {
		want[n] = true
	}
	out := map[string][]byte{}
	for _, f := range r.File {
		if !want[f.Name] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, nil, err
		}
		out[f.Name] = data
	}
	var missing []string
	for _, n := range names {
		if _, ok := out[n]; !ok {
			missing = append(missing, n)
		}
	}
	return out, missing, nil
}

// tzSel is the zone data a program embeds (tzSelect).
type tzSel struct {
	system bool              // --tzdata=system: none, the host's database
	zip    []byte            // the whole database (a name known at run time)
	names  []string          // the constant names that have data, sorted
	data   map[string][]byte // their TZif data
	notes  []string
}

// tzSelect chooses the zone data of a program that asks for use, for
// both the Go backend (tzFiles) and the native ones (tzSource). nil means
// none is needed.
func tzSelect(use gen.TZUse, mode string) (*tzSel, error) {
	if mode == TZDataSystem {
		if len(use.Names) == 0 && len(use.Dynamic) == 0 {
			return nil, nil
		}
		return &tzSel{system: true}, nil
	}
	var names []string
	for _, n := range use.Names {
		if n != "UTC" && n != "" && n != "Local" {
			names = append(names, n)
		}
	}
	if len(names) == 0 && len(use.Dynamic) == 0 {
		return nil, nil
	}
	zipData, err := zoneinfoZip()
	if err != nil {
		return nil, err
	}
	sel := &tzSel{}
	if len(use.Dynamic) > 0 {
		sel.zip = zipData
		for _, at := range use.Dynamic {
			sel.notes = append(sel.notes, fmt.Sprintf("%s: note: time.zone with a name known only at run time embeds the whole zone database (%d KB); pass a constant name to embed only that zone, or build with --tzdata=system to read the host's database", at, (len(zipData)+512)/1024))
		}
		return sel, nil
	}
	data, missing, err := zoneData(zipData, names)
	if err != nil {
		return nil, err
	}
	sel.data = data
	for _, n := range names {
		if _, ok := data[n]; ok {
			sel.names = append(sel.names, n)
		}
	}
	for _, n := range missing {
		sel.notes = append(sel.notes, fmt.Sprintf("note: zone %s is not in the zone database; time.zone returns ZoneErr.Unknown for it", strconv.Quote(n)))
	}
	return sel, nil
}

// tzFiles adds the zone data files of prog to files and returns the build
// notes.
func tzFiles(prog *gen.Program, mode string) ([]string, error) {
	sel, err := tzSelect(prog.TZ, mode)
	if sel == nil || err != nil {
		return nil, err
	}
	if sel.system {
		prog.Files["xo_tzdata.go"] = []byte("// Code generated by xo build. DO NOT EDIT.\n\npackage main\n\nimport \"" + gen.ModulePath + "/rt\"\n\nfunc init() { rt.TZUseSystem() }\n")
		return nil, nil
	}
	var b strings.Builder
	b.WriteString("// Code generated by xo build. DO NOT EDIT.\n\npackage main\n\n")
	if sel.zip != nil {
		b.WriteString("import (\n\t_ \"embed\"\n\n\t\"" + gen.ModulePath + "/rt\"\n)\n\n")
		b.WriteString("//go:embed xo_zoneinfo.zip\nvar xoZoneinfo string\n\n")
		prog.Files["xo_zoneinfo.zip"] = sel.zip
	} else {
		b.WriteString("import \"" + gen.ModulePath + "/rt\"\n\n")
	}
	b.WriteString("func init() {\n")
	if sel.zip != nil {
		b.WriteString("\trt.TZFull(xoZoneinfo)\n")
	}
	for _, n := range sel.names {
		fmt.Fprintf(&b, "\trt.TZEmbed(%s, %s)\n", strconv.Quote(n), strconv.Quote(string(sel.data[n])))
	}
	b.WriteString("}\n")
	prog.Files["xo_tzdata.go"] = []byte(b.String())
	return sel.notes, nil
}

// TZDataSource is the C file of a native build's zone data (tzSource).
const TZDataSource = "xo_tzdata.c"

// tzSource writes xo_tzdata.c, the zone data of a native build that uses
// std/time, into dir and returns its path; the build compiles it beside
// the runtime (std/llrt/xo_time.c declares its symbols), so the runtime's
// object does not depend on the program (decision 0157). xo_tz_mode is 0
// with the TZif data of each constant zone name, 1 with the whole
// zoneinfo.zip, or 2 for the host's database. It also returns the build
// notes, the same as the Go backend's.
func tzSource(dir string, use gen.TZUse, mode string) (string, []string, error) {
	sel, err := tzSelect(use, mode)
	if err != nil {
		return "", nil, err
	}
	var b strings.Builder
	b.WriteString("// Code generated by xo build. DO NOT EDIT.\n// The zone data of the program (decisions 0094, 0157).\n\n#include <stdint.h>\n\n")
	b.WriteString("typedef struct {\n\tconst char *name;\n\tconst char *data;\n\tint64_t len;\n} xo_tz_blob;\n\n")
	var notes []string
	tzMode := 0
	switch {
	case sel != nil && sel.system:
		tzMode = 2
	case sel != nil && sel.zip != nil:
		tzMode = 1
	}
	fmt.Fprintf(&b, "const int64_t xo_tz_mode = %d;\n", tzMode)
	if tzMode == 1 {
		fmt.Fprintf(&b, "const int64_t xo_tz_zip_len = %d;\nconst char xo_tz_zip[] =\n", len(sel.zip))
		cBytes(&b, sel.zip)
		b.WriteString(";\n")
		notes = sel.notes
	} else {
		b.WriteString("const int64_t xo_tz_zip_len = 0;\nconst char xo_tz_zip[1] = {0};\n")
	}
	b.WriteString("const xo_tz_blob xo_tz_blobs[] = {\n")
	if tzMode == 0 && sel != nil {
		for _, n := range sel.names {
			fmt.Fprintf(&b, "\t{%s,\n", strconv.Quote(n))
			cBytes(&b, sel.data[n])
			fmt.Fprintf(&b, "\t, %d},\n", len(sel.data[n]))
		}
		notes = sel.notes
	}
	b.WriteString("\t{0, 0, 0},\n};\n")
	path := filepath.Join(dir, TZDataSource)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", nil, err
	}
	return path, notes, nil
}

// cBytes writes data as C string literals: printable ASCII as is, other
// bytes as three digit octal escapes, 64 bytes a line.
func cBytes(b *strings.Builder, data []byte) {
	if len(data) == 0 {
		b.WriteString("\t\"\"\n")
		return
	}
	for i := 0; i < len(data); i += 64 {
		b.WriteString("\t\"")
		for _, c := range data[i:min(i+64, len(data))] {
			switch {
			case c == '"' || c == '\\' || c == '?':
				b.WriteByte('\\')
				b.WriteByte(c)
			case c >= 0x20 && c < 0x7f:
				b.WriteByte(c)
			default:
				fmt.Fprintf(b, "\\%03o", c)
			}
		}
		b.WriteString("\"\n")
	}
}

// timeFlags returns the C compiler flags, extra C sources, and build
// notes of a native build: when the program uses std/time, XO_TIME for
// std/llrt/xo_time.c and its zone data (xo_tzdata.c in dir, compiled
// beside the runtime); nothing otherwise, so other programs do not grow.
func timeFlags(dir string, res *check.Result, tz ssa.TZUse, opts Options) ([]string, []string, []string, error) {
	uses := false
	for _, m := range res.Info().AllModules() {
		if m.Path == "native/time" {
			uses = true
		}
	}
	if !uses {
		return nil, nil, nil, nil
	}
	if opts.TZData != "" && opts.TZData != "embedded" && opts.TZData != TZDataSystem {
		return nil, nil, nil, fmt.Errorf("unknown --tzdata=%s (want embedded or system)", opts.TZData)
	}
	src, notes, err := tzSource(dir, gen.TZUse(tz), opts.TZData)
	if err != nil {
		return nil, nil, nil, err
	}
	return []string{"-DXO_TIME"}, []string{src}, notes, nil
}

// IOFlags returns the C compiler flags that compile the runtime's Fs
// operations (std/llrt/xo_fs.c, XO_FS) and standard input reads
// (xo_stdin.c, XO_STDIN), and Proc.run (xo_proc.c, XO_PROC) into a
// native program whose code calls them
// (decision 0102), so programs without them do not grow.
func IOFlags(code []byte) []string {
	var out []string
	for _, sym := range []string{"xo_fs_stat", "xo_fs_names", "xo_fs_write", "xo_fs_rename", "xo_fs_remove", "xo_fs_mkdir_all", "xo_fs_chtimes", "xo_fs_create_temp", "xo_fs_cwd", "xo_fs_case_sensitive", "xo_fs_canceled", "xo_fs_info", "xo_time_of", "xo_time_ns",
		// handles, copies, modes, links, locks (decision 0172)
		"xo_fs_open", "xo_fs_file_", "xo_fs_copy", "xo_fs_perm", "xo_fs_chmod", "xo_fs_symlink", "xo_fs_read_link", "xo_fs_lock", "xo_fs_unlock"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_FS")
			break
		}
	}
	for _, sym := range []string{"xo_stdin_", "xo_tw_set_input", "xo_tw_set_tty", "xo_isatty", "xo_term_width"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_STDIN")
			break
		}
	}
	if callsSym(code, "xo_proc_") {
		// Proc.run and FakeProc.on_run (xo_proc.c, decision 0107)
		out = append(out, "-DXO_PROC")
	}
	// the C path of the Unicode operations (decision 0180), which only
	// XO_UNICODE=c builds call: no pack has it, so the runtime is compiled
	// locally
	for _, sym := range []string{"xo_str_lower", "xo_str_upper", "xo_str_eq_lower", "xo_rune_is_letter", "xo_rune_is_digit", "xo_rune_is_print", "xo_rune_to_case", "xo_rune_quote", "xo_bytes_quote", "xo_unicode_ranges"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_UNICODE_C")
			break
		}
	}
	// the C path of the number and time text (decision 0193), which only
	// XO_LLRT_C=text builds call: likewise compiled locally
	for _, sym := range []string{"xo_int_parse", "xo_rs_append_duration", "xo_rs_append_time", "xo_rs_append_quote", "xo_duration_parse", "xo_time_parse"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_TEXT_C")
			break
		}
	}
	// and of the Str operations (decision 0193, XO_LLRT_C=strings)
	for _, sym := range []string{"xo_str_trim", "xo_str_replace"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_STRINGS_C")
			break
		}
	}
	// and of the number text (decision 0193 step 3, XO_LLRT_C=num)
	for _, sym := range []string{"xo_float_parse", "xo_b64_encode", "xo_b64_ok", "xo_b64_decode", "xo_bytes_hex_ok", "xo_bytes_from_hex"} {
		if bytes.Contains(code, []byte(sym)) {
			out = append(out, "-DXO_NUM_C")
			break
		}
	}
	// and of the leaf text of Display and Debug (decision 0193 step 5,
	// XO_LLRT_C=show)
	for _, sym := range []string{"xo_rs_append_u64", "xo_rs_append_rune_quote"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_SHOW_C")
			break
		}
	}
	// and of the zone rules (decision 0193 step 5, XO_LLRT_C=tz)
	for _, sym := range []string{"xo_tz_load", "xo_tz_offset", "xo_tz_abbr"} {
		if callsSym(code, sym) {
			out = append(out, "-DXO_TZ_C")
			break
		}
	}
	return out
}

// callsSym reports whether code (arm64 assembly or LLVM IR) refers to the
// runtime symbol sym: preceded by what a reference has before it (`bl
// xo_f`, `bl _xo_f`, `@xo_f`, `:lo12:xo_f`, an operand after a comma or
// tab), not by the quote of a string literal (`.ascii "xo_f"`, `c"xo_f"`),
// which is the program's data. The self hosted compiler's own tables of
// these names (selfhost/xo link.xo io_flags, which does the same) turned
// on -DXO_UNICODE_C for xo1, which no pack variant has
// (gotchas/runtime-flags-from-string-literals.md).
func callsSym(code []byte, sym string) bool {
	for _, p := range symRefPrefixes {
		if bytes.Contains(code, []byte(p+sym)) {
			return true
		}
	}
	return false
}

var symRefPrefixes = []string{" ", "\t", "_", "@", ":", ","}
