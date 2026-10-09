package gen_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/gen"
)

// TestZoneDataPerUse checks that std/time zone data is embedded per use
// (decision 0094): nothing for UTC only programs, the TZif data of each
// constant zone name, and the whole database, with a note naming the
// call, for a name known only at run time; --tzdata=system embeds
// nothing and reads the host database.
func TestZoneDataPerUse(t *testing.T) {
	tzdata := func(t *testing.T, res *build.Result) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(res.Dir, "xo_tzdata.go"))
		if os.IsNotExist(err) {
			return ""
		}
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	t.Run("utc", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "main.xo"), []byte("use std/time\n\nfn main(os: Os) {\n  os.stdio.println(time.format(Time.UNIX_EPOCH, time.utc(), time.RFC3339))\n  os.stdio.println(time.format(Time.UNIX_EPOCH, time.fixed(\"X\", 2h), time.RFC3339))\n}\n"), 0o644)
		got, res := buildAndRunOpts(t, dir, build.Options{Mode: gen.Build})
		if got.stdout != "1970-01-01T00:00:00Z\n1970-01-01T02:00:00+02:00\n" {
			t.Errorf("stdout %q", got.stdout)
		}
		if src := tzdata(t, res); src != "" || len(res.Notes) > 0 {
			t.Errorf("a UTC only program embeds zone data: %q, notes %q", src, res.Notes)
		}
	})

	t.Run("constant", func(t *testing.T) {
		_, res := buildAndRunOpts(t, filepath.Join("testdata", "golden", "time_zones"), build.Options{Mode: gen.Build})
		src := tzdata(t, res)
		for _, want := range []string{`rt.TZEmbed("America/New_York", "TZif`, `rt.TZEmbed("Europe/Berlin", "TZif`} {
			if !strings.Contains(src, want) {
				t.Errorf("xo_tzdata.go lacks %s", want)
			}
		}
		if strings.Count(src, "rt.TZEmbed(") != 2 || strings.Contains(src, "TZFull") {
			t.Errorf("xo_tzdata.go embeds more than the two zones:\n%.300s", src)
		}
		if len(res.Notes) != 2 || !strings.Contains(strings.Join(res.Notes, "\n"), `zone "Mars/Olympus_Mons" is not in the zone database`) {
			t.Errorf("notes %q", res.Notes)
		}
	})

	t.Run("dynamic", func(t *testing.T) {
		_, res := buildAndRunOpts(t, filepath.Join("testdata", "golden", "time_dynamic_zone"), build.Options{Mode: gen.Build})
		if src := tzdata(t, res); !strings.Contains(src, "rt.TZFull(xoZoneinfo)") {
			t.Errorf("xo_tzdata.go does not embed the database:\n%s", src)
		}
		if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "main.xo:9:11: note: time.zone with a name known only at run time embeds the whole zone database") {
			t.Errorf("notes %q", res.Notes)
		}
	})

	t.Run("system", func(t *testing.T) {
		dir := filepath.Join("testdata", "golden", "time_dynamic_zone")
		got, res := buildAndRunOpts(t, dir, build.Options{Mode: gen.Build, TZData: build.TZDataSystem})
		if src := tzdata(t, res); !strings.Contains(src, "rt.TZUseSystem()") || strings.Contains(src, "TZFull") {
			t.Errorf("xo_tzdata.go with --tzdata=system:\n%s", src)
		}
		if len(res.Notes) != 0 {
			t.Errorf("notes %q", res.Notes)
		}
		if !strings.Contains(got.stdout, "Asia/Tokyo: Wed, 01 Jul 2026 21:00:00 JST 9h0m0s") {
			t.Errorf("stdout %q", got.stdout)
		}
	})
}
