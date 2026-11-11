package rt

import (
	"bytes"
	"io"
	iofs "io/fs"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Http_file_server is `http.file_server(fs, strip)` (decision 0607):
// net/http's FileServer, behind StripPrefix when strip is set, over an
// http.FileSystem that reads through the Xo Fs, with serve_content's
// recorder for the response.
func Http_file_server(tk *Task, fs Fs, strip string) func(*Task, Http_Request) Result[Http_Response, Never] {
	return func(tk *Task, req Http_Request) Result[Http_Response, Never] {
		var h http.Handler = http.FileServer(xoFileSystem{tk: tk, fs: fs})
		if strip != "" {
			h = http.StripPrefix(strip, h)
		}
		hr := &http.Request{Method: req.F_method, URL: &url.URL{Path: req.F_path, RawQuery: req.XRawQuery}, Header: http.Header{}, Host: req.F_url.F_host}
		for k, v := range Map_All(req.F_headers) {
			hr.Header.Set(k, v)
		}
		w := &serveRecorder{h: http.Header{}}
		h.ServeHTTP(w, hr)
		code := w.code
		if code == 0 {
			code = http.StatusOK
		}
		var hm Map[string, string]
		var fields []string
		for k, vs := range w.h {
			Map_put(tk, &hm, k, fieldView(k, vs))
			if len(vs) > 1 {
				fields = appendFields(fields, k, vs)
			}
		}
		return Ok[Http_Response, Never](Http_Response{F_status: int64(code), F_headers: hm, F_body: Bytes(w.body.Bytes()), XFields: fields})
	}
}

// xoFileSystem is an http.FileSystem over an Xo Fs: names are the
// FileServer's cleaned slash paths, read relative to the Fs's root.
type xoFileSystem struct {
	tk *Task
	fs Fs
}

func xoFsGoErr(e FsErr, name string) error {
	switch e.Tag {
	case 1, 4: // NotFound, NotDir: mapOpenError's ErrNotExist
		return &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrNotExist}
	case 6: // Denied
		return &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrPermission}
	}
	return &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrInvalid}
}

func (x xoFileSystem) Open(name string) (http.File, error) {
	p := strings.TrimPrefix(name, "/")
	if p == "" {
		p = "."
	}
	st := x.fs.M_stat(x.tk, p)
	if !st.Ok {
		return nil, xoFsGoErr(st.E, name)
	}
	if !st.V.Some {
		return nil, &iofs.PathError{Op: "open", Path: name, Err: iofs.ErrNotExist}
	}
	return &xoFile{x: x, p: p, info: st.V.V}, nil
}

// xoFile reads its bytes on the first Read or Seek.
type xoFile struct {
	x    xoFileSystem
	p    string
	info FileInfo
	r    *bytes.Reader
}

func (f *xoFile) load() error {
	if f.r != nil {
		return nil
	}
	b := f.x.fs.M_read(f.x.tk, f.p)
	if !b.Ok {
		return xoFsGoErr(b.E, f.p)
	}
	f.r = bytes.NewReader([]byte(b.V))
	return nil
}

func (f *xoFile) Close() error { return nil }

func (f *xoFile) Read(b []byte) (int, error) {
	if err := f.load(); err != nil {
		return 0, err
	}
	return f.r.Read(b)
}

func (f *xoFile) Seek(off int64, whence int) (int64, error) {
	if err := f.load(); err != nil {
		return 0, err
	}
	return f.r.Seek(off, whence)
}

func (f *xoFile) Readdir(count int) ([]iofs.FileInfo, error) {
	l := f.x.fs.M_read_dir(f.x.tk, f.p)
	if !l.Ok {
		return nil, xoFsGoErr(l.E, f.p)
	}
	var out []iofs.FileInfo
	for _, i := range l.V.s {
		out = append(out, xoFileInfo{i})
	}
	if count > 0 && len(out) == 0 {
		return nil, io.EOF
	}
	return out, nil
}

func (f *xoFile) Stat() (iofs.FileInfo, error) { return xoFileInfo{f.info}, nil }

type xoFileInfo struct{ i FileInfo }

func (i xoFileInfo) Name() string {
	if i.i.F_name == "" {
		return "."
	}
	return i.i.F_name
}
func (i xoFileInfo) Size() int64 { return i.i.F_size }
func (i xoFileInfo) Mode() iofs.FileMode {
	if i.i.F_is_dir {
		return iofs.ModeDir | 0o755
	}
	return 0o644
}
func (i xoFileInfo) ModTime() time.Time { return time.Unix(0, int64(i.i.F_modified)).UTC() }
func (i xoFileInfo) IsDir() bool        { return i.i.F_is_dir }
func (i xoFileInfo) Sys() any           { return nil }
