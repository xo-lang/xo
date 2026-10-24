// Command stdsync regenerates every file that a std declaration change
// derives, so one command keeps a change whole:
//
//	go run ./scripts/stdsync          # write every generated file
//	go run ./scripts/stdsync -check   # exit 1 and name the stale ones
//
// The steps are spec/std.md's prelude index (internal/stdindex), the habit
// goldens' stamp and their generated files when std/prelude.xo's
// declarations changed (cmd/xo TestHabitGolden -update-habits), and
// std/rt's generated declarations and signature checks (internal/stdgen).
// The tests that watch one artifact (internal/repotest TestStdIndexCurrent,
// stdgen -check) and their failure messages name this command.
//
// It runs the xo the tests run (internal/xotool: $XO_CLI, else the
// checkout's bin/xo, built when stale).
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"

	"github.com/xo-lang/xo/internal/stdgen"
	"github.com/xo-lang/xo/internal/stdsync"
	"github.com/xo-lang/xo/internal/xotool"
)

func main() {
	check := flag.Bool("check", false, "exit 1 when a generated file is out of date, and write nothing")
	flag.Parse()
	if err := run(*check); err != nil {
		fmt.Fprintln(os.Stderr, "stdsync:", err)
		os.Exit(1)
	}
}

func run(check bool) error {
	root, err := xotool.Root()
	if err != nil {
		return err
	}
	xo, err := xotool.Command()
	if err != nil {
		return err
	}
	o := stdsync.Options{
		Root: root,
		StdGen: func(root string, check bool) ([]string, error) {
			return stdgen.Run(root, xo, check)
		},
		Habits: func(root string) error {
			// the goldens are the shipped xo's diagnostics and fixes
			cmd := exec.Command("go", "test", "-count=1", "./cmd/xo", "-run", "TestHabitGolden", "-update-habits")
			cmd.Dir = root
			out, err := cmd.CombinedOutput()
			if err != nil {
				return fmt.Errorf("go test ./cmd/xo -run TestHabitGolden -update-habits: %v\n%s", err, out)
			}
			return nil
		},
	}
	stale, err := stdsync.Run(o, check)
	if err != nil {
		return err
	}
	if check {
		if len(stale) > 0 {
			fmt.Fprint(os.Stderr, stdsync.Report(stale))
			os.Exit(1)
		}
		fmt.Println("stdsync: every generated file is current")
		return nil
	}
	if len(stale) > 0 {
		fmt.Fprint(os.Stderr, stdsync.Report(stale))
		os.Exit(1)
	}
	fmt.Println("stdsync: every generated file is written")
	return nil
}
