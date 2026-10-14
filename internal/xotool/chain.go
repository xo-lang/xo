package xotool

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The seed chain (decision 0216 item 2). When the sources start to use
// what an older xo cannot compile (an intrinsic its emitters do not
// lower, gotchas/seed-older-than-intrinsics.md), that xo cannot build
// them; it builds an intermediate xo first, from a recorded commit whose
// compiler has the change, and the intermediate builds the sources.
// scripts/seed-chain.txt records the hops in order, one per line:
//
//	<commit> [std=<commit>]  # why
//
// A hop builds the commit's selfhost/xo with the previous xo and the
// std of std=<commit> (default: the commit's own): a hop whose own std
// already uses the change takes the std of an older commit, so the
// older xo compiles the new compiler against a runtime it knows. Each
// hop is built once into <user cache>/xo/seed/hop-<commit>[-std-<commit>].
// scripts/bootstrap-xo.sh takes the same hops in XO_SEED_CHAIN.

// Hop is one recorded intermediate build of the seed chain.
type Hop struct{ Commit, Std string }

// ChainHops reads root's scripts/seed-chain.txt (none when it is
// missing).
func ChainHops(root string) ([]Hop, error) {
	f, err := os.Open(filepath.Join(root, "scripts", "seed-chain.txt"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Hop
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line, _, _ := strings.Cut(sc.Text(), "#")
		fs := strings.Fields(line)
		if len(fs) == 0 {
			continue
		}
		h := Hop{Commit: fs[0]}
		for _, x := range fs[1:] {
			v, ok := strings.CutPrefix(x, "std=")
			if !ok {
				return nil, fmt.Errorf("scripts/seed-chain.txt: %q: want <commit> [std=<commit>]", sc.Text())
			}
			h.Std = v
		}
		out = append(out, h)
	}
	return out, sc.Err()
}

// Chain builds every hop of root's seed chain in order, the first with
// seed, and returns the last hop's xo (seed when there are none).
func Chain(root, seed string, log io.Writer) (string, error) {
	hops, err := ChainHops(root)
	if err != nil {
		return "", err
	}
	base, err := cacheBase()
	if err != nil {
		return "", err
	}
	cur := seed
	for _, h := range hops {
		name := "hop-" + short(h.Commit)
		if h.Std != "" {
			name += "-std-" + short(h.Std)
		}
		dir := filepath.Join(base, "seed", name)
		bin := filepath.Join(dir, "xo")
		if isFile(bin) {
			cur = bin
			continue
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		tree := filepath.Join(dir, "tree")
		os.RemoveAll(tree)
		if err := archiveTree(root, h.Commit, tree); err != nil {
			return "", err
		}
		std := filepath.Join(tree, "std")
		if h.Std != "" {
			stree := filepath.Join(dir, "stdtree")
			os.RemoveAll(stree)
			if err := archiveTree(root, h.Std, stree); err != nil {
				return "", err
			}
			std = filepath.Join(stree, "std")
		}
		fmt.Fprintf(log, "xotool: seed chain hop %s (std of %s) built by %s\n", h.Commit, orSelf(h.Std, h.Commit), cur)
		t0 := time.Now()
		backend, env := hostBackend()
		tmp := bin + ".tmp"
		if out, err := runIn(tree, env, cur, "build", "--backend="+backend, "--std="+std, "-o", tmp, filepath.Join(tree, "selfhost", "xo")); err != nil {
			return "", fmt.Errorf("hop %s: %v\n%s", h.Commit, err, lastLines(out, 20))
		}
		if err := os.Rename(tmp, bin); err != nil {
			return "", err
		}
		fmt.Fprintf(log, "xotool: hop %s built in %.0f s\n", h.Commit, time.Since(t0).Seconds())
		cur = bin
	}
	return cur, nil
}

func short(c string) string {
	if len(c) > 12 {
		return c[:12]
	}
	return c
}

func orSelf(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

// archiveTree writes commit's tree (git archive) into dir.
func archiveTree(root, commit, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	arch := exec.Command("git", "-C", root, "archive", "--format=tar", commit)
	untar := exec.Command("tar", "-x", "-f", "-", "-C", dir)
	pipe, err := arch.StdoutPipe()
	if err != nil {
		return err
	}
	untar.Stdin = pipe
	var e1, e2 bytes.Buffer
	arch.Stderr, untar.Stderr = &e1, &e2
	if err := untar.Start(); err != nil {
		return err
	}
	if err := arch.Run(); err != nil {
		return fmt.Errorf("git archive %s: %v %s", commit, err, e1.String())
	}
	if err := untar.Wait(); err != nil {
		return fmt.Errorf("tar: %v %s", err, e2.String())
	}
	return nil
}
