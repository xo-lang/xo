// Package stdsync regenerates every file that the std declarations derive,
// so one command keeps a std/prelude.xo change whole:
//
//   - std/rt's generated declarations and signature checks
//     (internal/stdgen: std_gen_M.go and std_sigs_M.go),
//   - spec/std.md's Appendix A prelude index (internal/stdindex),
//   - the habit goldens, whose want.json text and fixed.xo name prelude
//     methods (cmd/xo's TestHabitGolden -update-habits), when the prelude's
//     declarations changed since they were last made: the stamp
//     cmd/xo/testdata/habits/prelude.sha256 records that surface.
//
// Check mode reports every artifact that is out of date and writes
// nothing; it names the one command that fixes them all.
// scripts/stdsync runs it, and the failure messages of the tests that
// watch one artifact (internal/repotest TestStdIndexCurrent, stdgen
// -check) name that command too, so a stale generated file tells the
// reader what to type.
package stdsync

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xo-lang/xo/internal/stdindex"
)

// Command is the one command that regenerates everything this package
// checks, as the failure messages print it.
const Command = "go run ./scripts/stdsync"

// HabitsStamp is the file that records the prelude declarations the habit
// goldens were generated from. A test that walks cmd/xo/testdata/habits
// skips a file there (TestHabitGolden takes only directories).
const HabitsStamp = "cmd/xo/testdata/habits/prelude.sha256"

// PreludePath and SpecPath are the input and output of the index step.
const (
	PreludePath = "std/prelude.xo"
	SpecPath    = "spec/std.md"
)

// Stale names one artifact that is out of date. Artifact is the file or
// directory, Detail says what is wrong with it.
type Stale struct {
	Artifact string
	Detail   string
}

func (s Stale) String() string { return s.Artifact + " " + s.Detail }

// Options are the steps of a sync. A nil step is skipped, which the tests
// use to run the index and stamp without an xo build.
type Options struct {
	// Root is the repository root.
	Root string
	// StdGen regenerates std/rt's generated files (stdgen.Run); its
	// result is the list of stale or written paths.
	StdGen func(root string, check bool) ([]string, error)
	// Habits regenerates the habit goldens (cmd/xo TestHabitGolden
	// -update-habits). It runs only when the stamp is not current.
	Habits func(root string) error
}

// Run regenerates every step of o, or with check reports every artifact
// that is out of date and writes nothing. The steps run in a fixed order
// (the index, then the habit stamp, then stdgen) so output is stable.
func Run(o Options, check bool) (stale []Stale, err error) {
	indexStale, err := syncIndex(o.Root, check)
	if err != nil {
		return nil, err
	}
	if indexStale != nil {
		stale = append(stale, *indexStale)
	}
	stampStale, err := syncHabits(o, check)
	if err != nil {
		return nil, err
	}
	if stampStale != nil {
		stale = append(stale, *stampStale)
	}
	if o.StdGen != nil {
		paths, err := o.StdGen(o.Root, check)
		if err != nil {
			return nil, err
		}
		for _, p := range paths {
			stale = append(stale, Stale{p, "is stale"})
		}
	}
	return stale, nil
}

// syncIndex makes spec/std.md's prelude index std/prelude.xo's.
func syncIndex(root string, check bool) (*Stale, error) {
	prelude, err := os.ReadFile(filepath.Join(root, PreludePath))
	if err != nil {
		return nil, err
	}
	specPath := filepath.Join(root, SpecPath)
	spec, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	want := stdindex.Index(string(prelude))
	if stdindex.Current(string(spec)) == want {
		return nil, nil
	}
	if check {
		return &Stale{SpecPath, "is stale"}, nil
	}
	if err := os.WriteFile(specPath, []byte(stdindex.Splice(string(spec), want)), 0o644); err != nil {
		return nil, err
	}
	return nil, nil
}

// syncHabits regenerates the habit goldens when the prelude's declarations
// changed since the stamp was written, then rewrites the stamp.
func syncHabits(o Options, check bool) (*Stale, error) {
	prelude, err := os.ReadFile(filepath.Join(o.Root, PreludePath))
	if err != nil {
		return nil, err
	}
	// the index text is the prelude's declaration surface: every function
	// and method it declares, by type, in source order
	sum := sha256.Sum256([]byte(stdindex.Index(string(prelude))))
	want := hex.EncodeToString(sum[:]) + "\n"
	stampPath := filepath.Join(o.Root, HabitsStamp)
	have, readErr := os.ReadFile(stampPath)
	if readErr == nil && string(have) == want {
		return nil, nil
	}
	if readErr != nil && !os.IsNotExist(readErr) {
		return nil, readErr
	}
	if check {
		if os.IsNotExist(readErr) {
			return &Stale{HabitsStamp, "is missing: the habit goldens' prelude is not recorded"}, nil
		}
		return &Stale{HabitsStamp, "follows an older prelude: the habit goldens may be stale"}, nil
	}
	if o.Habits != nil {
		if err := o.Habits(o.Root); err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Dir(stampPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(stampPath, []byte(want), 0o644); err != nil {
		return nil, err
	}
	return nil, nil
}

// Report is the text Run's stale list prints, one line per artifact and
// then the command that fixes them all; "" when nothing is stale.
func Report(stale []Stale) string {
	if len(stale) == 0 {
		return ""
	}
	out := ""
	for _, s := range stale {
		out += fmt.Sprintf("%s %s\n", s.Artifact, s.Detail)
	}
	return out + "run " + Command + " to regenerate them\n"
}
