package rt

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
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
func Archive_gunzip(_ *Task, data Bytes, max int64) Result[Bytes, Archive_ArchiveErr] {
	if max < 0 {
		max = 0
	}
	r, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	out, err := io.ReadAll(io.LimitReader(r, max+1))
	if err != nil {
		return Err[Bytes](archiveErr("gunzip: " + err.Error()))
	}
	if int64(len(out)) > max {
		return Err[Bytes](archiveErr("gunzip: gzip: output exceeds max_size (" + strconv.FormatInt(max, 10) + " bytes)"))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(out))
}

// archivePath checks an entry's path: relative, inside the archive's
// root, `/` separated; directories end without `/`.
func archivePath(p string) (string, bool) {
	p = strings.ReplaceAll(p, "\\", "/")
	if p == "" || strings.Contains(p, "\x00") || strings.HasPrefix(p, "/") || len(p) >= 2 && p[1] == ':' {
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
			return Err[List[Archive_Entry]](archiveErr("untar: entry path " + strconv.Quote(h.Name) + archivePathWhy(h.Name)))
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

// tarFileReader reads a File for Go's tar.Reader with the current task.
type tarFileReader struct {
	f   File
	tk  *Task
	err string
}

func (z *tarFileReader) Read(p []byte) (int, error) {
	r := z.f.M_read(z.tk, int64(len(p)))
	if !r.Ok {
		z.err = r.E.M_display(z.tk)
		return 0, io.ErrClosedPipe
	}
	if !r.V.Some {
		return 0, io.EOF
	}
	return copy(p, r.V.V), nil
}

// Archive_TarReader is `archive.TarReader` (move only): Go's tar.Reader
// over the File, giving untar's entries.
type Archive_TarReader struct {
	tr   *tar.Reader
	fr   *tarFileReader
	done bool
}

func (Archive_TarReader) XoDebug(b *strings.Builder) { b.WriteString("<tar_reader>") }
func (x Archive_TarReader) M_debug(*Task) string     { return Debug(x) }

func Archive_tar_reader(_ *Task, f File) Archive_TarReader {
	fr := &tarFileReader{f: f}
	return Archive_TarReader{tr: tar.NewReader(fr), fr: fr}
}

func (x *Archive_TarReader) fail(err error) Archive_ArchiveErr {
	x.done = true
	if x.fr.err != "" {
		return archiveErr("tar_reader: " + x.fr.err)
	}
	return archiveErr("tar_reader: " + err.Error())
}

func (x *Archive_TarReader) M_next(tk *Task) Result[Option[Archive_TarFile], Archive_ArchiveErr] {
	x.fr.tk = tk
	for !x.done {
		h, err := x.tr.Next()
		if err == io.EOF {
			x.done = true
			break
		}
		if err != nil {
			return Err[Option[Archive_TarFile]](x.fail(err))
		}
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(h.Name))
		if !ok {
			x.done = true
			return Err[Option[Archive_TarFile]](archiveErr("tar_reader: entry path " + strconv.Quote(h.Name) + archivePathWhy(h.Name)))
		}
		e := Archive_TarFile{F_path: p, F_size: h.Size, F_mode: h.Mode & 0o7777, F_modified: Time(h.ModTime.UnixNano())}
		switch h.Typeflag {
		case tar.TypeDir:
			e.F_is_dir, e.F_size = true, 0
		case tar.TypeXGlobalHeader:
			continue
		case tar.TypeReg:
		default:
			x.done = true
			return Err[Option[Archive_TarFile]](archiveErr("tar_reader: entry " + strconv.Quote(h.Name) + " is not a file or a directory"))
		}
		return Ok[Option[Archive_TarFile], Archive_ArchiveErr](Some(e))
	}
	return Ok[Option[Archive_TarFile], Archive_ArchiveErr](None[Archive_TarFile]())
}

func (x *Archive_TarReader) M_read(tk *Task, max int64) Result[Option[Bytes], Archive_ArchiveErr] {
	x.fr.tk = tk
	if max <= 0 {
		return Ok[Option[Bytes], Archive_ArchiveErr](Some(Bytes("")))
	}
	if x.done {
		return Ok[Option[Bytes], Archive_ArchiveErr](None[Bytes]())
	}
	buf := make([]byte, max)
	n, err := x.tr.Read(buf)
	if n > 0 {
		return Ok[Option[Bytes], Archive_ArchiveErr](Some(Bytes(buf[:n])))
	}
	if err == io.EOF {
		return Ok[Option[Bytes], Archive_ArchiveErr](None[Bytes]())
	}
	if err == nil {
		return Ok[Option[Bytes], Archive_ArchiveErr](Some(Bytes("")))
	}
	return Err[Option[Bytes]](x.fail(err))
}

// Archive_zip is `archive.zip(entries)`.
// archiveMethod is Go's method for a ZipMethod.
func archiveMethod(m Archive_ZipMethod) uint16 {
	if m.Tag == 2 {
		return zip.Store
	}
	return zip.Deflate
}

func Archive_zip(_ *Task, entries List[Archive_Entry], comment string, method Archive_ZipMethod, offset int64) Result[Bytes, Archive_ArchiveErr] {
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	w.SetOffset(offset)
	if err := w.SetComment(comment); err != nil {
		return Err[Bytes](archiveErr("zip: " + err.Error()))
	}
	for _, e := range entries.s {
		p, ok := archivePath(e.F_path)
		if !ok {
			return Err[Bytes](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
		}
		h := &zip.FileHeader{Name: p, Method: archiveMethod(method), Modified: entryTime(e)}
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
func Archive_unzip(_ *Task, data Bytes, max int64) Result[List[Archive_Entry], Archive_ArchiveErr] {
	if max < 0 {
		max = 0
	}
	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return Err[List[Archive_Entry]](archiveErr("unzip: " + err.Error()))
	}
	// the declared sizes bound what reading may hold (Go's reader stops a
	// file past its own with ErrFormat), so check their sum first
	var declared uint64
	for _, f := range r.File {
		declared += f.UncompressedSize64
		if declared > uint64(max) {
			return Err[List[Archive_Entry]](archiveErr("unzip: zip: files exceed max_size (" + strconv.FormatInt(max, 10) + " bytes)"))
		}
	}
	var out []Archive_Entry
	for _, f := range r.File {
		// a name from the archive: outside bytes (decision 0259)
		p, ok := archivePath(textOf(f.Name))
		if !ok {
			return Err[List[Archive_Entry]](archiveErr("unzip: entry path " + strconv.Quote(f.Name) + archivePathWhy(f.Name)))
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

// Archive_zip_reader is `archive.zip_reader(data)`: unzip's checks on every
// entry, none inflated. The Go backend keeps an entry's index in the
// ZipFile's offset, and reads an entry by opening the archive again.
func Archive_zip_reader(_ *Task, data Bytes) Result[Archive_ZipReader, Archive_ArchiveErr] {
	r, err := zip.NewReader(strings.NewReader(string(data)), int64(len(data)))
	if err != nil {
		return Err[Archive_ZipReader](archiveErr("zip_reader: " + err.Error()))
	}
	var out []Archive_ZipFile
	for i, f := range r.File {
		p, ok := archivePath(textOf(f.Name))
		if !ok {
			return Err[Archive_ZipReader](archiveErr("zip_reader: entry path " + strconv.Quote(f.Name) + archivePathWhy(f.Name)))
		}
		m := f.Mode()
		if !m.IsDir() && !m.IsRegular() {
			return Err[Archive_ZipReader](archiveErr("zip_reader: entry " + strconv.Quote(f.Name) + " is not a file or a directory"))
		}
		out = append(out, Archive_ZipFile{F_path: p, F_size: int64(f.UncompressedSize64), F_compressed: int64(f.CompressedSize64), F_mode: int64(m.Perm()),
			F_is_dir: m.IsDir(), F_modified: Time(f.Modified.UnixNano()), F_offset: int64(i), F_method: int64(f.Method), F_flags: int64(f.Flags), F_crc: int64(f.CRC32)})
	}
	return Ok[Archive_ZipReader, Archive_ArchiveErr](Archive_ZipReader{F_data: data, F_list: ListOf(out...), F_note: textOf(r.Comment)})
}

func (r *Archive_ZipReader) M_files(*Task) List[Archive_ZipFile] { return r.F_list }

// M_comment is Reader.Comment.
func (r *Archive_ZipReader) M_comment(*Task) string { return r.F_note }

func (r *Archive_ZipReader) M_read(_ *Task, f Archive_ZipFile) Result[Bytes, Archive_ArchiveErr] {
	if f.F_is_dir {
		return Ok[Bytes, Archive_ArchiveErr](Bytes(""))
	}
	zr, err := zip.NewReader(strings.NewReader(string(r.F_data)), int64(len(r.F_data)))
	if err != nil || f.F_offset < 0 || int(f.F_offset) >= len(zr.File) {
		return Err[Bytes](archiveErr("zip_reader: not a file of this archive"))
	}
	rc, err := zr.File[f.F_offset].Open()
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	b, err := io.ReadAll(rc)
	rc.Close()
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b))
}

// zipFileOf is the zip.File of f in r (the Go backend's offset is its
// index).
func (r *Archive_ZipReader) zipFileOf(f Archive_ZipFile) (*zip.File, error) {
	zr, err := zip.NewReader(strings.NewReader(string(r.F_data)), int64(len(r.F_data)))
	if err != nil || f.F_offset < 0 || int(f.F_offset) >= len(zr.File) {
		return nil, errors.New("not a file of this archive")
	}
	return zr.File[f.F_offset], nil
}

// M_raw is File.OpenRaw read to the end.
func (r *Archive_ZipReader) M_raw(_ *Task, f Archive_ZipFile) Result[Bytes, Archive_ArchiveErr] {
	zf, err := r.zipFileOf(f)
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	rd, err := zf.OpenRaw()
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	b, err := io.ReadAll(rd)
	if err != nil {
		return Err[Bytes](archiveErr("zip_reader: " + err.Error()))
	}
	return Ok[Bytes, Archive_ArchiveErr](Bytes(b))
}

// M_data_offset is File.DataOffset.
func (r *Archive_ZipReader) M_data_offset(_ *Task, f Archive_ZipFile) Result[int64, Archive_ArchiveErr] {
	zf, err := r.zipFileOf(f)
	if err != nil {
		return Err[int64](archiveErr("zip_reader: " + err.Error()))
	}
	off, err := zf.DataOffset()
	if err != nil {
		return Err[int64](archiveErr("zip_reader: " + err.Error()))
	}
	return Ok[int64, Archive_ArchiveErr](off)
}

// Archive_ZipWriter is `archive.ZipWriter` (move only): Go's zip.Writer over
// the File, which a Write adapter feeds with the current task.
type Archive_ZipWriter struct {
	zw  *zip.Writer
	fw  *zipFileWriter
	cur io.Writer
}

type zipFileWriter struct {
	f   File
	tk  *Task
	err string
}

func (z *zipFileWriter) Write(p []byte) (int, error) {
	if r := z.f.M_write(z.tk, Bytes(append([]byte(nil), p...))); !r.Ok {
		z.err = r.E.M_display(z.tk)
		return 0, io.ErrClosedPipe
	}
	return len(p), nil
}

func (Archive_ZipWriter) XoDebug(b *strings.Builder) { b.WriteString("<zip_writer>") }
func (x Archive_ZipWriter) M_debug(*Task) string     { return Debug(x) }

func Archive_zip_writer(_ *Task, f File) Archive_ZipWriter {
	fw := &zipFileWriter{f: f}
	return Archive_ZipWriter{zw: zip.NewWriter(fw), fw: fw}
}

func (x *Archive_ZipWriter) fail(tk *Task, err error) Result[Unit, Archive_ArchiveErr] {
	if x.fw.err != "" {
		return Err[Unit](archiveErr("zip_writer: " + x.fw.err))
	}
	return Err[Unit](archiveErr("zip_writer: " + err.Error()))
}

func (x *Archive_ZipWriter) start(tk *Task, name string, mode int64, isDir bool, mod time.Time, method uint16) error {
	h := &zip.FileHeader{Name: name, Method: method, Modified: mod}
	h.SetMode(fs.FileMode(mode & 0o777))
	if isDir {
		h.Name = name + "/"
		h.Method = zip.Store
		h.SetMode(fs.ModeDir | fs.FileMode(mode&0o777))
	}
	x.fw.tk = tk
	w, err := x.zw.CreateHeader(h)
	x.cur = w
	return err
}

func (x *Archive_ZipWriter) M_add(tk *Task, e Archive_Entry, method Archive_ZipMethod) Result[Unit, Archive_ArchiveErr] {
	p, ok := archivePath(textOf(e.F_path))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(e.F_path)))
	}
	if err := x.start(tk, p, e.F_mode, e.F_is_dir, entryTime(e), archiveMethod(method)); err != nil {
		return x.fail(tk, err)
	}
	if !e.F_is_dir {
		if _, err := x.cur.Write([]byte(e.F_data)); err != nil {
			return x.fail(tk, err)
		}
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_ZipWriter) M_create(tk *Task, p string, mode int64, method Archive_ZipMethod) Result[Unit, Archive_ArchiveErr] {
	clean, ok := archivePath(textOf(p))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(p)))
	}
	if err := x.start(tk, clean, mode, false, time.Unix(0, 0).UTC(), archiveMethod(method)); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_ZipWriter) M_write(tk *Task, data Bytes) Result[Unit, Archive_ArchiveErr] {
	if x.cur == nil {
		return Err[Unit](archiveErr("zip_writer: write without create"))
	}
	x.fw.tk = tk
	if _, err := x.cur.Write([]byte(data)); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// M_copy is Writer.Copy.
func (x *Archive_ZipWriter) M_copy(tk *Task, r Archive_ZipReader, f Archive_ZipFile) Result[Unit, Archive_ArchiveErr] {
	zf, err := r.zipFileOf(f)
	if err != nil {
		return Err[Unit](archiveErr("zip_writer: " + err.Error()))
	}
	x.fw.tk = tk
	x.cur = nil
	if err := x.zw.Copy(zf); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// M_add_raw is Writer.CreateRaw of f's header (archive.xo) and raw.
func (x *Archive_ZipWriter) M_add_raw(tk *Task, f Archive_ZipFile, raw Bytes) Result[Unit, Archive_ArchiveErr] {
	p, ok := archivePath(textOf(f.F_path))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(f.F_path)))
	}
	h := &zip.FileHeader{Name: p, Method: uint16(f.F_method), Flags: uint16(f.F_flags), CRC32: uint32(f.F_crc),
		CompressedSize64: uint64(f.F_compressed), UncompressedSize64: uint64(f.F_size), CreatorVersion: 0x0314, ReaderVersion: 20}
	h.SetMode(fs.FileMode(f.F_mode & 0o777))
	if f.F_is_dir {
		h.Name = p + "/"
		h.SetMode(fs.ModeDir | fs.FileMode(f.F_mode&0o777))
	}
	h.SetModTime(time.Unix(0, int64(f.F_modified)).UTC())
	x.fw.tk = tk
	w, err := x.zw.CreateRaw(h)
	x.cur = nil
	if err != nil {
		return x.fail(tk, err)
	}
	if _, err := w.Write([]byte(raw)); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// M_add_fs is Writer.AddFS over src.walk(".") (archive.xo).
