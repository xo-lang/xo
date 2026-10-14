package xotool

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Prebuilt seeds (decision 0216 item 1, accepted 2026-10-13): release N
// is built by release N-1. A seed is a release shaped tree (bin/xo,
// lib/xo/std, lib/xo/third_party, lib/xo/SEED) packed by
// scripts/seed-pack.sh and kept in a private S3 bucket:
//
//	seeds/latest.txt                         the newest version, one line
//	seeds/<version>/<goos>-<goarch>.tar.gz   the tree
//	seeds/<version>/<goos>-<goarch>.sha256   "<hex>  <goos>-<goarch>.tar.gz"
//	seeds/<version>/manifest.txt             version, commit, date, sha256s
//
// FetchSeed reads latest.txt, downloads the host's tarball with the aws
// CLI, checks its sha256, and unpacks it into
// <user cache>/xo/seeds/<version>/<goos>-<goarch>/tree.

// DefaultSeedBucket is the bucket seeds are read from when
// $XO_SEED_BUCKET is unset.
const DefaultSeedBucket = "xo-seeds-ACCOUNT_ID"

// defaultSeedRegion is the bucket's region ($XO_SEED_REGION overrides).
const defaultSeedRegion = "us-east-1"

// SeedURL is the location seeds are fetched from: $XO_SEED_URL (an
// s3://bucket[/prefix] URL, a file:// URL, or a local directory; "off"
// turns fetching off), else s3://$XO_SEED_BUCKET (default
// DefaultSeedBucket; it may carry a /prefix). The location holds
// seeds/latest.txt and seeds/<version>/.
func SeedURL() string {
	if u := strings.TrimSpace(os.Getenv("XO_SEED_URL")); u != "" {
		return strings.TrimRight(u, "/")
	}
	b := strings.Trim(strings.TrimSpace(os.Getenv("XO_SEED_BUCKET")), "/")
	if b == "" {
		b = DefaultSeedBucket
	}
	return "s3://" + strings.TrimPrefix(b, "s3://")
}

// HostPlatform is the seed platform of this host, <goos>-<goarch>.
func HostPlatform() string { return runtime.GOOS + "-" + runtime.GOARCH }

