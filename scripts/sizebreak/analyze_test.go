package main

import (
	"strings"
	"testing"
)

// TestGapsAndGroups: sizes are the gaps between symbol addresses, the gaps
// must cover the text section, one symbol per address is sized and the rest
// count as aliases, and the groups are module, hidden and non-Xo.
func TestGapsAndGroups(t *testing.T) {
	bin := &execFile{
		Name:   "prog",
		Format: "Mach-O darwin/arm64",
		Arch:   "arm64",
		Text:   TextInfo{Addr: 0x1000, Size: 0x100},
		Syms: []Symbol{
			{Name: "_xo_f_main_2erun", Addr: 0x1000},
			{Name: "_xo_f_x509_2eparse", Addr: 0x1040},
			{Name: "_xo_f_x509_2eparse", Addr: 0x1040}, // alias
			{Name: "_xo_hhMzpO2fs7S", Addr: 0x1080},
			{Name: "_main", Addr: 0x10f0},
		},
	}
	rep, err := analyze(bin)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Sum != 0x100 || rep.Text.Size != 0x100 {
		t.Fatalf("sum %#x, text %#x", rep.Sum, rep.Text.Size)
	}
	if rep.Symbols != 4 || rep.Aliases != 1 {
		t.Fatalf("symbols %d aliases %d", rep.Symbols, rep.Aliases)
	}
	want := []struct {
		name  string
		bytes uint64
	}{
		{"main", 0x40}, {X509Group, 0x40}, {Hidden, 0x70}, {NonXo, 0x10},
	}
	groups := map[string]uint64{}
	for _, g := range rep.Groups {
		groups[g.Name] = g.Bytes
	}
	for _, w := range want {
		if groups[w.name] != w.bytes {
			t.Errorf("group %s: %d, want %d (all %v)", w.name, groups[w.name], w.bytes, groups)
		}
	}
}

// X509Group is the module name of std/x509's symbols, used above.
const X509Group = "x509"

// TestGapFails: a text start with no symbol leaves bytes unaccounted for,
// and the tool fails instead of inventing a size.
func TestGapFails(t *testing.T) {
	bin := &execFile{
		Name: "prog",
		Text: TextInfo{Addr: 0x1000, Size: 0x40},
		Syms: []Symbol{{Name: "_xo_f_main_2erun", Addr: 0x1010}},
	}
	if _, err := analyze(bin); err == nil {
		t.Fatal("a symbol table that does not cover the text section was accepted")
	}
}

// TestMangling: module, demangling and the Map element type.
func TestMangling(t *testing.T) {
	cases := []struct {
		sym, module, group string
	}{
		{"_xo_f_x509_2eparse", "x509", "x509"},
		{"_xo_f_cabi__tls_2esrvtls__h2", "cabi__tls", "cabi__tls"},
		{"_xo_hhMzpO2fs7S", "", Hidden},
		{"_xo_f_drop_20Str", "", Glue},
		{"_main", "", NonXo},
		// ELF: no leading underscore (linux/amd64 and linux/arm64)
		{"xo_f_x509_2eparse", "x509", "x509"},
		{"xo_f_cabi__tls_2esrvtls__h2", "cabi__tls", "cabi__tls"},
		{"xo_hhMzpO2fs7S", "", Hidden},
		{"xo_f_drop_20Str", "", Glue},
		{"main", "", NonXo},
		{"memcpy", "", NonXo},
	}
	for _, c := range cases {
		if m, ok := moduleOf(c.sym); c.module != "" && (!ok || m != c.module) {
			t.Errorf("%s: module %q (ok %v), want %q", c.sym, m, ok, c.module)
		}
		if c.module == "" && c.group != Glue && func() bool { _, ok := moduleOf(c.sym); return ok }() {
			t.Errorf("%s: reported a module", c.sym)
		}
		if g := groupOf(c.sym); g != c.group {
			t.Errorf("%s: group %q, want %q", c.sym, g, c.group)
		}
	}
	if got := demangle("_xo_f_x509_2eparse"); got != "_xo_f_x509.parse" {
		t.Errorf("demangle: %q", got)
	}
	// the module starts with two hex letters: only the function part is
	// unescaped, or "_ae" would become a byte
	if got := displayName("_xo_f_aesgcm_2eGcm_2egcm__bulk"); got != "aesgcm.Gcm.gcm__bulk" {
		t.Errorf("display: %q", got)
	}
	if got := displayName("xo_f_aesgcm_2eGcm_2egcm__bulk"); got != "aesgcm.Gcm.gcm__bulk" {
		t.Errorf("display of an ELF symbol: %q", got)
	}
	if got := displayName("_xo_f_xo_2emlog_2eprobe_5bMap_5bStr_2c_20Int_5d_5d"); got != "xo.mlog.probe[Map[Str, Int]]" {
		t.Errorf("display of a Map family: %q", got)
	}
	if got := displayName("_xo_f_drop_20List_5bInt_5d"); got != "drop List[Int]" {
		t.Errorf("display of a glue function: %q", got)
	}
	if got := mapType(demangle("_xo_f_xo_2emlog_2eprobe_5bxo_2eMapLog_5bMap_5bStr_2c_20xo_2edyn_5d_5d_5d")); got != "Map[Str, xo.dyn]" {
		t.Errorf("map type: %q", got)
	}
}

// TestMapFamilies: the three families are broken out and keep the element
// types they are instantiated for, largest first.
func TestMapFamilies(t *testing.T) {
	bin := &execFile{
		Name: "prog",
		Text: TextInfo{Addr: 0x1000, Size: 0x100},
		Syms: []Symbol{
			{Name: "_xo_f_xo_2emap_2enew_5bMap_5bStr_2c_20Int_5d_5d", Addr: 0x1000},
			{Name: "_xo_f_xo_2emlog_2eprobe_5bMap_5bStr_2c_20xo_2edyn_5d_5d", Addr: 0x1020},
			{Name: "_xo_f_xo_2emlog_2eset_5bMap_5bStr_2c_20xo_2edyn_5d_5d", Addr: 0x1060},
			{Name: "_xo_f_xo_2emaph_2eget_5bMap_5bInt_2c_20User_5d_5d", Addr: 0x10c0},
		},
	}
	rep, err := analyze(bin)
	if err != nil {
		t.Fatal(err)
	}
	fams := map[string]Family{}
	for _, f := range rep.Families {
		fams[f.Name] = f
	}
	if fams["xo.mlog"].Bytes != 0xa0 || len(fams["xo.mlog"].Top) != 1 {
		t.Errorf("mlog family: %+v", fams["xo.mlog"])
	}
	if fams["xo.mlog"].Top[0].Type != "Map[Str, xo.dyn]" || fams["xo.mlog"].Top[0].Bytes != 0xa0 {
		t.Errorf("mlog instantiation: %+v", fams["xo.mlog"].Top)
	}
	if fams["xo.maph"].Top[0].Type != "Map[Int, User]" {
		t.Errorf("maph instantiation: %+v", fams["xo.maph"].Top)
	}
	if !strings.HasPrefix(fams["xo.map"].Top[0].Type, "Map[") {
		t.Errorf("map instantiation: %+v", fams["xo.map"].Top)
	}
}
