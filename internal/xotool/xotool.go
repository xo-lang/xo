// Package xotool finds the xo the repository's Go tests and scripts
// drive: the self hosted compiler (compiler/xo) of this checkout.
//
// Since decision 0215 the Go implementation of the compiler is gone, so
// `xo` is never built by `go build`: an existing xo builds the checkout's
// compiler/xo (bootstrapping, toolchain.md "Bootstrapping without Go").
// Path returns, in order:
//
//   - $XO_CLI (its first field), when set: that binary, as it is, so a
//     run can pin one (a release's bin/xo, or one built before a change);
//     but when the binary records the key it was built for (bin/xo.key
//     beside it, or the cache's checkout/<key>/xo) and the checkout's
//     sources now have another key, Path fails (StaleCLI): a worktree's
//     bin/xo pinned before a rebase tested newer std and goldens against
//     an older compiler (Xo lang 5, 2026-10-21: 7 bogus failures of a
//     60 minute suite). XO_CLI_STALE_OK=1 allows a deliberate cross
//     version run;
//   - else <repo>/bin/xo, rebuilt first when it was not built from the
//     checkout's current sources (bin/xo.key holds the key it was built
//     for, Key).
//
// A build (Ensure) runs once per key across processes and worktrees: the
// binary goes into <user cache>/xo/checkout/<key>/xo (under a file lock;
// $XO_CHECKOUT_CACHE overrides the directory) and is copied to bin/xo,
// whose std is <repo>/std (xo1 looks for std at <bin>/../std). Keys past
// the newest four unused for a day are pruned (prune.go). The seed,
// the xo that compiles it, is the first of: $XO_SEED; <repo>/bin/xo; the
// newest earlier build in the cache whose recorded commit is an ancestor
// of the checkout's HEAD (ancestorBuild); `xo` on PATH; the newest prebuilt
// seed (decision 0216 item 1: seeds/latest.txt of $XO_SEED_URL or
// s3://$XO_SEED_BUCKET, fetched with the aws CLI into
// <user cache>/xo/seeds; seed.go). A seed that fails is reported and
// the next one tried; when none builds, Ensure fails (owner,
// 2026-10-13). Only with XO_SEED_FALLBACK=chain (or from-source: the
// root alone, `xobuild --from-source`) does the xo1 of the
// go-compiler-final tag (the last commit with the Go compiler, built
// once with `go build ./cmd/xo` from `git archive go-compiler-final`
// into <user cache>/xo/seed) build through the hops of
// scripts/seed-chain.txt. The backend is the host's: the Go backend on Windows (as
// scripts/windows-batch.sh builds xo1), arm64 on an arm64 host, else LLVM
// split into 12 modules without LTO (decision 0168), as
// scripts/bootstrap-xo.sh builds xo.
//
// A build takes 5 to 12 minutes and several GB of memory (the Mac,
// 2026-10-13); set XO_CLI to a built xo to skip it.
package xotool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/xo-lang/xo/internal/testtmp"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SeedCommit is the root of the seed chain (decision 0216): the commit
// tagged go-compiler-final (4fd10d38), whose Go compiler builds its xo1.
// It is an ancestor of every later commit, so a clone without tags (a
// git bundle) has it; SeedTag is tried when it is not there. A root
// older than an intrinsic the sources use reaches them through the
// recorded hops of scripts/seed-chain.txt (Chain;
// gotchas/seed-older-than-intrinsics.md).
const (
	SeedCommit = "87873aa5480676efa23c21ae08eb4107bb5d1d80"
	SeedTag    = "go-compiler-final"
)

// Root is the repository root: the nearest directory at or above the
// working directory whose go.mod declares module github.com/xo-lang/xo.
func Root() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		b, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err == nil && bytes.HasPrefix(b, []byte("module github.com/xo-lang/xo\n")) {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("xotool: not inside the xo repository (no go.mod of module github.com/xo-lang/xo above the working directory)")
		}
		dir = parent
	}
}

var (
	pathOnce sync.Once
	pathBin  string
	pathErr  error
)

// Path is the xo to run (see the package comment), built at most once
// per process.
func Path() (string, error) {
	pathOnce.Do(func() {
		if f := strings.Fields(os.Getenv("XO_CLI")); len(f) > 0 {
			pathBin = f[0]
			if _, err := os.Stat(pathBin); err != nil {
				pathErr = fmt.Errorf("XO_CLI=%s: %v", pathBin, err)
				return
			}
			root, _ := Root()
			announce(root, pathBin)
			if os.Getenv("XO_CLI_STALE_OK") != "1" && root != "" {
				pathErr = StaleCLI(root, pathBin)
				if pathErr != nil {
					// once, where go test shows it: the callers report a
					// failed start in their own ways, some not at all
					fmt.Fprintf(os.Stderr, "xotool: %v\n", pathErr)
				}
			}
			return
		}
		root, err := Root()
		if err != nil {
			pathErr = err
			return
		}
		pathBin, pathErr = Ensure(root, os.Stderr)
		if pathErr == nil {
			announce(root, pathBin)
		}
	})
	return pathBin, pathErr
}

// VerifyBuilt is an error when the xo at bin was stamped with another
// source key than key: Ensure never returns an xo built from other
// sources than the tree's (a bin/xo replaced by hand beside a current
// xo.key, or a cache entry under the wrong key; 2026-10-21). An xo that
// reports no stamp passes (one built before the stamp).
func VerifyBuilt(bin, key string) error {
	k, c, ok := stampedKey(bin)
	if !ok || k == key {
		return nil
	}
	return fmt.Errorf("%s was built from sources with key %s (commit %s), not this tree's %s: remove it and build again", bin, k, c, key)
}