var seedVersionRE = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([.+-][0-9A-Za-z.-]+)?$`)

// seedStore reads objects under a seed location.
type seedStore struct{ loc string }

// local is the directory of a local location ("" for s3).
func (s seedStore) local() string {
	if strings.HasPrefix(s.loc, "s3://") {
		return ""
	}
	return strings.TrimPrefix(s.loc, "file://")
}

// aws runs the aws CLI with a timeout and returns its standard output.
func (s seedStore) aws(timeout time.Duration, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("aws"); err != nil {
		return nil, fmt.Errorf("no aws CLI on PATH (%v)", err)
	}
	region := os.Getenv("XO_SEED_REGION")
	if region == "" {
		region = defaultSeedRegion
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "aws", append(args, "--region", region, "--only-show-errors")...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("aws %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.Bytes(), nil
}

// read returns the object rel (a small one).
func (s seedStore) read(rel string) ([]byte, error) {
	if d := s.local(); d != "" {
		return os.ReadFile(filepath.Join(d, filepath.FromSlash(rel)))
	}
	return s.aws(2*time.Minute, "s3", "cp", s.loc+"/"+rel, "-")
}

// fetch copies the object rel to the file dst.
func (s seedStore) fetch(rel, dst string) error {
	if d := s.local(); d != "" {
		return copyFile(filepath.Join(d, filepath.FromSlash(rel)), dst)
	}
	_, err := s.aws(20*time.Minute, "s3", "cp", s.loc+"/"+rel, dst)
	return err
}

// LatestSeedVersion reads seeds/latest.txt of the seed location.
func LatestSeedVersion() (string, error) {
	loc := SeedURL()
	if loc == "off" {
		return "", errors.New("seed fetching is off (XO_SEED_URL=off)")
	}
	b, err := seedStore{loc}.read("seeds/latest.txt")
	if err != nil {
		return "", fmt.Errorf("reading %s/seeds/latest.txt: %v", loc, err)
	}
	v := strings.TrimSpace(string(b))
	if !seedVersionRE.MatchString(v) {
		return "", fmt.Errorf("%s/seeds/latest.txt: %q is not a version", loc, v)
	}
	return v, nil
}

// FetchSeed returns the xo of the newest prebuilt seed for this host,
// downloading and unpacking it the first time (progress to log). When
// latest.txt cannot be read, the newest seed already unpacked in the
// cache is used, if any.
func FetchSeed(log io.Writer) (string, error) {
	base, err := cacheBase()
	if err != nil {
		return "", err
	}
	plat := HostPlatform()
	v, err := LatestSeedVersion()
	if err != nil {
		if c := newestCachedSeed(base, plat); c != "" {
			fmt.Fprintf(log, "xotool: %v; using the seed already unpacked at %s\n", err, c)
			return c, nil
		}
		return "", err
	}
	return fetchSeedVersion(base, v, plat, log)
}

func seedExe() string {
	if runtime.GOOS == "windows" {
		return "xo.exe"
	}
	return "xo"
}

// newestCachedSeed is the bin/xo of the highest version unpacked in
// base/seeds for plat ("" when none).
func newestCachedSeed(base, plat string) string {
	ms, _ := filepath.Glob(filepath.Join(base, "seeds", "*", plat, "tree", "bin", seedExe()))
	best, bestV := "", ""
	for _, m := range ms {
		v := filepath.Base(filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(m)))))
		if isFile(m) && (best == "" || versionLess(bestV, v)) {
			best, bestV = m, v
		}
	}
	return best
}

// versionLess orders versions by their dot separated fields, numbers
// numerically and anything else as strings.
func versionLess(a, b string) bool {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) && i < len(pb); i++ {
		na, ea := strconv.Atoi(pa[i])
		nb, eb := strconv.Atoi(pb[i])
		switch {
		case ea == nil && eb == nil && na != nb:
			return na < nb
		case (ea != nil || eb != nil) && pa[i] != pb[i]:
			return pa[i] < pb[i]
		}
	}
	return len(pa) < len(pb)
}

// fetchSeedVersion makes base/seeds/<v>/<plat>/tree the seed of version
// v once (under a file lock) and returns its bin/xo.
func fetchSeedVersion(base, v, plat string, log io.Writer) (string, error) {
	dir := filepath.Join(base, "seeds", v, plat)
	tree := filepath.Join(dir, "tree")
	bin := filepath.Join(tree, "bin", seedExe())
	if isFile(bin) {
		return bin, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	unlock, err := lockFile(filepath.Join(dir, "fetch.lock"))
	if err != nil {
		return "", err
	}
	defer unlock()
	if isFile(bin) {
		return bin, nil
	}
	st := seedStore{SeedURL()}
	rel := "seeds/" + v + "/" + plat
	sum, err := st.read(rel + ".sha256")
	if err != nil {
		return "", fmt.Errorf("no seed %s for %s at %s (%s.sha256: %v)", v, plat, st.loc, rel, err)
	}
	want := strings.ToLower(firstField(string(sum)))
	if len(want) != 64 {
		return "", fmt.Errorf("%s/%s.sha256: no sha256 in %q", st.loc, rel, strings.TrimSpace(string(sum)))
	}
	fmt.Fprintf(log, "xotool: fetching seed %s for %s from %s\n", v, plat, st.loc)
	t0 := time.Now()
	tgz := filepath.Join(dir, plat+".tar.gz.tmp")
	defer os.Remove(tgz)
	if err := st.fetch(rel+".tar.gz", tgz); err != nil {
		return "", fmt.Errorf("downloading %s/%s.tar.gz: %v", st.loc, rel, err)
	}
	got, err := fileSHA256(tgz)
	if err != nil {
		return "", err
	}
	if got != want {
		return "", fmt.Errorf("%s/%s.tar.gz: sha256 %s, want %s (the .sha256 beside it)", st.loc, rel, got, want)
	}
	tmp := tree + ".tmp"
	os.RemoveAll(tmp)
	if err := untarGz(tgz, tmp); err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("unpacking %s.tar.gz: %v", rel, err)
	}
	if !isFile(filepath.Join(tmp, "bin", seedExe())) {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("%s.tar.gz has no bin/%s", rel, seedExe())
	}
	os.RemoveAll(tree)
	if err := os.Rename(tmp, tree); err != nil {
		return "", err
	}
	fmt.Fprintf(log, "xotool: seed %s unpacked into %s in %.1f s\n", v, tree, time.Since(t0).Seconds())
	return bin, nil
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

func fileSHA256(p string) (string, error) {
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// untarGz unpacks the gzip compressed tar file src into the new
// directory dst: regular files and directories only, every name inside
// dst.
func untarGz(src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	zr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.FromSlash(strings.TrimPrefix(h.Name, "./"))
		if name == "" || name == "." {
			continue
		}
		if filepath.IsAbs(name) || !filepath.IsLocal(name) {
			return fmt.Errorf("%q: a name outside the tree", h.Name)
		}
		if strings.HasPrefix(filepath.Base(name), "._") {
			continue // macOS metadata
		}
		p := filepath.Join(dst, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(0o644)
			if h.Mode&0o111 != 0 {
				mode = 0o755
			}
			out, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			if err := out.Close(); err != nil {
				return err
			}
		case tar.TypeXGlobalHeader:
		default:
			return fmt.Errorf("%q: type %q is not a file or a directory", h.Name, h.Typeflag)
		}
	}
}
