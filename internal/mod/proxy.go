package mod

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/sumdb"
)

// The module proxy protocol (decision 0126), read only, over HTTP(S), as
// Go's GOPROXY protocol where it fits. Below a proxy's base URL, with the
// module path and version escaped (an upper case letter is '!' and its
// lower case):
//
//	GET <path>/@v/list             the versions, one per line
//	GET <path>/@v/<version>.mod    the version's xo.mod
//	GET <path>/@v/<version>.zip    the version's tree, every file under "<path>@<version>/"
//	GET <path>/@v/<version>.lock   its xo.lock lines (tree and xo.mod hashes)
//
// 404 or 410 means the proxy does not have the module or version (the
// client tries the next XO_PROXY source after a comma). A proxy is not
// trusted: the tree it serves is hashed and checked against xo.lock (and
// the checksum log with XO_SUMDB) like a tree fetched from git; its .lock
// answer only catches a proxy that is inconsistent with itself.

// maxZip bounds a module zip and the files extracted from it (as Go's
// 500 MB module limit).
const maxZip = 500 << 20

var proxyHTTP = &http.Client{Timeout: 5 * time.Minute}

// proxyGet fetches base/p; notFound is set for 404 and 410.
func proxyGet(base, p string, limit int64) ([]byte, bool, error) {
	resp, err := proxyHTTP.Get(base + "/" + p)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		first, _, _ := strings.Cut(strings.TrimSpace(string(msg)), "\n")
		nf := resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone
		if first != "" {
			return nil, nf, fmt.Errorf("%s/%s: %s: %s", base, p, resp.Status, first)
		}
		return nil, nf, fmt.Errorf("%s/%s: %s", base, p, resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(body)) > limit {
		return nil, false, fmt.Errorf("%s/%s: larger than %d bytes", base, p, limit)
	}
	return body, false, nil
}

func proxyBase(modPath string) string { return sumdb.EscapePath(modPath) + "/@v/" }

// proxyList lists the versions a proxy has of a module.
func proxyList(base, modPath string) ([]string, bool, error) {
	body, nf, err := proxyGet(base, proxyBase(modPath)+"list", 1<<20)
	if err != nil {
		return nil, nf, err
	}
	return strings.Fields(string(body)), false, nil
}

// proxyFetch extracts a module version's zip from a proxy into dst and
// checks it against the proxy's own .lock answer when it has one.
func proxyFetch(base, modPath, version, dst string) (bool, error) {
	name := proxyBase(modPath) + sumdb.EscapePath(version)
	data, nf, err := proxyGet(base, name+".zip", maxZip)
	if err != nil {
		return nf, err
	}
	if err := unzipModule(data, modPath, version, dst); err != nil {
		return false, fmt.Errorf("%s/%s.zip: %v", base, name, err)
	}
	lock, _, err := proxyGet(base, name+".lock", 1<<16)
	if err != nil {
		return false, nil // a proxy without .lock answers
	}
	got, err := HashDir(dst)
	if err != nil {
		return false, err
	}
	for _, l := range strings.Split(string(lock), "\n") {
		f := strings.Fields(l)
		if len(f) == 3 && f[0] == modPath && f[1] == version && f[2] != got {
			return false, fmt.Errorf("%s/%s.zip hashes to %s but the proxy's .lock says %s", base, name, got, f[2])
		}
	}
	return false, nil
}