// BuiltKey is the source key the xo at bin was built for, and its
// commit when recorded ("" for unknown): bin/xo.key and bin/xo.commit
// beside a checkout's bin/xo (Ensure), or the cache's checkout/<key>/xo
// and its commit file; ok is false when bin records no key (a release's
// or seed's xo).
func BuiltKey(bin string) (key, commit string, ok bool) {
	dir := filepath.Dir(bin)
	if b, err := os.ReadFile(filepath.Join(dir, "xo.key")); err == nil {
		c, _ := os.ReadFile(filepath.Join(dir, "xo.commit"))
		return strings.TrimSpace(string(b)), strings.TrimSpace(string(c)), true
	}
	if filepath.Base(filepath.Dir(dir)) == "checkout" && isKey(filepath.Base(dir)) {
		c, _ := os.ReadFile(filepath.Join(dir, "commit"))
		return filepath.Base(dir), strings.TrimSpace(string(c)), true
	}
	// any other xo: the key and commit it was stamped with (`xo version
	// --json`, decision 0264), so a copy or an `xo build -o` one is
	// checked too
	if k, c, ok := stampedKey(bin); ok {
		return k, c, true
	}
	return "", "", false
}

// stampedKey is the source key and commit xo version --json of bin
// reports (the build stamps them), ok false when it reports none (an xo
// built before the stamp, or one that does not run).
func stampedKey(bin string) (key, commit string, ok bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "version", "--json").Output()
	if err != nil {
		return "", "", false
	}
	var v struct {
		Commit    string `json:"commit"`
		Dirty     bool   `json:"dirty"`
		SourceKey string `json:"source_key"`
	}
	if json.Unmarshal(out, &v) != nil || !isKey(v.SourceKey) {
		return "", "", false
	}
	c := v.Commit
	if v.Dirty && c != "" {
		c += " with uncommitted changes"
	}
	return v.SourceKey, c, true
}

// announce prints, once per process, which xo the run uses: its path,
// the key and commit it was built from ("no key" when unknown), and the
// tree's key, so a stale xo is visible in a run's log even when nothing
// stops it (Xo lang 5's suites, 2026-10-21).
func announce(root, bin string) {
	k, c, ok := BuiltKey(bin)
	built := "no key"
	if ok {
		built = "key " + k
		if c != "" {
			built += ", commit " + shortCommit(c)
		}
	}
	tree := "unknown"
	if root != "" {
		if tk, err := Key(root); err == nil {
			tree = tk
			if ok && tk != k {
				tree += " (differs)"
			}
		}
	}
	fmt.Fprintf(os.Stderr, "xotool: xo %s (%s); tree key %s\n", bin, built, tree)
}

func shortCommit(c string) string {
	if len(c) >= 40 && isHex(c[:40]) {
		return c[:12] + c[40:]
	}
	return c
}

func isHex(s string) bool {
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

func isKey(s string) bool {
	if len(s) != 16 {
		return false
	}
	for _, r := range s {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
}

// StaleCLI is an error when the xo at bin was built for other sources
// than root's now (Key), nil when they match or bin records no key.
func StaleCLI(root, bin string) error {
	k, commit, ok := BuiltKey(bin)
	if !ok {
		return nil
	}
	now, err := Key(root)
	if err != nil || now == k {
		return nil
	}
	if commit == "" {
		commit = "not recorded"
	}
	head := headCommit(root)
	if head == "" {
		head = "unknown"
	}
	return fmt.Errorf("XO_CLI=%s is stale: it was built from sources with key %s (commit %s), but %s has key %s now (HEAD %s): compiler/ or the std it builds in changed since (a rebase of its worktree?), so newer std and goldens would run against an older compiler. Rebuild it (go run ./scripts/xobuild in that tree) or unset XO_CLI; XO_CLI_STALE_OK=1 runs it anyway, for a deliberate cross version run", bin, k, commit, root, now, head)
}

// Command is Path with the leading flags $XO_CLI carries (its fields
// after the first), for a command line.
// init makes every xo the tests run fail a build whose inlined arm64
// module does not verify (compiler/xo main.xo inline_or_say), where a
// normal build says so and goes on without inlining: a broken inliner
// must fail the tests, not pass them slowly
// (gotchas/arm64-inline-verify-fallback.md). XO_INLINE_STRICT=0 in the
// environment keeps the normal behavior.
func init() {
	if os.Getenv("XO_INLINE_STRICT") == "" {
		os.Setenv("XO_INLINE_STRICT", "1")
	}
}

func Command() ([]string, error) {
	bin, err := Path()
	if err != nil {
		return nil, err
	}
	if f := strings.Fields(os.Getenv("XO_CLI")); len(f) > 0 {
		return f, nil
	}
	return []string{bin}, nil
}

// Key is the key of the xo built from root's sources on this host: a
// hash of the host and Inputs.
func Key(root string) (string, error) {
	names, err := Inputs(root)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	fmt.Fprintf(h, "xo checkout v1\n%s/%s\n", runtime.GOOS, runtime.GOARCH)
	if o := LLVMOpt(); o != "" {
		// an LLVM built xo of another clang level is another build
		fmt.Fprintf(h, "llvm opt %s\n", o)
	}
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s %d\n", n, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// Inputs are the files the built xo depends on, relative to root with
// slashes, sorted: every .xo file and xo.mod under compiler/, and of std
// the Xo files of the prelude and of each module compiler/ reaches
// through `use` lines, followed through the std files themselves
// (std/NAME.xo and std/native/NAME.xo for `use std/NAME`;
// std/native/NAME.xo for `use native/NAME`; std/native/llrt/NAME.xo and
// NAME_c.xo for `use native/llrt/NAME`). The other std modules and the
// C runtime (std/llrt) are read by xo at run time from
// the checkout (<bin>/../std), so a change there needs no new xo.
// Test files (*_test.xo) are never inputs: xo does not compile them into
// itself (decision 0262), so adding or editing a test rebuilds nothing.
func Inputs(root string) ([]string, error) {
	in := map[string]bool{}
	var queue []string
	add := func(name string) {
		if in[name] || strings.HasSuffix(name, "_test.xo") {
			return
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(name))); err != nil {
			return
		}
		in[name] = true
		queue = append(queue, name)
	}
	err := filepath.WalkDir(filepath.Join(root, CompilerDir(root)), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
			return filepath.SkipDir
		}
		if !d.IsDir() && (strings.HasSuffix(p, ".xo") || d.Name() == "xo.mod") {
			rel, _ := filepath.Rel(root, p)
			add(filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	add("std/prelude.xo")
	add("std/native/prelude.xo")
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		if !strings.HasSuffix(n, ".xo") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(n)))
		if err != nil {
			return nil, err
		}
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 || f[0] != "use" {
				continue
			}
			p := strings.Trim(f[1], `"`)
			switch {
			case strings.HasPrefix(p, "std/"):
				m := strings.TrimPrefix(p, "std/")
				add("std/" + m + ".xo")
				add("std/native/" + m + ".xo")
			case strings.HasPrefix(p, "native/llrt/"):
				add("std/" + p + ".xo")
				add("std/" + p + "_c.xo")
			case strings.HasPrefix(p, "native/"):
				add("std/" + p + ".xo")
			}
		}
	}
	names := make([]string, 0, len(in))
	for n := range in {
		names = append(names, n)
	}
	sort.Strings(names)
	return names, nil
}

