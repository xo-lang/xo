package llvm

import (
	"archive/zip"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/xotool"
)

// tzDiffProg prints the local zone (TZ, from env.txt) at a few instants,
// then reads lines from standard input: `z <name>` loads the zone named
// (the name is not a constant, so the build embeds the whole
// zoneinfo.zip), `t <unix seconds>` prints the zone's offset and
// abbreviation at that instant, the instant formatted in the zone with
// its abbreviation, and the instant of that civil time back again with
// each Ambiguity (a gap or an overlap shows as an error or another
// instant).
const tzDiffProg = `use std/time

fn back(c: time.Civil, z: time.Zone, a: time.Ambiguity) -> Str {
  match time.instant(c.date, c.time, z, a) {
    Ok(t) => "${t.unix()}"
    Err(e) => "${e}"
  }
}

fn main(os: Os) {
  let input = match os.stdio.read_all() {
    Ok(s) => s
    Err(_) => ""
  }
  var z = time.utc()
  var out: List[Str] = []
  match time.local(os.env) {
    Ok(lz) => {
      for s in [-2000000000, 0, 1000000000, 1700000000, 4000000000] {
        let t = Time.UNIX_EPOCH + Duration.from_secs_int(s)
        out.push("local ${lz} ${lz.offset_at(t)} ${lz.abbreviation_at(t)}")
      }
    }
    Err(e) => out.push("${e}")
  }
  for line in input.lines() {
    if line.len() < 2 {
      continue
    }
    let arg = line[2..line.len()]
    if line[0..1] == "z" {
      match time.zone(arg) {
        Ok(nz) => {
          let same = match time.zone(arg) {
            Ok(again) => again == nz && nz != time.utc()
            Err(_) => false
          }
          z = nz
          out.push("zone ${z} ${same}")
        }
        Err(e) => out.push("${e}")
      }
      continue
    }
    let s = Int.parse(arg) ?? 0
    let t = Time.UNIX_EPOCH + Duration.from_secs_int(s)
    let c = time.civil(t, z)
    let r = back(c, z, time.Ambiguity.Reject)
    let e = back(c, z, time.Ambiguity.Earlier)
    let l = back(c, z, time.Ambiguity.Later)
    out.push("${z.offset_at(t)} ${z.abbreviation_at(t)} ${time.format(t, z, time.RFC1123)} ${c.offset} ${c.zone} | ${r} | ${e} | ${l}")
  }
  os.stdio.println(out.join("\n"))
}
`

