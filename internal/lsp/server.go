// Package lsp is the Language Server Protocol server behind `xo lsp`:
// JSON-RPC 2.0 over stdio with Content-Length framing (LSP 3.17).
//
// The server keeps the text of open documents and rechecks the modules
// that hold them with check.CheckOpts, the open texts as an overlay, after
// each change (debounced) and before each request. Every answer comes from
// that one check result: diagnostics with their fixes as quick fixes,
// hover, definition, references, rename, document symbols, semantic tokens,
// and formatting (internal/format, the same layout as `xo fmt`).
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/query"
)

// Server is one LSP session.
type Server struct {
	// Version is reported in serverInfo.
	Version string
	// Delay is how long after the last change diagnostics are recomputed.
	Delay time.Duration
	// StdDir is where std sources are written so definitions in std can be
	// opened (default <user cache dir>/xo/lsp-std).
	StdDir string

	w         *writer
	docs      map[string][]byte // absolute path to open text
	snap      *snapshot
	dirty     bool
	published map[string]bool
	wake      chan struct{}
	timer     *time.Timer
	shutdown  bool
}

// snapshot is one check of the open documents' modules.
type snapshot struct {
	stdDir string
	res    *check.Result
	info   *check.Info
	sess   *query.Session
	files  map[string]*check.File // by absolute path; std files by "std/<name>.xo"
	texts  map[string]*text
	diags  map[string][]diag.Diagnostic // by absolute path
	sites  map[*check.File][]identSite
	err    error
}

// identsOf returns the identifiers of f, walking its tree once per snapshot.
func (snap *snapshot) identsOf(f *check.File) []identSite {
	if snap.sites == nil {
		snap.sites = map[*check.File][]identSite{}
	}
	ss, ok := snap.sites[f]
	if !ok {
		ss = idents(f.AST)
		snap.sites[f] = ss
	}
	return ss
}