// cacheBase is $XO_CHECKOUT_CACHE, else <user cache>/xo.
func cacheBase() (string, error) {
	if d := os.Getenv("XO_CHECKOUT_CACHE"); d != "" {
		return d, nil
	}
	c, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(c, "xo"), nil
}

// Ensure makes root/bin/xo the xo of root's current sources and returns
// its path, building it when no build of the key exists (progress and
// failures of seeds go to log).
func Ensure(root string, log io.Writer) (string, error) {
	key, err := Key(root)
	if err != nil {
		return "", err
	}
	bin := filepath.Join(root, "bin", exeName("xo"))
	keyFile := filepath.Join(root, "bin", "xo.key")
	if k, err := os.ReadFile(keyFile); err == nil && strings.TrimSpace(string(k)) == key && isFile(bin) && VerifyBuilt(bin, key) == nil {
		if base, err := cacheBase(); err == nil {
			useCheckout(base, key)
		}
		return bin, nil
	}
	base, err := cacheBase()
	if err != nil {
		return "", err
	}
	defer useCheckout(base, key)
	built := filepath.Join(base, "checkout", key, exeName("xo"))
	if !isFile(built) {
		if err := buildLocked(root, built, log); err != nil {
			return "", err
		}
	}
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return "", err
	}
	if err := VerifyBuilt(built, key); err != nil {
		return "", err
	}
	if err := copyFile(built, bin); err != nil {
		return "", err
	}
	if err := writeAtomic(keyFile, []byte(key+"\n"), 0o644); err != nil {
		return "", err
	}
	// the commit it was built from, for StaleCLI's message ("" when the
	// sources had uncommitted changes)
	commitFile := filepath.Join(root, "bin", "xo.commit")
	if c, err := os.ReadFile(filepath.Join(filepath.Dir(built), "commit")); err == nil {
		writeAtomic(commitFile, c, 0o644)
	} else {
		os.Remove(commitFile)
	}
	// the clang level of an LLVM built xo, which scripts/seed-pack.sh
	// checks (seeds are -O2)
	optFile := filepath.Join(root, "bin", "xo.opt")
	if o := LLVMOpt(); o != "" {
		if err := writeAtomic(optFile, []byte(o+"\n"), 0o644); err != nil {
			return "", err
		}
	} else {
		os.Remove(optFile)
	}
	return bin, nil
}

// SeedFallback is $XO_SEED_FALLBACK: "" (the default: no build from
// source; a failed fetch of the prebuilt seed is an error), "chain"
// (after the other seeds, the go-compiler-final root through the seed
// chain), or "from-source" (only that: the replay from source, decision
// 0216 item 3, or a new platform; `xobuild --from-source`). Owner,
// 2026-10-13: the chain costs 25 to 37 minutes and hid fetch failures,
// so it runs only when asked for.
func SeedFallback() string { return strings.TrimSpace(os.Getenv("XO_SEED_FALLBACK")) }