// tzCorpus is the input of tzDiffProg: every zone of Go's zoneinfo.zip
// (and a few names that are not zones), each with the instants around its
// transitions from 1700 to 2250 (one second before, at, and after; at
// most 40 transitions a zone, the first and last ones always), instants
// past the last transition (the POSIX TZ footer, every year to 2250 for
// some), before the first, near now (Go's cache), and random ones.
func tzCorpus(t *testing.T) string {
	zr, err := zip.OpenReader(tzZip(t))
	if err != nil {
		t.Skip("no zoneinfo.zip: ", err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	rng := rand.New(rand.NewPCG(5, 6))
	lo := time.Date(1700, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	hi := time.Date(2250, 1, 1, 0, 0, 0, 0, time.UTC).Unix()
	now := time.Now().Unix()
	var b strings.Builder
	for _, bad := range []string{"Mars/Olympus", "", "Local", "../etc/passwd", "/etc/localtime", "Europe/Berlin\\x"} {
		b.WriteString("z " + bad + "\n")
		b.WriteString("t 0\n")
	}
	for k, name := range names {
		b.WriteString("z " + name + "\n")
		loc, err := time.LoadLocation(name)
		if err != nil {
			continue
		}
		// the transitions, from the zone bounds
		var tx []int64
		for s := lo; s < hi && len(tx) < 2000; {
			_, end := time.Unix(s, 0).In(loc).ZoneBounds()
			if end.IsZero() {
				break
			}
			tx = append(tx, end.Unix())
			s = end.Unix()
		}
		pick := map[int]bool{}
		for i := range tx {
			if i < 3 || i >= len(tx)-3 {
				pick[i] = true
			}
		}
		for range 34 {
			if len(tx) > 0 {
				pick[rng.IntN(len(tx))] = true
			}
		}
		var at []int64
		for i := range tx {
			if pick[i] {
				at = append(at, tx[i]-1, tx[i], tx[i]+1)
			}
		}
		at = append(at, lo, lo+86400*400, now, now+3600*24*45, now-3600*24*200)
		if k%10 == 0 {
			// every year to 2250 past the last transition: the footer
			for y := 2030; y < 2250; y += 3 {
				at = append(at, time.Date(y, time.Month(1+y%12), 1+y%28, y%24, 0, 0, 0, time.UTC).Unix())
			}
		}
		for range 6 {
			at = append(at, lo+rng.Int64N(hi-lo))
		}
		for _, s := range at {
			b.WriteString("t " + strconv.FormatInt(s, 10) + "\n")
		}
	}
	return b.String()
}

// tzZip is the path of Go's zoneinfo.zip.
func tzZip(t *testing.T) string {
	goroot, err := exec.Command("go", "env", "GOROOT").Output()
	if err != nil {
		t.Skip("no go command for the zone data")
	}
	return filepath.Join(strings.TrimSpace(string(goroot)), "lib", "time", "zoneinfo.zip")
}

// TestTZDiff runs tzDiffProg over tzCorpus on the Go backend and on both
// native backends, with the zone rules in Xo (std/native/llrt/tz.xo) and
// with their C path (XO_LLRT_C=tz), and requires the Go backend's lines
// (decision 0193 step 5): offsets, abbreviations, civil times and their
// instants in every zone across its DST transitions and historical rules.
// The zone data is the embedded zoneinfo.zip, then the host's
// (--tzdata=system), then with --tzdata=system and $ZONEINFO naming Go's
// zoneinfo.zip (time.LoadLocation reads a zip there); the local zone is
// TZ's.
func TestTZDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds five programs")
	}
	in := tzCorpus(t)
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte(tzDiffProg), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "stdin.txt"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"embedded", "system", "zoneinfo"} {
		t.Run(mode, func(t *testing.T) {
			env, tzdata := "TZ=:America/Los_Angeles\n", mode
			if mode == "zoneinfo" {
				env += "ZONEINFO=" + tzZip(t) + "\n"
				tzdata = "system"
			}
			if err := os.WriteFile(filepath.Join(dir, "env.txt"), []byte(env), 0o644); err != nil {
				t.Fatal(err)
			}
			tzDiff(t, dir, in, tzdata)
		})
	}
}

// tzDiff is TestTZDiff with the zone data tzdata.
func tzDiff(t *testing.T, dir, in, tzdata string) {
	ref := buildAndRun(t, dir, xotool.Options{Backend: xotool.BackendGo, TZData: tzdata})
	if ref.exit != 0 {
		t.Fatalf("go backend: exit %d: %s", ref.exit, ref.stderr)
	}
	want := strings.Split(strings.TrimSuffix(ref.stdout, "\n"), "\n")
	inLines := strings.Split(in, "\n")
	unknown := strings.Count(ref.stdout, "unknown time zone")
	t.Logf("%d input lines, %d output lines, %d unknown zones; a sample:\n%s", len(inLines), len(want), unknown, strings.Join(want[len(want)/2:len(want)/2+8], "\n"))
	for _, path := range []string{"xo", "c"} {
		for _, be := range []string{xotool.BackendArm64, xotool.BackendLLVM} {
			t.Run(path+"/"+be, func(t *testing.T) {
				if path == "c" {
					t.Setenv("XO_LLRT_C", "tz")
				}
				r := buildAndRun(t, dir, xotool.Options{Backend: be, TZData: tzdata})
				if r.exit != 0 {
					t.Fatalf("exit %d: %s", r.exit, r.stderr)
				}
				got := strings.Split(strings.TrimSuffix(r.stdout, "\n"), "\n")
				bad := 0
				for i := 0; i < len(want) || i < len(got); i++ {
					var g, w string
					if i < len(got) {
						g = got[i]
					}
					if i < len(want) {
						w = want[i]
					}
					if g != w {
						if bad < 20 {
							t.Errorf("line %d: got %q, want %q", i+1, g, w)
						}
						bad++
					}
				}
				if bad > 0 {
					t.Errorf("%d of %d lines differ", bad, len(want))
				}
			})
		}
	}
}