func (x *Archive_ZipWriter) M_add_fs(tk *Task, src Fs) Result[Unit, Archive_ArchiveErr] {
	l := src.M_walk(tk, ".")
	if !l.Ok {
		return Err[Unit](archiveErr("zip_writer: " + l.E.M_display(tk)))
	}
	for _, fi := range l.V.s {
		if fi.F_is_symlink {
			return Err[Unit](archiveErr("zip_writer: zip: cannot add non-regular file " + strconv.Quote(fi.F_path)))
		}
		m := src.M_mode(tk, fi.F_path)
		if !m.Ok {
			return Err[Unit](archiveErr("zip_writer: " + m.E.M_display(tk)))
		}
		p, ok := archivePath(textOf(fi.F_path))
		if !ok {
			return Err[Unit](archiveErr("bad entry path " + strconv.Quote(fi.F_path)))
		}
		mod := time.Unix(0, int64(fi.F_modified)).UTC()
		if err := x.start(tk, p, m.V, fi.F_is_dir, mod, zip.Deflate); err != nil {
			return x.fail(tk, err)
		}
		if fi.F_is_dir {
			continue
		}
		if msg, err := archiveFsCopy(tk, src, fi.F_path, x.cur); msg != "" {
			return Err[Unit](archiveErr("zip_writer: " + msg))
		} else if err != nil {
			return x.fail(tk, err)
		}
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// M_set_comment is Writer.SetComment.
func (x *Archive_ZipWriter) M_set_comment(tk *Task, comment string) Result[Unit, Archive_ArchiveErr] {
	if err := x.zw.SetComment(comment); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// M_set_offset is Writer.SetOffset; its panic is a fault.
func (x *Archive_ZipWriter) M_set_offset(_ *Task, n int64) {
	defer func() {
		if r := recover(); r != nil {
			FaultHere("%v", r)
		}
	}()
	x.zw.SetOffset(n)
}

func (x *Archive_ZipWriter) M_close(tk *Task) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if err := x.zw.Close(); err != nil {
		return x.fail(tk, err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// Archive_TarWriter is `archive.TarWriter` (move only): Go's tar.Writer
// over the File.
type Archive_TarWriter struct {
	tw *tar.Writer
	fw *zipFileWriter
}

func (Archive_TarWriter) XoDebug(b *strings.Builder) { b.WriteString("<tar_writer>") }
func (x Archive_TarWriter) M_debug(*Task) string     { return Debug(x) }

func Archive_tar_writer(_ *Task, f File) Archive_TarWriter {
	fw := &zipFileWriter{f: f}
	return Archive_TarWriter{tw: tar.NewWriter(fw), fw: fw}
}

func (x *Archive_TarWriter) fail(err error) Result[Unit, Archive_ArchiveErr] {
	if x.fw.err != "" {
		return Err[Unit](archiveErr("tar_writer: " + x.fw.err))
	}
	return Err[Unit](archiveErr("tar_writer: " + err.Error()))
}

func (x *Archive_TarWriter) start(tk *Task, path string, mode, size int64, isDir bool, mod time.Time) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if err := x.tw.Flush(); err != nil {
		return x.fail(err)
	}
	p, ok := archivePath(textOf(path))
	if !ok {
		return Err[Unit](archiveErr("bad entry path " + strconv.Quote(path)))
	}
	if size < 0 {
		return Err[Unit](archiveErr("tar_writer: archive/tar: negative size"))
	}
	h := &tar.Header{Name: p, Mode: mode & 0o7777, ModTime: mod, Typeflag: tar.TypeReg, Size: size, Format: tar.FormatPAX}
	if isDir {
		h.Typeflag, h.Name, h.Size = tar.TypeDir, p+"/", 0
	}
	if err := x.tw.WriteHeader(h); err != nil {
		return x.fail(err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_TarWriter) M_add(tk *Task, e Archive_Entry) Result[Unit, Archive_ArchiveErr] {
	if r := x.start(tk, e.F_path, e.F_mode, int64(len(e.F_data)), e.F_is_dir, entryTime(e)); !r.Ok {
		return r
	}
	if !e.F_is_dir {
		return x.M_write(tk, e.F_data)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_TarWriter) M_create(tk *Task, path string, size, mode int64, mod Time) Result[Unit, Archive_ArchiveErr] {
	return x.start(tk, path, mode, size, false, time.Unix(0, int64(mod)).UTC())
}

func (x *Archive_TarWriter) M_write(tk *Task, data Bytes) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if _, err := x.tw.Write([]byte(data)); err != nil {
		return x.fail(err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_TarWriter) M_flush(tk *Task) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if err := x.tw.Flush(); err != nil {
		return x.fail(err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// archiveFsCopy copies p of src into w in 64 KiB reads (io.Copy in Go's
// AddFS); a read error is its FsErr text.
func archiveFsCopy(tk *Task, src Fs, p string, w io.Writer) (string, error) {
	o := src.M_open(tk, p)
	if !o.Ok {
		return o.E.M_display(tk), nil
	}
	f := o.V
	msg, err := archiveFsCopyFile(tk, f, w)
	if c := f.M_close(tk); !c.Ok && msg == "" && err == nil {
		return c.E.M_display(tk), nil
	}
	return msg, err
}

func archiveFsCopyFile(tk *Task, f File, w io.Writer) (string, error) {
	for {
		r := f.M_read(tk, 65536)
		if !r.Ok {
			return r.E.M_display(tk), nil
		}
		if !r.V.Some {
			return "", nil
		}
		if _, err := w.Write([]byte(r.V.V)); err != nil {
			return "", err
		}
	}
}

// M_add_fs is Writer.AddFS over src.walk(".") (archive.xo).
func (x *Archive_TarWriter) M_add_fs(tk *Task, src Fs) Result[Unit, Archive_ArchiveErr] {
	l := src.M_walk(tk, ".")
	if !l.Ok {
		return Err[Unit](archiveErr("tar_writer: " + l.E.M_display(tk)))
	}
	for _, fi := range l.V.s {
		if fi.F_is_symlink {
			return Err[Unit](archiveErr("tar_writer: tar: cannot add non-regular file " + strconv.Quote(fi.F_path)))
		}
		m := src.M_mode(tk, fi.F_path)
		if !m.Ok {
			return Err[Unit](archiveErr("tar_writer: " + m.E.M_display(tk)))
		}
		mod := time.Unix(0, int64(fi.F_modified)).UTC()
		if r := x.start(tk, fi.F_path, m.V, fi.F_size, fi.F_is_dir, mod); !r.Ok {
			return r
		}
		if fi.F_is_dir {
			continue
		}
		x.fw.tk = tk
		if msg, err := archiveFsCopy(tk, src, fi.F_path, x.tw); msg != "" {
			return Err[Unit](archiveErr("tar_writer: " + msg))
		} else if err != nil {
			return x.fail(err)
		}
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

func (x *Archive_TarWriter) M_close(tk *Task) Result[Unit, Archive_ArchiveErr] {
	x.fw.tk = tk
	if err := x.tw.Close(); err != nil {
		return x.fail(err)
	}
	return Ok[Unit, Archive_ArchiveErr](Unit{})
}

// Archive_zip_open is `archive.zip_open(f, size)`: the Go backend reads the
// File whole through read_at and opens it as zip_reader does (its memory is
// the archive's; the native backends read the directory and each entry).
func Archive_zip_open(tk *Task, f File, size int64) Result[Archive_ZipReader, Archive_ArchiveErr] {
	var buf []byte
	for int64(len(buf)) < size {
		r := f.M_read_at(tk, int64(len(buf)), size-int64(len(buf)))
		if !r.Ok {
			return Err[Archive_ZipReader](archiveErr("zip_open: " + r.E.M_display(tk)))
		}
		if !r.V.Some || len(r.V.V) == 0 {
			return Err[Archive_ZipReader](archiveErr("zip_open: unexpected EOF"))
		}
		buf = append(buf, r.V.V...)
	}
	r := Archive_zip_reader(tk, Bytes(buf))
	if !r.Ok {
		return Err[Archive_ZipReader](archiveErr("zip_open:" + strings.TrimPrefix(r.E.F_msg, "zip_reader:")))
	}
	return r
}

// archivePathWhy is the end of an entry path error: a NUL byte is its own
// reason, every other refused name leaves the archive.
func archivePathWhy(name string) string {
	if strings.Contains(name, "\x00") {
		return " contains a NUL byte"
	}
	return " leaves the archive"
}