// buildLocked builds root's compiler/xo into out once across processes.
func buildLocked(root, out string, log io.Writer) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	unlock, err := lockFile(filepath.Join(filepath.Dir(out), "build.lock"))
	if err != nil {
		return err
	}
	defer unlock()
	if isFile(out) {
		return nil
	}
	seeds, err := Seeds(root)
	if err != nil {
		return err
	}
	chain := SeedFallback() != ""
	var errs []string
	for _, s := range seeds {
		if s.Path == "" {
			p, err := s.Resolve(log)
			if err != nil {
				fmt.Fprintf(log, "xotool: no seed from %s: %v\n", s.Why, err)
				errs = append(errs, s.Why+": "+err.Error())
				continue
			}
			s.Path = p
		}
		// a check first (seconds) so an error in the sources fails each seed
		// at once instead of after a build of minutes (Xo Lang 2: a
		// receiver error cost a 5 minute build, 2026-10-20); a seed whose
		// check fails is too old for these sources or the sources do not
		// check: the next seed is tried, or the seed chain when it is on
		if PrecheckOn() && (s.NoChain || !chain) {
			if cerr := CheckXo(root, s.Path); cerr != nil {
				fmt.Fprintf(log, "xotool: compiler/xo does not check with %s (%s); trying the next seed\n", s.Why, summary(cerr))
				errs = append(errs, s.Why+": its check of compiler/xo: "+cerr.Error())
				continue
			}
		}
		// a broken builder (a miscompiled xo recorded as a clean build) is
		// skipped in seconds
		if serr := SmokeXo(root, s.Path); serr != nil {
			fmt.Fprintf(log, "xotool: %s cannot build a small program (%s); trying the next seed\n", s.Why, summary(serr))
			errs = append(errs, s.Why+": its smoke build: "+serr.Error())
			continue
		}
		fmt.Fprintf(log, "xotool: building xo (compiler/xo of %s) with %s (%s), a few minutes, into %s\n", root, s.Why, s.Path, out)
		t0 := time.Now()
		err := BuildXo(root, s.Path, out)
		if err != nil {
			// one line here; the whole output is in the error when no
			// seed builds xo (a cached build from another branch can fail
			// with pages of linker errors before the next seed works)
			fmt.Fprintf(log, "xotool: %s failed (%s); trying the next seed\n", s.Why, summary(err))
			errs = append(errs, s.Why+": "+err.Error())
			if s.NoChain || !chain {
				continue
			}
			// the recorded intermediate hops (decision 0216 item 2),
			// then the checkout
			fmt.Fprintf(log, "xotool: walking scripts/seed-chain.txt from %s (XO_SEED_FALLBACK=%s)\n", s.Why, SeedFallback())
			hop, herr := Chain(root, s.Path, log)
			if herr != nil {
				errs = append(errs, s.Why+" through the seed chain: "+herr.Error())
				continue
			}
			if hop == s.Path {
				continue
			}
			if err = BuildXo(root, hop, out); err != nil {
				errs = append(errs, s.Why+" after the seed chain: "+err.Error())
				continue
			}
		}
		// a builder far behind HEAD can miscompile newer compiler sources
		// (an evaluation order or semantics change the compiler's own code
		// uses): the xo it built then builds them again, as the bootstrap's
		// second stage does (Windows CI 39772751654: a cached build 214
		// commits back made an xo that refused all of std/native with
		// XO1201, gotchas/xobuild-ancestor-older-semantics.md)
		if err := SecondHop(root, s.Path, s.Why, out, log); err != nil {
			fmt.Fprintf(log, "xotool: the second hop from %s failed (%s); trying the next seed\n", s.Why, summary(err))
			errs = append(errs, s.Why+" (second hop): "+err.Error())
			continue
		}
		fmt.Fprintf(log, "xotool: built in %.0f s with %s\n", time.Since(t0).Seconds(), s.Why)
		recordCommit(root, filepath.Dir(out))
		return nil
	}
	msg := "xotool: no seed built xo"
	if !chain {
		msg += fmt.Sprintf(" and building from source is opt in: the prebuilt seed comes from %s/seeds/ (latest.txt, then seeds/<seed>/%s.tar.gz; needs the aws CLI and read access, toolchain.md \"Prebuilt seeds\"). Fix the cause below, or set XO_SEED to an xo, or XO_CLI to skip the build; only to rebuild from the %s root through scripts/seed-chain.txt (25 to 37 minutes, needs Go and git) set XO_SEED_FALLBACK=chain or run `go run ./scripts/xobuild --from-source`", SeedURL(), HostPlatform(), SeedTag)
	} else {
		msg += " (set XO_SEED to an xo, or XO_CLI to skip the build)"
	}
	return fmt.Errorf("%s:\n  %s", msg, strings.Join(errs, "\n  "))
}

// Seed is an xo that can compile compiler/xo, and where it came from.
// An empty Path is resolved when the seed is reached (Resolve): the
// prebuilt seed (FetchSeed) and the go-compiler-final root through the
// seed chain (SeedFromTag, Chain). NoChain skips the seed chain when
// the seed fails to build the checkout.
type Seed struct {
	Path, Why string
	Resolve   func(io.Writer) (string, error)
	NoChain   bool
}

