package xotool

import (
	"archive/tar"
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
		bin := filepath.Join(dir, exeName("xo"))
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
		if out, err := runIn(tree, env, cur, "build", "--backend="+backend, "--std="+xoPath(std), "-o", xoPath(tmp), xoPath(filepath.Join(tree, "selfhost", "xo"))); err != nil {
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

// archiveTree writes commit's tree (git archive) into dir. The tar stream
// is unpacked here (archive/tar), not by an external tar: on Windows the
// Git Bash tar misreads a C:\ path for -C and exits, and with the pipe's
// read end still held by this process git then blocks writing forever
// (gotchas/xotool-tar-pipe-windows.md).
func archiveTree(root, commit, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	arch := exec.Command("git", "-C", root, "archive", "--format=tar", commit)
	var e1 bytes.Buffer
	arch.Stderr = &e1
	out, err := arch.StdoutPipe()
	if err != nil {
		return err
	}
	if err := arch.Start(); err != nil {
		return err
	}
	uerr := untarTo(out, dir)
	// drain what is left so git can exit, then wait for it
	io.Copy(io.Discard, out)
	werr := arch.Wait()
	if werr != nil {
		return fmt.Errorf("git archive %s: %v %s", commit, werr, e1.String())
	}
	if uerr != nil {
		return fmt.Errorf("unpacking git archive %s: %v", commit, uerr)
	}
	return nil
}

// untarTo unpacks a tar stream into dir: directories, regular files (with
// their executable bits), and symbolic links (copied as files where the OS
// refuses links). Paths that leave dir are rejected.
func untarTo(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(h.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("unsafe path %q", h.Name)
		}
		path := filepath.Join(dir, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777|0o600)
			if err != nil {
				return err
			}
			_, cerr := io.Copy(f, tr)
			if err := f.Close(); cerr == nil {
				cerr = err
			}
			if cerr != nil {
				return cerr
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			if err := os.Symlink(h.Linkname, path); err != nil {
				// no symlink permission (Windows): keep a copy if the
				// target is a file already unpacked, else skip it
				target := filepath.Join(filepath.Dir(path), filepath.FromSlash(h.Linkname))
				if b, rerr := os.ReadFile(target); rerr == nil {
					if err := os.WriteFile(path, b, 0o644); err != nil {
						return err
					}
				}
			}
		case tar.TypeXGlobalHeader:
			// git archive's commit id comment
		}
	}
}
