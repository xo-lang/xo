package rt

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"io"
	"io/fs"
	"path"
	"strconv"
	"strings"
	"time"
)

// std/archive (std.md 18, decision 0164): gzip, tar, and zip in memory
// with Go's packages. Native builds report it as XO1201 (planned).

// M_display is `archive: msg`.
func (e Archive_ArchiveErr) M_display(*Task) string { return "archive: " + e.F_msg }

func archiveErr(msg string) Archive_ArchiveErr { return Archive_ArchiveErr{F_msg: msg} }

// Archive_file is `archive.file(path, data)`.
func Archive_file(_ *Task, p string, data Bytes) Archive_Entry {
	return Archive_Entry{F_path: p, F_data: data, F_mode: 0o644}
}

// Archive_dir is `archive.dir(path)`.
func Archive_dir(_ *Task, p string) Archive_Entry {
	return Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: 0o755, F_is_dir: true}
}

// Archive_gzip is `archive.gzip(data)`. The header carries no name and no
// time, so the output depends on the data only.
func Archive_gzip(_ *Task, data Bytes) Bytes {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(data))
	w.Close()
	return Bytes(b.String())
}

// Archive_gunzip is `archive.gunzip(data)`; concatenated members are read
// as one stream, as gzip(1) does.
func Archive_gunzip(_ *Task, data Bytes) Result[Bytes, Archive_ArchiveErr] {
	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	out, err := io.ReadAll(r)
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(out))
}

// archivePath checks an entry's path: relative, inside the archive's
// root, `/` separated; directories end without `/`.
func archivePath(p string) (string, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.HasPrefix(p, "/") || len(p) >= 2 && p[1] == ':' {
		return "", false
	}
	clean := path.Clean(p)
	if clean == ".." || strings.HasPrefix(clean, "../") || clean == "." {
		return "", false
	}
	return clean, true
}

func entryTime(e Archive_Entry) time.Time { return time.Unix(0, int64(e.F_modified)).UTC() }

// Archive_tar is `archive.tar(entries)`.
func Archive_tar(_ *Task, entries List[Archive_Entry]) Result[Bytes, Archive_ArchiveErr] {
	var b bytes.Buffer
	w := tar.NewWriter(&b)
	for _, e := range entries.s {
		p, ok := archivePath(e.F_path)
		if !ok {
			return Err[Bytes](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
		}
		h := &tar.Header{Name: p, Mode: e.F_mode & 0o7777, ModTime: entryTime(e), Typeflag: tar.TypeReg, Size: int64(len(e.F_data)), Format: tar.FormatPAX}
		if e.F_is_dir {
			h.Typeflag, h.Name, h.Size = tar.TypeDir, p+"/", 0
		}
		if err := w.WriteHeader(h); err != nil {
			return Err[Bytes](archiveErr("tar: " + err.Error()))
		}
		if !e.F_is_dir {
			if _, err := w.Write([]byte(e.F_data)); err != nil {
				return Err[Bytes](archiveErr("tar: " + err.Error()))
			}
		}
	}
	if err := w.Close(); err != nil {
		return Err[Bytes](archiveErr("tar: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b.String()))
}

// Archive_untar is `archive.untar(data)`.
func Archive_untar(_ *Task, data Bytes) Result[List[Archive_Entry], Archive_ArchiveErr] {
	r := tar.NewReader(strings.NewReader(string(data)))
	var out []Archive_Entry
	for {
		h, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Err[List[Archive_Entry]](archiveErr("untar: " + err.Error()))
		}
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(h.Name))
		if !ok {
			return Err[List[Archive_Entry]](archiveErr("untar: entry path " + strconv.Quote(h.Name) + " leaves the archive"))
		}
		e := Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: h.Mode & 0o7777, F_modified: Time(h.ModTime.UnixNano())}
		switch h.Typeflag {
		case tar.TypeDir:
			e.F_is_dir = true
		case tar.TypeXGlobalHeader:
			// describes the archive (`git archive` writes one holding
			// the commit), not an entry: skipped (std.md 18)
			continue
		case tar.TypeReg:
			b, err := io.ReadAll(r)
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("untar: " + err.Error()))
			}
			e.F_data = Bytes(b)
		default:
			return Err[List[Archive_Entry]](archiveErr("untar: entry " + strconv.Quote(h.Name) + " is not a file or a directory"))
		}
		out = append(out, e)
	}
	return Ok[List[Archive_Entry], Archive_ArchiveErr](ListOf(out...))
}

// Archive_zip is `archive.zip(entries)`.
func Archive_zip(_ *Task, entries List[Archive_Entry]) Result[Bytes, Archive_ArchiveErr] {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, e := range entries.s {
		p, ok := archivePath(e.F_path)
		if !ok {
			return Err[Bytes](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
		}
		h := &zip.FileHeader{Name: p, Method: zip.Deflate, Modified: entryTime(e)}
		h.SetMode(fs.FileMode(e.F_mode & 0o777))
		if e.F_is_dir {
			h.Name, h.Method = p+"/", zip.Store
			h.SetMode(fs.ModeDir | fs.FileMode(e.F_mode&0o777))
		}
		fw, err := w.CreateHeader(h)
		if err != nil {
			return Err[Bytes](archiveErr("zip: " + err.Error()))
		}
		if !e.F_is_dir {
			if _, err := fw.Write([]byte(e.F_data)); err != nil {
				return Err[Bytes](archiveErr("zip: " + err.Error()))
			}
		}
	}
	if err := w.Close(); err != nil {
		return Err[Bytes](archiveErr("zip: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b.String()))
}

// Archive_unzip is `archive.unzip(data)`.
func Archive_unzip(_ *Task, data Bytes) Result[List[Archive_Entry], Archive_ArchiveErr] {
	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
	}
	var out []Archive_Entry
	for _, f := range r.File {
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(f.Name))
		if !ok {
			return Err[List[Archive_Entry]](archiveErr("unzip: entry path " + strconv.Quote(f.Name) + " leaves the archive"))
		}
		m := f.Mode()
		e := Archive_Entry{F_path: p, F_data: Bytes(""), F_mode: int64(m.Perm()), F_modified: Time(f.Modified.UnixNano())}
		switch {
		case m.IsDir():
			e.F_is_dir = true
		case m.IsRegular():
			rc, err := f.Open()
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
			}
			b, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
			}
			e.F_data = Bytes(b)
		default:
			return Err[List[Archive_Entry]](archiveErr("unzip: entry " + strconv.Quote(f.Name) + " is not a file or a directory"))
		}
		out = append(out, e)
	}
	return Ok[List[Archive_Entry], Archive_ArchiveErr](ListOf(out...))
}