// Seeds are the candidate seeds of root, in order (the package comment):
// $XO_SEED, the checkout's bin/xo, the newest earlier build, xo on PATH,
// the prebuilt seed, and, only with XO_SEED_FALLBACK=chain, the
// go-compiler-final root through the seed chain. XO_SEED_FALLBACK=
// from-source gives the root alone.
func Seeds(root string) ([]Seed, error) {
	rootSeed := Seed{Why: "the " + SeedTag + " root through scripts/seed-chain.txt", Resolve: func(log io.Writer) (string, error) {
		fmt.Fprintf(log, "xotool: building the %s root and walking scripts/seed-chain.txt (XO_SEED_FALLBACK=%s; needs Go and git)\n", SeedTag, SeedFallback())
		p, err := SeedFromTag(root, log)
		if err != nil {
			return "", err
		}
		// the root goes through the recorded hops first: the chain is
		// recorded because the root cannot build these sources directly
		// (an EC2 run spent minutes failing the direct build, 2026-10-13)
		hop, err := Chain(root, p, log)
		if err != nil {
			return "", fmt.Errorf("the seed chain: %v", err)
		}
		return hop, nil
	}, NoChain: true}
	switch SeedFallback() {
	case "", "chain":
	case "from-source":
		return []Seed{rootSeed}, nil
	default:
		return nil, fmt.Errorf("xotool: XO_SEED_FALLBACK=%s: want chain or from-source (or unset)", SeedFallback())
	}
	var out []Seed
	seen := map[string]bool{}
	add := func(p, why string) {
		if p == "" || seen[p] || !isFile(p) {
			return
		}
		seen[p] = true
		out = append(out, Seed{Path: p, Why: why})
	}
	add(os.Getenv("XO_SEED"), "$XO_SEED")
	// not a bin/xo built from a commit that is not an ancestor of HEAD (a
	// newer one, after checking out an older commit to bisect): its
	// output calls runtime exports the older std lacks
	// (gotchas/xobuild-seed-newer-than-commit.md)
	if c, err := os.ReadFile(filepath.Join(root, "bin", "xo.commit")); err != nil || !knownCommit(root, strings.TrimSpace(string(c))) || isAncestor(root, strings.TrimSpace(string(c))) {
		add(filepath.Join(root, "bin", exeName("xo")), "the checkout's earlier bin/xo")
	}
	if base, err := cacheBase(); err == nil {
		ms, _ := filepath.Glob(filepath.Join(base, "checkout", "*", exeName("xo")))
		sort.Slice(ms, func(i, j int) bool { return modTime(ms[i]).After(modTime(ms[j])) })
		if p := ancestorBuild(root, ms); p != "" {
			add(p, "the newest earlier build "+p)
		}
	}
	if p, err := exec.LookPath("xo"); err == nil {
		add(p, "xo on PATH ("+p+")")
	}
	if loc := SeedURL(); loc != "off" {
		// one hop from the newest seed (decision 0216 item 1)
		out = append(out, Seed{Why: "the prebuilt seed of " + loc, Resolve: func(log io.Writer) (string, error) { return FetchSeedFor(root, log) }, NoChain: true})
	}
	if SeedFallback() == "chain" {
		out = append(out, rootSeed)
	}
	return out, nil
}

// recordCommit writes, beside a build in dir, the commit its sources came
// from (the commit file) and that they were that commit's (the clean
// file), for ancestorBuild. A build from a tree with uncommitted changes
// under compiler/ or std/ is not that commit's: its xo can call runtime
// exports the commit's std lacks (`dyld: Symbol not found:
// _xo_x_task_spawn_fn`, 2026-10-16, gotchas/xobuild-dirty-build-as-ancestor.md),
// so it gets neither file (an older one is removed).
func recordCommit(root, dir string) {
	commitFile := filepath.Join(dir, "commit")
	cleanFile := filepath.Join(dir, "clean")
	os.Remove(cleanFile)
	c := headCommit(root)
	if c == "" || !sourcesClean(root) {
		os.Remove(commitFile)
		return
	}
	os.WriteFile(commitFile, []byte(c+"\n"), 0o644)
	os.WriteFile(cleanFile, []byte("compiler/ and std/ had no uncommitted changes\n"), 0o644)
}

// sourcesClean reports whether root's compiler/ and std/ match HEAD: no
// modified, staged, or untracked files there.
func sourcesClean(root string) bool {
	out, err := exec.Command("git", "-C", root, "status", "--porcelain", "--untracked-files=normal", "--", CompilerDir(root), "std").Output()
	return err == nil && len(strings.TrimSpace(string(out))) == 0
}

// ancestorBuild is the cached build of ms (newest first) whose recorded
// commit (the commit file recordCommit writes beside it, with the clean
// file) is the nearest ancestor of root's HEAD (the fewest commits
// between), or "". A build of another branch can emit calls into a
// runtime this checkout's std does not have (on a shared CI runner,
// 2026-10-14, run 38632539643), and so can a build of a dirty tree
// (2026-10-16); a build with no record, no clean file (a dirty tree's, or
// one recorded before 2026-10-16), or outside a git checkout cannot be
// placed, so none of those is used. The nearest, not the most recently
// written: a worktree detached at an old commit rebuilds an old xo
// whose language can predate what the sources need (decision 0326's
// evaluation order: an xo 70 commits old built a d5c2c17f7 xo that
// faulted in gen, 2026-10-22, gotchas/xobuild-ancestor-older-semantics.md).
func ancestorBuild(root string, ms []string) string {
	head := headCommit(root)
	if head == "" {
		return ""
	}
	best, bestN := "", -1
	for i, m := range ms {
		if i == 40 {
			break
		}
		if _, err := os.Stat(filepath.Join(filepath.Dir(m), "clean")); err != nil {
			continue
		}
		b, err := os.ReadFile(filepath.Join(filepath.Dir(m), "commit"))
		c := strings.TrimSpace(string(b))
		if err != nil || c == "" {
			continue
		}
		if exec.Command("git", "-C", root, "merge-base", "--is-ancestor", c, head).Run() != nil {
			continue
		}
		out, err := exec.Command("git", "-C", root, "rev-list", "--count", c+".."+head).Output()
		if err != nil {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(string(out)))
		if err != nil {
			continue
		}
		if bestN < 0 || n < bestN {
			best, bestN = m, n
		}
	}
	return best
}