// unzipModule extracts the regular files of a module zip, all under
// "<path>@<version>/", into dir; anything else fails.
func unzipModule(data []byte, modPath, version, dir string) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	prefix := modPath + "@" + version + "/"
	seen := map[string]bool{}
	var total uint64
	for _, f := range zr.File {
		rel, ok := strings.CutPrefix(f.Name, prefix)
		if !ok {
			return fmt.Errorf("zip entry %q is not under %s", f.Name, prefix)
		}
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue // a directory
		}
		if !f.Mode().IsRegular() {
			return fmt.Errorf("zip entry %q is not a regular file", f.Name)
		}
		name := filepath.FromSlash(rel)
		if !filepath.IsLocal(name) || strings.Contains(rel, "\\") {
			return fmt.Errorf("zip entry %q leaves the module", f.Name)
		}
		if key := strings.ToLower(rel); seen[key] {
			return fmt.Errorf("zip entry %q appears twice (ignoring case)", f.Name)
		} else {
			seen[key] = true
		}
		total += f.UncompressedSize64
		if total > maxZip {
			return fmt.Errorf("the module is larger than %d bytes", maxZip)
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		body, err := io.ReadAll(io.LimitReader(rc, maxZip+1))
		rc.Close()
		if err != nil {
			return err
		}
		target := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(target, body, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// ZipModule writes the tree of a module version in dir as a proxy zip:
// its regular files, without nested modules, sorted, under
// "<path>@<version>/", with fixed times, so the same tree always gives
// the same bytes.
func ZipModule(w io.Writer, dir, modPath, version string) error {
	var files []string
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && p != dir {
			if _, err := os.Stat(filepath.Join(p, "xo.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		if d.Type().IsRegular() {
			rel, err := filepath.Rel(dir, p)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	})
	if err != nil {
		return err
	}
	sort.Strings(files)
	zw := zip.NewWriter(w)
	stamp := time.Date(1980, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, f := range files {
		h := &zip.FileHeader{Name: modPath + "@" + version + "/" + f, Method: zip.Deflate, Modified: stamp}
		h.SetMode(0o644)
		fw, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(f)))
		if err != nil {
			return err
		}
		if _, err := fw.Write(data); err != nil {
			return err
		}
	}
	return zw.Close()
}

// ProxyHandler serves the module cache at root (the XO_MODCACHE layout)
// over the proxy protocol, read only. With fetch, a module version or
// list the cache lacks is fetched from git into root first (never through
// XO_PROXY, so proxies do not loop): a caching proxy for a team. Without
// it only what root holds is served.
func ProxyHandler(root string, fetch bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "the module proxy is read only", http.StatusMethodNotAllowed)
			return
		}
		esc, file, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/@v/")
		modPath, err := sumdb.UnescapePath(esc)
		if !ok || err != nil || !ValidPath(modPath) || strings.Contains(file, "/") {
			http.Error(w, "want /<module path>/@v/list or /<module path>/@v/<version>.zip, .mod, or .lock", http.StatusNotFound)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		if file == "list" {
			vs := cacheVersions(root, modPath)
			if fetch {
				more, err := gitVersions(root, modPath)
				if err != nil && len(vs) == 0 {
					http.Error(w, err.Error(), http.StatusNotFound)
					return
				}
				vs = append(vs, more...)
			}
			vs = sortVersions(modPath, vs)
			if len(vs) == 0 {
				http.Error(w, "no versions of "+modPath, http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, strings.Join(vs, "\n")+"\n")
			return
		}
		ext := filepath.Ext(file)
		version, err := sumdb.UnescapePath(strings.TrimSuffix(file, ext))
		if err != nil || !ValidVersion(version) || (ext != ".zip" && ext != ".mod" && ext != ".lock") {
			http.Error(w, "unknown file "+file, http.StatusNotFound)
			return
		}
		dir := versionDirIn(root, modPath, version)
		if _, err := os.Stat(filepath.Join(dir, "xo.mod")); err != nil {
			if !fetch {
				http.Error(w, modPath+" "+version+" is not in this proxy", http.StatusNotFound)
				return
			}
			if _, p := download(root, modPath, version, directOnly); p != nil {
				http.Error(w, p.Msg, http.StatusNotFound)
				return
			}
		}
		switch ext {
		case ".mod":
			data, err := os.ReadFile(filepath.Join(dir, "xo.mod"))
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write(data)
		case ".lock":
			lines, err := Record(dir, modPath, version)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			io.WriteString(w, strings.Join(lines, "\n")+"\n")
		case ".zip":
			var buf bytes.Buffer
			if err := ZipModule(&buf, dir, modPath, version); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/zip")
			w.Write(buf.Bytes())
		}
	})
}
