package query

import (
	"strconv"
	"strings"
	"testing"
)

// TestImports covers `xo query imports` and `deps` (decision 0095): the
// `use` line of each edge, and the deny rule that forbids one.
func TestImports(t *testing.T) {
	s, _ := load(t, map[string]string{
		"xo.mod":           "module example.com/notes\n\ndeny example.com/notes/domain -> example.com/notes/api\n",
		"api/api.xo":       "pub fn name() -> Str { \"api\" }\n",
		"store/store.xo":   "use example.com/notes/api\n\npub fn get() -> Str { api.name() }\n",
		"domain/note.xo":   "use example.com/notes/store\nuse std/strings\n\npub fn f() -> Str { strings.to_upper(store.get()) }\n",
		"domain/extra.xo":  "use example.com/notes/api\n\npub fn g() -> Str { api.name() }\n",
		"domain/a_test.xo": "test \"f\" {\n  expect(f()) == \"API\"\n}\n",
		"server/server.xo": "use example.com/notes/domain\n\nfn main(os: Os) {\n  os.stdio.println(domain.f())\n}\n",
	})
	r, err := s.Imports("example.com/notes/domain", false)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, e := range r.Imports {
		l := e.Path + "@" + e.Pos.File[strings.LastIndex(e.Pos.File, "/")+1:] + ":" + strconv.Itoa(e.Pos.Line)
		if e.Std {
			l += " std"
		}
		if e.Denied != "" {
			l += " denied"
		}
		lines = append(lines, l)
	}
	want := "example.com/notes/api@extra.xo:1 denied|example.com/notes/store@note.xo:1|std/strings@note.xo:2 std"
	if got := strings.Join(lines, "|"); got != want {
		t.Errorf("imports:\n got %s\nwant %s", got, want)
	}
	if strings.Join(r.Packages, ",") != "example.com/notes/api,example.com/notes/store,std/strings" || r.Transitive {
		t.Errorf("result %+v", r)
	}

	d, err := s.Imports("example.com/notes/server", true)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.Packages, ",") != "example.com/notes/api,example.com/notes/domain,example.com/notes/store,std/strings" || !d.Transitive {
		t.Errorf("deps %+v", d)
	}
	// store's use of api is listed with store as the importer
	found := false
	for _, e := range d.Imports {
		if e.From == "example.com/notes/store" && e.Path == "example.com/notes/api" && e.Pos.Line == 1 && e.Denied == "" {
			found = true
		}
	}
	if !found {
		t.Errorf("deps lacks store -> api: %+v", d.Imports)
	}
	if _, err := s.Imports("example.com/notes/nope", false); err == nil {
		t.Error("imports of a missing package succeeded")
	}
}