// SmokeXo has xo build a small program with root's std and the host
// architecture's native backend, the runtime of std/native included: an error when it cannot.
// Windows CI 39772751654: an xo that ran (`xo version`) refused all of
// std/native with XO1201, and as a recorded build of a near commit it
// would have been the next build's seed.
func SmokeXo(root, xo string) error {
	dir, err := os.MkdirTemp("", "xo-smoke-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err := os.WriteFile(filepath.Join(dir, "xo.mod"), []byte("module smoke\n"), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "main.xo"), []byte("fn main(os: Os) {\n  os.stdio.println(\"smoke\")\n}\n"), 0o644); err != nil {
		return err
	}
	// the native backend of the host's architecture even where xo itself
	// is built with the Go backend (Windows): the native SSA builder is
	// what a miscompiled xo broke there
	backend := "arm64"
	if runtime.GOARCH == "amd64" {
		backend = "amd64"
	}
	_, env := hostBackend()
	o, err := runIn(dir, env, xo, "build", "--backend="+backend, "--std="+xoPath(filepath.Join(root, "std")), "-o", xoPath(filepath.Join(dir, exeName("smoke"))), xoPath(dir))
	if err != nil {
		return fmt.Errorf("%v\n%s", err, firstErrors(o, 10))
	}
	return nil
}

// SecondHop builds root's compiler/xo again into out with the xo at out,
// which seed (described by why) built, when seed is more than HopMax
// commits behind HEAD or how far is unknown (commitsBehind); else it does
// nothing.
func SecondHop(root, seed, why, out string, log io.Writer) error {
	n := commitsBehind(root, seed)
	if n >= 0 && n <= HopMax {
		return nil
	}
	behind := "an unknown number of commits"
	if n >= 0 {
		behind = fmt.Sprintf("%d commits", n)
	}
	fmt.Fprintf(log, "xotool: %s is %s behind HEAD (more than %d): building again with the xo it built\n", why, behind, HopMax)
	x1 := filepath.Join(filepath.Dir(out), exeName("xo-hop1"))
	os.Remove(x1)
	if err := os.Rename(out, x1); err != nil {
		return err
	}
	defer os.Remove(x1)
	return BuildXo(root, x1, out)
}

// HopMax is how many commits behind HEAD a builder of compiler/xo may be
// before Ensure builds again with the xo it built (a second hop).
const HopMax = 50

// commitsBehind is how many commits the xo seed's recorded commit (`xo
// version --json`, decision 0264) is behind root's HEAD, or -1 when that
// is unknown: no record (an older xo), a commit this checkout does not
// have or that is not an ancestor of HEAD, or no git.
func commitsBehind(root, seed string) int {
	head := headCommit(root)
	if head == "" {
		return -1
	}
	o, err := runIn(root, nil, seed, "version", "--json")
	if err != nil {
		return -1
	}
	var v struct {
		Commit string `json:"commit"`
	}
	if json.Unmarshal([]byte(strings.TrimSpace(o)), &v) != nil || v.Commit == "" {
		return -1
	}
	if exec.Command("git", "-C", root, "merge-base", "--is-ancestor", v.Commit, head).Run() != nil {
		return -1
	}
	out, err := exec.Command("git", "-C", root, "rev-list", "--count", v.Commit+".."+head).Output()
	if err != nil {
		return -1
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return -1
	}
	return n
}

// headCommit is root's HEAD commit, or "" outside a git checkout.
func headCommit(root string) string {
	out, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "-q", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// summary is the first line of err and how many lines follow it.
func summary(err error) string {
	ls := strings.Split(strings.TrimRight(err.Error(), "\n"), "\n")
	if len(ls) == 1 {
		return ls[0]
	}
	return fmt.Sprintf("%s; %d more lines", ls[0], len(ls)-1)
}

// SeedFromTag is the xo1 of the last commit with the Go compiler
// (SeedCommit, else the go-compiler-final tag), built once into
// <user cache>/xo/seed/<commit>/tree/bin/xo: `git archive` of the
// commit, `go build ./cmd/xo` (the Go compiler at its last commit), and
// that xo builds the commit's compiler/xo with its own driver (so the
// seed is a self hosted xo that takes --std like every later one).
func SeedFromTag(root string, log io.Writer) (string, error) {
	rev, err := exec.Command("git", "-C", root, "rev-parse", "--verify", "-q", SeedCommit+"^{commit}").Output()
	if err != nil {
		rev, err = exec.Command("git", "-C", root, "rev-parse", "--verify", "-q", SeedTag+"^{commit}").Output()
	}
	if err != nil {
		return "", fmt.Errorf("neither the tag %s nor commit %s is in the repository (git fetch origin tag %s)", SeedTag, SeedCommit, SeedTag)
	}
	base, err := cacheBase()
	if err != nil {
		return "", err
	}
	commit := strings.TrimSpace(string(rev))
	dir := filepath.Join(base, "seed", commit[:12])
	tree := filepath.Join(dir, "tree")
	bin := filepath.Join(tree, "bin", exeName("xo"))
	if isFile(bin) {
		return bin, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockFile(filepath.Join(dir, "build.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	if isFile(bin) {
		return bin, nil
	}
	if _, err := exec.LookPath("go"); err != nil {
		return "", fmt.Errorf("building the %s seed needs Go: %v", SeedTag, err)
	}
	os.RemoveAll(tree)
	if err := os.MkdirAll(tree, 0o755); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "xotool: building the %s seed in %s\n", SeedTag, tree)
	if err := archiveTree(root, commit, tree); err != nil {
		return "", fmt.Errorf("%s: %v", SeedTag, err)
	}
	goXo := filepath.Join(dir, exeName("goxo"))
	if out, err := runIn(tree, nil, "go", "build", "-o", goXo, "./cmd/xo"); err != nil {
		return "", fmt.Errorf("go build ./cmd/xo at %s: %v\n%s", SeedTag, err, out)
	}
	backend, env := hostBackend()
	env = append(env, "XO_SELF_DRIVER=0")
	if err := os.MkdirAll(filepath.Join(tree, "bin"), 0o755); err != nil {
		return "", err
	}
	tmp := bin + ".tmp"
	if out, err := runIn(tree, env, goXo, "build", "--backend="+backend, "-o", tmp, "./"+CompilerDir(tree)+"/xo"); err != nil {
		return "", fmt.Errorf("the %s Go xo building its selfhost/xo: %v\n%s", SeedTag, err, out)
	}
	return bin, os.Rename(tmp, bin)
}

// exeName is name with the host's executable suffix: Windows only runs
// a program by path when it ends in .exe (gotchas/xotool-tar-pipe-windows.md
// covers the fresh Windows build this broke along with the tar pipe).
func exeName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// LLVMOpt is the clang level xo is built at on a host whose xo is built
// by the LLVM backend: $XO_LLVM_OPT, else O0 (decision 0220: development
// builds; seeds and releases are -O2); "" on other hosts.
func LLVMOpt() string {
	if b, _ := hostBackend(); b != "llvm" {
		return ""
	}
	if o := os.Getenv("XO_LLVM_OPT"); o != "" {
		return o
	}
	return "O0"
}

// hostBackend is hostBackendNative's backend and environment, with
// XO_NOLIBC=0 on Linux: since decision 0243 a Linux program without `use
// c` links no C library by default, but the toolchain's own native Linux
// builds of compiler/xo keep the C library until a Linux self rebuild
// fixed point has passed with none (0243, pending; the seeds' Linux xo is
// cross built with the Go backend, which no-libc does not touch).
func hostBackend() (string, []string) {
	backend, env := hostBackendNative()
	if runtime.GOOS == "linux" && os.Getenv("XO_NOLIBC") == "" {
		env = append(env, "XO_NOLIBC=0")
	}
	return backend, env
}

// hostBackendNative is the backend that builds xo on this host and the
// environment it needs.
func hostBackendNative() (string, []string) {
	if runtime.GOOS == "windows" {
		// the Go backend (compiler/gen and the go command), as
		// scripts/windows-batch.sh built xo1 there
		return "go", nil
	}
	if runtime.GOARCH == "arm64" {
		return "arm64", nil
	}
	var env []string
	if os.Getenv("XO_LLVM_SPLIT") == "" {
		env = append(env, "XO_LLVM_SPLIT=12")
	}
	if os.Getenv("XO_LLVM_LTO") == "" {
		env = append(env, "XO_LLVM_LTO=none")
	}
	// a development build of xo: clang -O0 (decision 0220; c7i.8xlarge:
	// 3:45 against 18:51 at -O2, its compiles about 1.5 times slower);
	// releases keep -O2 (scripts/release.sh, xo build --release)
	if os.Getenv("XO_LLVM_OPT") == "" {
		env = append(env, "XO_LLVM_OPT=O0")
	}
	return "llvm", env
}

// CompilerDir is the directory of the compiler's sources in the tree at
// root: compiler/ since decision 0249, selfhost/ in older commits (the
// seed chain's hops and the go-compiler-final tag build those).
func CompilerDir(root string) string {
	if _, err := os.Stat(filepath.Join(root, "selfhost", "xo")); err == nil {
		if _, err := os.Stat(filepath.Join(root, "compiler", "xo")); err != nil {
			return "selfhost"
		}
	}
	return "compiler"
}

// BuildXo has the xo seed build root's compiler/xo into out with the
// host's backend and root's std.
func BuildXo(root, seed, out string) error {
	backend, env := hostBackend()
	tmp := fmt.Sprintf("%s.%d.tmp", out, os.Getpid())
	defer os.Remove(tmp)
	args := []string{"build", "--backend=" + backend, "--std=" + xoPath(filepath.Join(root, "std")), "-o", xoPath(tmp)}
	args = append(args, StampArgs(root, seed)...)
	args = append(args, xoPath(filepath.Join(root, CompilerDir(root), "xo")))
	o, err := runIn(root, env, seed, args...)
	if err != nil {
		return fmt.Errorf("%v\n%s", err, lastLines(o, 20))
	}
	if !isFile(tmp) {
		return fmt.Errorf("no binary written\n%s", lastLines(o, 20))
	}
	// an xo that cannot start is no build: a seed newer than root's std
	// links calls to runtime exports it lacks, which only the dynamic
	// loader reports (`dyld: Symbol not found: _xo_x_preempt`, 2026-10-22)
	if vo, err := runIn(root, nil, tmp, "version"); err != nil {
		return fmt.Errorf("the built xo does not run (%v)\n%s", err, lastLines(vo, 5))
	}
	// an xo that runs can still be miscompiled: one that cannot build a
	// small program is no build, and is not cached (SmokeXo)
	if err := SmokeXo(root, tmp); err != nil {
		return fmt.Errorf("the built xo cannot build a program: %v", err)
	}
	return os.Rename(tmp, out)
}

// CheckXo runs `xo check` of root's compiler/xo with seed and root's std:
// the check errors, if any (seconds, where BuildXo takes minutes).
func CheckXo(root, seed string) error {
	o, err := runIn(root, []string{"XO_STD=" + xoPath(filepath.Join(root, "std"))}, seed, "check", xoPath(filepath.Join(root, CompilerDir(root), "xo")))
	if err != nil {
		return fmt.Errorf("%v\n%s", err, firstErrors(o, 20))
	}
	return nil
}

// PrecheckOn is false when XO_BUILD_PRECHECK=0 turns CheckXo off before
// each build.
func PrecheckOn() bool { return os.Getenv("XO_BUILD_PRECHECK") != "0" }

// firstErrors is the error lines of xo check's output and the line after
// each (its note or fix), at most n lines, then its summary line.
func firstErrors(s string, n int) string {
	var out []string
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range ls {
		if strings.Contains(l, ": error ") && len(out) < n {
			out = append(out, l)
			if i+1 < len(ls) && strings.HasPrefix(ls[i+1], "  ") {
				out = append(out, ls[i+1])
			}
		}
	}
	if len(ls) > 0 {
		out = append(out, ls[len(ls)-1])
	}
	return strings.Join(out, "\n")
}

// xoPath is p as xo takes paths: with / separators on Windows (std.md
// 3.1). xo's checker splits paths only at /, so a C:\ path from
// filepath.Join never reaches the module root's xo.mod and every import
// between the module's packages fails with XO1306
// (gotchas/windows-seed-xo1306-module-not-required.md).
func xoPath(p string) string { return filepath.ToSlash(p) }

func runIn(dir string, env []string, name string, args ...string) (string, error) {
	// behind the process's lifeline: a killed or timed out test leaves no
	// xo build, and no program it runs, behind (testtmp.Guard)
	cmd := testtmp.Guard(exec.Command(name, args...))
	cmd.Dir = dir
	cmd.Env = TestJobsEnv(append(append(os.Environ(), "PWD="+xoPath(dir)), env...))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func lastLines(s string, n int) string {
	ls := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(ls) > n {
		ls = ls[len(ls)-n:]
	}
	return strings.Join(ls, "\n")
}

func isFile(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.Mode().IsRegular()
}

func modTime(p string) time.Time {
	fi, err := os.Stat(p)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// copyFile copies src to dst through a temporary file renamed into
// place, so a concurrent reader sees the old or the new binary.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := fmt.Sprintf("%s.%d.tmp", dst, os.Getpid())
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, dst)
}

func writeAtomic(p string, data []byte, mode os.FileMode) error {
	tmp := fmt.Sprintf("%s.%d.tmp", p, os.Getpid())
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// TestJobs is the XO_TEST_JOBS the Go tests give the xo they run: `xo
// test` runs its test processes at once on every physical core by
// default (decision 0235), which multiplies with the other tests and
// heavy runs on the machine (gotchas: limit-parallel-test-load).
const TestJobs = "2"

// TestJobsEnv is env with XO_TEST_JOBS=TestJobs added unless env
// already sets it.
func TestJobsEnv(env []string) []string {
	for _, kv := range env {
		if strings.HasPrefix(kv, "XO_TEST_JOBS=") {
			return env
		}
	}
	return append(env, "XO_TEST_JOBS="+TestJobs)
}

// StampArgs are the `--stamp` flags of a build of root's compiler/xo
// (decision 0264, compiler/xo provenance.xo): the commit, a hash of the
// uncommitted changes to tracked files ("" for none), the source key,
// and the seed that builds it. None when root's compiler has no
// provenance.xo (an older tree, a seed chain hop) or the key cannot be
// computed; the commit and diff are "" outside git.
func StampArgs(root, seed string) []string {
	if !isFile(filepath.Join(root, CompilerDir(root), "xo", "provenance.xo")) {
		return nil
	}
	key, err := Key(root)
	if err != nil {
		return nil
	}
	commit, dirty := gitState(root)
	args := []string{
		"--stamp", "XO_COMMIT=" + commit,
		"--stamp", "XO_DIRTY=" + dirty,
		"--stamp", "XO_SOURCE_KEY=" + key,
		"--stamp", "XO_BUILT_BY=" + shortPath(seed),
	}
	// the code generation key (decision 0275), when this tree's xo has
	// the constant; none (xo keys its caches on its own binary) when the
	// closure cannot be computed
	if src, err := os.ReadFile(filepath.Join(root, CompilerDir(root), "xo", "provenance.xo")); err == nil && bytes.Contains(src, []byte("const XO_CODEGEN_KEY: Str")) {
		if ck, err := CodegenKey(root); err == nil {
			args = append(args, "--stamp", "XO_CODEGEN_KEY="+ck)
		}
	}
	return args
}

// gitState is root's HEAD commit and, when tracked files differ from it,
// the first 12 hex digits of the SHA-256 of `git diff HEAD`.
func gitState(root string) (string, string) {
	out, err := exec.Command("git", "-C", root, "rev-parse", "HEAD").Output()
	if err != nil {
		return "", ""
	}
	commit := strings.TrimSpace(string(out))
	diff, err := exec.Command("git", "-C", root, "diff", "HEAD").Output()
	if err != nil || len(diff) == 0 {
		return commit, ""
	}
	sum := sha256.Sum256(diff)
	return commit, hex.EncodeToString(sum[:])[:12]
}

// shortPath is p's last three elements (a seed's
// seeds/0.20.0-b84c5c56/darwin-arm64/..., a cache entry's key/xo).
func shortPath(p string) string {
	parts := strings.Split(filepath.ToSlash(p), "/")
	if len(parts) > 3 {
		parts = parts[len(parts)-3:]
	}
	return strings.Join(parts, "/")
}