// Serve runs the session until the client sends exit or r ends.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	if s.Delay == 0 {
		s.Delay = 150 * time.Millisecond
	}
	if s.StdDir == "" {
		if d, err := os.UserCacheDir(); err == nil {
			s.StdDir = filepath.Join(d, "xo", "lsp-std")
		}
	}
	s.w = &writer{w: w}
	s.docs = map[string][]byte{}
	s.published = map[string]bool{}
	s.wake = make(chan struct{}, 1)

	msgs := make(chan *message)
	errc := make(chan error, 1)
	go func() {
		br := bufio.NewReader(r)
		for {
			m, err := readMessage(br)
			if err != nil {
				errc <- err
				return
			}
			msgs <- m
		}
	}()
	for {
		select {
		case m := <-msgs:
			if m.Method == "exit" {
				return nil
			}
			s.handle(m)
		case <-s.wake:
			if s.dirty {
				s.recheck()
			}
		case err := <-errc:
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func (s *Server) handle(m *message) {
	if m.ID == nil {
		s.notification(m)
		return
	}
	result, err := s.request(m)
	if err != nil {
		code := codeRequestFailed
		if e, ok := err.(*rpcError); ok {
			code = e.Code
			err = fmt.Errorf("%s", e.Message)
		}
		s.w.replyError(m.ID, code, err.Error())
		return
	}
	s.w.reply(m.ID, result)
}

func (e *rpcError) Error() string { return e.Message }

func decode(m *message, v any) error {
	if err := json.Unmarshal(m.Params, v); err != nil {
		return &rpcError{Code: codeInvalidParams, Message: err.Error()}
	}
	return nil
}

func (s *Server) notification(m *message) {
	switch m.Method {
	case "textDocument/didOpen":
		var p didOpenParams
		if decode(m, &p) == nil {
			if path := uriToPath(p.TextDocument.URI); path != "" {
				s.docs[path] = []byte(p.TextDocument.Text)
				s.changed(0)
			}
		}
	case "textDocument/didChange":
		var p didChangeParams
		if decode(m, &p) == nil {
			path := uriToPath(p.TextDocument.URI)
			src, ok := s.docs[path]
			if !ok {
				return
			}
			for _, c := range p.ContentChanges {
				if c.Range == nil {
					src = []byte(c.Text)
					continue
				}
				t := newText(src)
				a, b := t.offset(c.Range.Start), t.offset(c.Range.End)
				src = append(append(append([]byte(nil), src[:a]...), c.Text...), src[b:]...)
			}
			s.docs[path] = src
			s.changed(s.Delay)
		}
	case "textDocument/didClose":
		var p documentParams
		if decode(m, &p) == nil {
			delete(s.docs, uriToPath(p.TextDocument.URI))
			s.changed(0)
		}
	case "textDocument/didSave", "workspace/didChangeWatchedFiles":
		s.changed(0)
	}
}

// changed marks the snapshot stale and schedules a recheck after d.
func (s *Server) changed(d time.Duration) {
	s.dirty = true
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(d, func() {
		select {
		case s.wake <- struct{}{}:
		default:
		}
	})
}

// fresh returns a snapshot of the current texts, rechecking if needed.
func (s *Server) fresh() *snapshot {
	if s.dirty || s.snap == nil {
		s.recheck()
	}
	return s.snap
}

// recheck checks the modules of the open documents and publishes their
// diagnostics.
func (s *Server) recheck() {
	s.dirty = false
	seen := map[string]bool{}
	var dirs []string
	overlay := map[string][]byte{}
	for p, src := range s.docs {
		if s.isStdCopy(p) {
			// a std source opened through a definition: it is served from
			// the std module, never checked as user code
			continue
		}
		overlay[p] = src
		if d := filepath.Dir(p); !seen[d] {
			seen[d] = true
			dirs = append(dirs, d)
		}
	}
	sort.Strings(dirs)
	snap := &snapshot{stdDir: s.StdDir, files: map[string]*check.File{}, texts: map[string]*text{}, diags: map[string][]diag.Diagnostic{}}
	s.snap = snap
	if len(dirs) == 0 {
		s.publish(snap)
		return
	}
	res, err := check.CheckOpts(mod.WorkspaceDirs(dirs), check.Options{Overlay: overlay})
	if err != nil {
		snap.err = err
		s.w.notify("window/logMessage", map[string]any{"type": sevWarning, "message": "xo: " + err.Error()})
		s.publish(snap)
		return
	}
	snap.res = res
	snap.info = res.Info()
	snap.sess = &query.Session{Res: res, Info: snap.info}
	for _, m := range snap.info.AllModules() {
		for _, f := range m.Files {
			snap.files[snap.key(f.Path, m)] = f
		}
	}
	for _, d := range res.Diagnostics {
		p := absPath(d.Span.File)
		snap.diags[p] = append(snap.diags[p], d)
	}
	s.publish(snap)
}

func (snap *snapshot) key(path string, m *check.Module) string {
	if m != nil && m.IsStd {
		return path
	}
	return absPath(path)
}

// stdKey maps a std copy under stdDir to its std file path ("std/json.xo").
func stdKey(stdDir, path string) (string, bool) {
	if stdDir == "" {
		return "", false
	}
	rel, err := filepath.Rel(stdDir, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return filepath.ToSlash(rel), true
}

func (s *Server) isStdCopy(path string) bool {
	_, ok := stdKey(s.StdDir, path)
	return ok
}

func absPath(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}

// file returns the checked file at path and its text.
func (snap *snapshot) file(path string) (*check.File, *text) {
	f := snap.files[path]
	if f == nil {
		if key, ok := stdKey(snap.stdDir, path); ok {
			if f = snap.files[key]; f != nil {
				return f, snap.text(key, f.Src)
			}
		}
	}
	if f == nil {
		return nil, nil
	}
	return f, snap.text(path, f.Src)
}

func (snap *snapshot) text(path string, src []byte) *text {
	t := snap.texts[path]
	if t == nil {
		t = newText(src)
		snap.texts[path] = t
	}
	return t
}

func (s *Server) publish(snap *snapshot) {
	now := map[string]bool{}
	all := map[string][]diag.Diagnostic{}
	for p := range s.docs {
		all[p] = nil
	}
	for p, ds := range snap.diags {
		all[p] = ds
	}
	for p, ds := range all {
		f, t := snap.file(p)
		if f == nil {
			src, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			t = snap.text(p, src)
		}
		out := make([]lspDiagnostic, 0, len(ds))
		for _, d := range ds {
			out = append(out, toLSP(d, t))
		}
		now[p] = true
		s.w.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: pathToURI(p), Diagnostics: out})
	}
	for p := range s.published {
		if !now[p] {
			s.w.notify("textDocument/publishDiagnostics", publishDiagnosticsParams{URI: pathToURI(p), Diagnostics: []lspDiagnostic{}})
		}
	}
	s.published = now
}

func spanRange(sp diag.Span, t *text) lspRange {
	start := t.position(sp.Line, sp.Col)
	end := start
	if sp.EndLine > 0 {
		end = t.position(sp.EndLine, sp.EndCol)
	}
	return lspRange{Start: start, End: end}
}

func toLSP(d diag.Diagnostic, t *text) lspDiagnostic {
	sev := sevError
	switch d.Severity {
	case diag.Warning:
		sev = sevWarning
	case diag.Note:
		sev = sevInformation
	}
	msg := d.Message
	for _, n := range d.Notes {
		msg += "\nnote: " + n.Message
	}
	return lspDiagnostic{Range: spanRange(d.Span, t), Severity: sev, Code: d.Code, Source: "xo", Message: msg}
}

func (s *Server) request(m *message) (any, error) {
	switch m.Method {
	case "initialize":
		return s.initialize(), nil
	case "shutdown":
		s.shutdown = true
		return nil, nil
	case "textDocument/hover":
		var p textDocumentPositionParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.hover(p)
	case "textDocument/definition":
		var p textDocumentPositionParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.definition(p)
	case "textDocument/references":
		var p referenceParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.references(p.textDocumentPositionParams, p.Context.IncludeDeclaration)
	case "textDocument/prepareRename":
		var p textDocumentPositionParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.prepareRename(p)
	case "textDocument/rename":
		var p renameParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.rename(p)
	case "textDocument/formatting":
		var p documentParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.format(p)
	case "textDocument/codeAction":
		var p codeActionParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.codeActions(p)
	case "textDocument/documentSymbol":
		var p documentParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.symbols(p)
	case "textDocument/semanticTokens/full":
		var p documentParams
		if err := decode(m, &p); err != nil {
			return nil, err
		}
		return s.semanticTokens(p)
	}
	return nil, &rpcError{Code: codeMethodNotFound, Message: "method not supported: " + m.Method}
}

func (s *Server) initialize() any {
	return map[string]any{
		"serverInfo": map[string]any{"name": "xo", "version": s.Version},
		"capabilities": map[string]any{
			"positionEncoding": "utf-16",
			"textDocumentSync": map[string]any{
				"openClose": true,
				"change":    2, // incremental
				"save":      map[string]any{"includeText": false},
			},
			"hoverProvider":              true,
			"definitionProvider":         true,
			"referencesProvider":         true,
			"renameProvider":             map[string]any{"prepareProvider": true},
			"documentFormattingProvider": true,
			"documentSymbolProvider":     true,
			"codeActionProvider":         map[string]any{"codeActionKinds": []string{"quickfix"}},
			"semanticTokensProvider": map[string]any{
				"legend": map[string]any{"tokenTypes": tokenTypes, "tokenModifiers": tokenModifiers},
				"full":   true,
			},
		},
	}
}

// at finds the identifier at an LSP position in an open document.
func (s *Server) at(p textDocumentPositionParams) (*snapshot, *check.File, *text, *identSite, error) {
	snap := s.fresh()
	path := uriToPath(p.TextDocument.URI)
	f, t := snap.file(path)
	if f == nil {
		if snap.err != nil {
			return snap, nil, nil, nil, snap.err
		}
		return snap, nil, nil, nil, nil
	}
	off := t.offset(p.Position)
	for _, site := range snap.identsOf(f) {
		sp := site.id.Sp
		if sp.Start.Offset <= off && off <= sp.End.Offset && site.id.Name != "" {
			site := site
			return snap, f, t, &site, nil
		}
	}
	return snap, f, t, nil, nil
}

// spanLocation converts a byte range of a checked file, writing std
// sources to StdDir so the editor can open them.
func (s *Server) spanLocation(snap *snapshot, file string, startOff, endOff int) (*location, bool) {
	path := file
	f := snap.files[file]
	if f == nil {
		path = absPath(file)
		f = snap.files[path]
	}
	if f == nil {
		return nil, false
	}
	t := snap.text(path, f.Src)
	if f.Module != nil && f.Module.IsStd {
		if s.StdDir == "" {
			return nil, false
		}
		dst := filepath.Join(s.StdDir, filepath.FromSlash(f.Path))
		if old, err := os.ReadFile(dst); err != nil || string(old) != string(f.Src) {
			// read only, so the editor shows it cannot be edited; the old
			// copy is removed first because it is read only too
			os.Remove(dst)
			if os.MkdirAll(filepath.Dir(dst), 0o755) != nil || os.WriteFile(dst, f.Src, 0o444) != nil {
				return nil, false
			}
		}
		path = dst
	}
	return &location{URI: pathToURI(path), Range: lspRange{Start: t.offsetPosition(startOff), End: t.offsetPosition(endOff)}}, true
}

func (s *Server) definition(p textDocumentPositionParams) (any, error) {
	snap, f, _, site, err := s.at(p)
	if err != nil || site == nil {
		return nil, err
	}
	tg := snap.resolve(f, *site)
	if tg.def.File == "" {
		return nil, nil
	}
	loc, ok := s.spanLocation(snap, tg.def.File, tg.def.Start.Offset, tg.def.End.Offset)
	if !ok {
		return nil, nil
	}
	return loc, nil
}

// refs returns every identifier in the user files that names the same
// entity as tg.
func (snap *snapshot) refs(tg target) []identSite {
	var out []identSite
	paths := make([]string, 0, len(snap.files))
	for p, f := range snap.files {
		if f.Module != nil && !f.Module.IsStd {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	for _, p := range paths {
		f := snap.files[p]
		for _, site := range snap.identsOf(f) {
			if site.id.Name == "" {
				continue
			}
			if sameObj(snap.resolve(f, site), tg) {
				out = append(out, site)
			}
		}
	}
	return out
}

func (s *Server) references(p textDocumentPositionParams, withDecl bool) (any, error) {
	snap, f, _, site, err := s.at(p)
	if err != nil || site == nil {
		return nil, err
	}
	tg := snap.resolve(f, *site)
	if tg.obj == nil && tg.def.File == "" {
		return []location{}, nil
	}
	out := []location{}
	for _, r := range snap.refs(tg) {
		sp := r.id.Sp
		if !withDecl && sp == tg.def {
			continue
		}
		if loc, ok := s.spanLocation(snap, sp.File, sp.Start.Offset, sp.End.Offset); ok {
			out = append(out, *loc)
		}
	}
	return out, nil
}

// renamable reports whether tg is declared in a user file.
func (snap *snapshot) renamable(tg target) bool {
	if tg.def.File == "" || tg.kind == "namespace" {
		return false
	}
	f := snap.files[absPath(tg.def.File)]
	return f != nil && f.Module != nil && !f.Module.IsStd
}

func (s *Server) prepareRename(p textDocumentPositionParams) (any, error) {
	snap, f, t, site, err := s.at(p)
	if err != nil || site == nil {
		return nil, err
	}
	if !snap.renamable(snap.resolve(f, *site)) {
		return nil, &rpcError{Code: codeRequestFailed, Message: "cannot rename this name"}
	}
	sp := site.id.Sp
	return lspRange{Start: t.offsetPosition(sp.Start.Offset), End: t.offsetPosition(sp.End.Offset)}, nil
}

func (s *Server) rename(p renameParams) (any, error) {
	snap, f, _, site, err := s.at(p.textDocumentPositionParams)
	if err != nil || site == nil {
		return nil, err
	}
	tg := snap.resolve(f, *site)
	if !snap.renamable(tg) {
		return nil, &rpcError{Code: codeRequestFailed, Message: "cannot rename this name"}
	}
	if !isIdent(p.NewName) {
		return nil, &rpcError{Code: codeRequestFailed, Message: fmt.Sprintf("%q is not an identifier", p.NewName)}
	}
	we := workspaceEdit{Changes: map[string][]textEdit{}}
	for _, r := range snap.refs(tg) {
		sp := r.id.Sp
		loc, ok := s.spanLocation(snap, sp.File, sp.Start.Offset, sp.End.Offset)
		if !ok {
			continue
		}
		we.Changes[loc.URI] = append(we.Changes[loc.URI], textEdit{Range: loc.Range, NewText: p.NewName})
	}
	return we, nil
}

func isIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if !(c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func (s *Server) codeActions(p codeActionParams) (any, error) {
	snap := s.fresh()
	path := uriToPath(p.TextDocument.URI)
	f, t := snap.file(path)
	if f == nil {
		return []codeAction{}, nil
	}
	out := []codeAction{}
	for _, d := range snap.diags[path] {
		ld := toLSP(d, t)
		if !overlaps(ld.Range, p.Range) {
			continue
		}
		for i, fx := range d.Fixes {
			we := workspaceEdit{Changes: map[string][]textEdit{}}
			for _, e := range fx.Edits {
				ep := absPath(e.Span.File)
				et := t
				if ep != path {
					ef, eft := snap.file(ep)
					if ef == nil {
						continue
					}
					et = eft
				}
				we.Changes[pathToURI(ep)] = append(we.Changes[pathToURI(ep)], textEdit{Range: spanRange(e.Span, et), NewText: e.Replace})
			}
			out = append(out, codeAction{Title: fx.Title, Kind: "quickfix", Diagnostics: []lspDiagnostic{ld}, IsPreferred: i == 0 && len(d.Fixes) == 1, Edit: &we})
		}
	}
	return out, nil
}

func overlaps(a, b lspRange) bool {
	return !less(a.End, b.Start) && !less(b.End, a.Start)
}

func less(a, b position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

func (s *Server) hover(p textDocumentPositionParams) (any, error) {
	snap, f, t, site, err := s.at(p)
	if err != nil || f == nil {
		return nil, err
	}
	var parts []string
	var rng *lspRange
	if site != nil {
		sp := site.id.Sp
		rng = &lspRange{Start: t.offsetPosition(sp.Start.Offset), End: t.offsetPosition(sp.End.Offset)}
		sig, doc := snap.describe(snap.resolve(f, *site))
		if sig != "" {
			parts = append(parts, "```xo\n"+sig+"\n```")
		}
		if doc != "" {
			parts = append(parts, doc)
		}
	}
	if len(parts) == 0 {
		line, col := t.col(p.Position)
		if r, err := snap.sess.Type(f.Path, line, col); err == nil {
			parts = append(parts, "```xo\n"+r.Type+"\n```")
		}
	}
	if len(parts) == 0 {
		return nil, nil
	}
	return hover{Contents: markupContent{Kind: "markdown", Value: strings.Join(parts, "\n\n")}, Range: rng}, nil
}

// describe returns a declaration line and doc text for a hover.
func (snap *snapshot) describe(tg target) (string, string) {
	surface := func(f *check.File, d ast.Decl) (string, string) {
		if f == nil || d == nil {
			return "", ""
		}
		def, ok := query.DeclSurface(f.Module.Path, f.Src, d, true)
		if !ok {
			return "", ""
		}
		return def.Signature, trimDoc(def.Doc)
	}
	switch o := tg.obj.(type) {
	case *check.FuncObj:
		if o.Decl != nil && o.File != nil {
			return surface(o.File, o.Decl)
		}
		return o.DisplayName(), ""
	case *check.ConstObj:
		if o.Decl != nil && o.File != nil {
			return surface(o.File, o.Decl)
		}
	case *check.TypeObj:
		f, d := declOf(o)
		if d != nil {
			return surface(f, d)
		}
		return o.QualName(), ""
	case *check.VarObj:
		kw := "let "
		switch {
		case o.Kind == check.VParam || o.Kind == check.VRecv:
			kw = ""
		case o.Mutable:
			kw = "var "
		}
		if o.Type == nil {
			return kw + o.Name, ""
		}
		return kw + o.Name + ": " + check.Deep(o.Type).String(), ""
	case *check.FieldObj:
		return o.Name + ": " + o.Type.String(), ""
	case *check.VariantObj:
		s := o.Enum.Name + "." + o.Name
		if !o.Unit {
			var fs []string
			for _, vf := range o.Fields {
				switch {
				case vf.From:
					fs = append(fs, "from "+vf.Type.String())
				case vf.Name != "":
					fs = append(fs, vf.Name+": "+vf.Type.String())
				default:
					fs = append(fs, vf.Type.String())
				}
			}
			s += "(" + strings.Join(fs, ", ") + ")"
		}
		return s, ""
	case *check.Module:
		return "use " + o.Path, ""
	}
	if tg.label != "" {
		return tg.label, ""
	}
	return "", ""
}
