package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestOutsideBytesValidated keeps decision 0259's rule in place: every
// boundary where outside bytes (a request, a response, a file, the
// environment, standard input, a child's output, an archive) become a Str
// goes through the rule's helper, scan.xo's text_at, text_of, text,
// url_text_at, url_text on the native backends and text.go's textOf and
// urlTextOf on the Go backend (std/rt). Each boundary below must call one
// of them; a boundary rewritten without it fails here. A new conversion
// with rt.cast[Bytes, Str] in std/native fails too unless the file's count
// below is raised with a reason: that cast is how unvalidated bytes
// became Str before (prelude.xo's fs_read_text).
func TestOutsideBytesValidated(t *testing.T) {
	native := []string{"scan.text_at(", "scan.text_of(", "scan.text(", "scan.url_text_at(", "scan.url_text(", "texts("}
	goRT := []string{"textOf(", "urlTextOf(", "textsOf("}
	boundaries := []struct {
		file, fn string
		helpers  []string
	}{
		{"std/native/llrt/http1.xo", "parse_head", native},
		{"std/native/llrt/neturl.xo", "unescape", native},
		{"std/native/llrt/h2srv.xo", "fragment", native},
		{"std/native/llrt/hclient.xo", "headers", native},
		{"std/native/prelude.xo", "fs_read_text", native},
		{"std/native/prelude.xo", "file_read_line", native},
		{"std/native/prelude.xo", "child_read_line", native},
		{"std/native/prelude.xo", "stdio_read_line", native},
		{"std/native/prelude.xo", "stdio_read_all", native},
		{"std/native/prelude.xo", "llrt_env_args", native},
		{"std/native/prelude.xo", "llrt_env_program", native},
		{"std/native/prelude.xo", "llrt_env_get", native},
		{"std/native/prelude.xo", "llrt_env_vars", native},
		{"std/native/prelude.xo", "fs_cwd_path", native},
		{"std/native/prelude.xo", "fs_read_link", native},
		{"std/native/prelude.xo", "fs_read_dir", native},
		{"std/native/prelude.xo", "fs_walk_dir", native},
		{"std/native/archive.xo", "parse_cstr", native},
		{"std/native/archive.xo", "parse_pax", native},
		{"std/native/archive.xo", "zip_dir_header", native},
		{"std/rt/caps.go", "M_read_text", goRT},
		{"std/rt/caps.go", "M_cwd_path", goRT},
		{"std/rt/caps.go", "readDir", goRT},
		{"std/rt/caps.go", "M_get", goRT},
		{"std/rt/caps.go", "vars", goRT},
		{"std/rt/caps.go", "M_args", goRT},
		{"std/rt/caps.go", "M_program", goRT},
		{"std/rt/caps.go", "M_home", goRT},
		{"std/rt/caps.go", "readLine", goRT},
		{"std/rt/caps.go", "readAll", goRT},
		{"std/rt/fsfile.go", "M_read_line", goRT},
		{"std/rt/fsfile.go", "M_read_link", goRT},
		{"std/rt/fsfile.go", "fsGlob", goRT},
		{"std/rt/proccmd.go", "M_read_line", goRT},
		{"std/rt/http.go", "urlOf", goRT},
		{"std/rt/http_net.go", "serve", goRT},
		{"std/rt/http_net.go", "responseFields", goRT}, // send and do read the header through it
		{"std/rt/archive.go", "Archive_untar", goRT},
		{"std/rt/archive.go", "Archive_unzip", goRT},
	}
	for _, b := range boundaries {
		src, err := os.ReadFile(filepath.Join(root, b.file))
		if err != nil {
			t.Fatal(err)
		}
		body, ok := topLevelBody(string(src), b.fn, strings.HasSuffix(b.file, ".go"))
		if !ok {
			t.Errorf("%s: no function %s (decision 0259's boundary list is out of date)", b.file, b.fn)
			continue
		}
		found := false
		for _, h := range b.helpers {
			if strings.Contains(body, h) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s: %s makes a Str of outside bytes without the decision 0259 helper (one of %v)", b.file, b.fn, b.helpers)
		}
	}

	// rt.cast[Bytes, Str] in std/native: the current uses, each internal
	// or already validated
	casts := map[string]int{
		"std/native/path.xo":         3, // cut and clean: ranges of the Str path cut at `/` and `.` bytes, and `/` and `.`, valid as the path is
		"std/native/csv.xo":          1, // field_str: a slice of the Str text at a field boundary, valid as the text is
		"std/native/idna.xo":         1, // encode: the Punycode prefix, the ASCII runes and the digits of a valid Str label, valid by construction
		"std/native/norm.xo":         1, // normalize: normalized runes and copied runs of a valid Str, valid by construction
		"std/native/url.xo":          1, // escape: an ASCII only buffer (every byte outside the kept set is escaped)
		"std/native/html.xo":         1, // escape_string: the Str text with ASCII replacements between its own runs, valid as the text is
		"std/native/archive.xo":      1, // parse_pax: records parsed on raw bytes, keys and values validated
		"std/native/testing.xo":      3, // test fakes: the program's own text
		"std/native/time.xo":         3, // format_items and format_stream: ASCII digits and names and the layout's own text; format_rfc3339: ASCII digits and the zone offset
		"std/native/netip.xo":        2, // string4 and string6: digits, dots, colons, hex and a valid zone Str
		"std/native/llrt/netconn.xo": 1,
		"std/native/llrt/netwasi.xo": 1, // conn_read on wasm: a read's bytes are the program's data, as netconn's read_list
		"std/native/llrt/raw.xo":     2, // the conversion helpers themselves
		"std/native/llrt/scan.xo":    1, // text_of: validates
		"std/native/internal.xo":     1, // uuid_text_words: 36 ASCII bytes it wrote itself (hex digits and dashes)
		"std/native/prelude.xo":      1, // bytebuffer_read_until: an unchecked view of Bytes, cast back to Bytes in the same expression (no Str escapes)
	}
	files, _ := filepath.Glob(filepath.Join(root, "std/native/*.xo"))
	more, _ := filepath.Glob(filepath.Join(root, "std/native/llrt/*.xo"))
	for _, f := range append(files, more...) {
		if fi, err := os.Stat(f); err == nil && fi.IsDir() {
			continue // std/native/.xo: the `use c` binding cache (decision 0534)
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(root, f)
		rel = filepath.ToSlash(rel)
		if n := strings.Count(string(src), "rt.cast[Bytes, Str]"); n > casts[rel] {
			t.Errorf("%s: %d rt.cast[Bytes, Str] (at most %d): outside bytes become Str through scan.text_of (decision 0259); raise the count here with a reason only for internal bytes", rel, n, casts[rel])
		}
	}
}

// topLevelBody is the text of the top level function (Xo `fn`, Go `func`,
// either possibly a method) named fn in src, up to its closing brace.
func topLevelBody(src, fn string, goFile bool) (string, bool) {
	var re *regexp.Regexp
	if goFile {
		re = regexp.MustCompile(`(?m)^func (\([^)]*\) )?` + regexp.QuoteMeta(fn) + `[\[(]`)
	} else {
		re = regexp.MustCompile(`(?m)^(pub )?fn (\([^)]*\) )?` + regexp.QuoteMeta(fn) + `[\[(]`)
	}
	loc := re.FindStringIndex(src)
	if loc == nil {
		return "", false
	}
	rest := src[loc[0]:]
	end := strings.Index(rest, "\n}\n")
	if end < 0 {
		return rest, true
	}
	return rest[:end], true
}
