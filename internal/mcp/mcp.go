// Package mcp is the Model Context Protocol server behind `xo mcp`: JSON-RPC
// 2.0 over stdio, one message per line, exposing the toolchain (check,
// query, explain, fmt, test, build) as tools that return compact JSON.
//
// Each tool runs the matching xo command line in process through Exec with
// --json and reshapes the result, so the tools answer exactly what the CLI
// answers (and use `xo serve` when the CLI would).
package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// ProtocolVersion is the MCP revision this server implements. A client
// asking for another revision gets this one back (the spec lets the client
// decide whether to continue).
const ProtocolVersion = "2025-06-18"

// Exec runs one xo command line and returns its exit status.
type Exec func(args []string, stdout, stderr io.Writer) int

// Server answers MCP requests by running xo commands.
type Server struct {
	Exec    Exec
	Version string
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

// JSON-RPC error codes.
const (
	errParse          = -32700
	errInvalidRequest = -32600
	errMethodNotFound = -32601
	errInvalidParams  = -32602
)

// Serve reads newline delimited JSON-RPC messages from r and writes the
// responses to w until r ends.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	bw := bufio.NewWriter(w)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		resp := s.handle(line)
		if resp == nil {
			continue
		}
		data, err := json.Marshal(resp)
		if err != nil {
			return err
		}
		bw.Write(data)
		bw.WriteByte('\n')
		if err := bw.Flush(); err != nil {
			return err
		}
	}
	return sc.Err()
}

func (s *Server) handle(line []byte) *response {
	var req request
	if err := json.Unmarshal(line, &req); err != nil {
		return &response{JSONRPC: "2.0", ID: json.RawMessage("null"), Error: &rpcError{errParse, "parse error: " + err.Error()}}
	}
	isNotification := len(req.ID) == 0
	if req.JSONRPC != "2.0" || req.Method == "" {
		if isNotification {
			return nil
		}
		return &response{JSONRPC: "2.0", ID: req.ID, Error: &rpcError{errInvalidRequest, "invalid request"}}
	}
	result, rerr := s.dispatch(req)
	if isNotification {
		return nil
	}
	resp := &response{JSONRPC: "2.0", ID: req.ID}
	if rerr != nil {
		resp.Error = rerr
	} else {
		resp.Result = result
	}
	return resp
}

func (s *Server) dispatch(req request) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &p)
		v := s.Version
		if v == "" {
			v = "dev"
		}
		return map[string]any{
			"protocolVersion": ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]any{"name": "xo", "version": v},
			"instructions":    "Xo toolchain. Run check after every edit; diagnostics carry stable XOnnnn codes and machine applicable fixes. Use query for types, definitions, callers, effects, holes, and module APIs; explain for a code's long form.",
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": toolList}, nil
	case "tools/call":
		var p struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{errInvalidParams, err.Error()}
		}
		t, ok := tools[p.Name]
		if !ok {
			return nil, &rpcError{errInvalidParams, fmt.Sprintf("unknown tool %q", p.Name)}
		}
		args := map[string]any{}
		if len(p.Arguments) > 0 && string(p.Arguments) != "null" {
			if err := json.Unmarshal(p.Arguments, &args); err != nil {
				return nil, &rpcError{errInvalidParams, "arguments: " + err.Error()}
			}
		}
		out, err := t.run(s, args)
		if err != nil {
			return toolResult(map[string]string{"error": err.Error()}, true), nil
		}
		return toolResult(out, false), nil
	}
	if strings.HasPrefix(req.Method, "notifications/") {
		return nil, nil
	}
	return nil, &rpcError{errMethodNotFound, fmt.Sprintf("method %q not found", req.Method)}
}

// toolResult wraps v as compact JSON text content.
func toolResult(v any, isError bool) map[string]any {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": strings.TrimRight(b.String(), "\n")}},
		"isError": isError,
	}
}

// ---------------------------------------------------------------------------
// tools

type tool struct {
	def map[string]any
	run func(s *Server, args map[string]any) (any, error)
}

var (
	tools    = map[string]tool{}
	toolList []map[string]any
)

func register(name, desc string, props map[string]any, required []string, run func(*Server, map[string]any) (any, error)) {
	schema := map[string]any{"type": "object", "properties": props, "additionalProperties": false}
	if len(required) > 0 {
		schema["required"] = required
	}
	def := map[string]any{"name": name, "description": desc, "inputSchema": schema}
	tools[name] = tool{def: def, run: run}
	toolList = append(toolList, def)
}

var dirsProp = map[string]any{
	"type": "array", "items": map[string]any{"type": "string"},
	"description": "module directories (default [\".\"]); every directory holding .xo files is a module",
}

func init() {
	register("check", "Type check the modules under dirs. Returns {ok, errors, warnings, modules, diagnostics}; each diagnostic has code, severity, message, span, notes, and fixes (machine applicable edits). With fix=true (`xo check --fix`, decision 0096) every unambiguous machine fix is applied and the changed files reformatted first; the result then also has applied: [{file, line, rule, title}].",
		map[string]any{"dirs": dirsProp, "no_gc": noGCProp, "fix": map[string]any{"type": "boolean", "description": "apply the unambiguous machine fixes and reformat before reporting (default false: read only)"}}, nil, toolCheck)
	register("query", "Semantic query: type (arg file:line:col), def (arg name, Type.member, module.name), callers (arg module.fn), effects (arg module.fn, with the call chain per effect), holes (no arg), api (arg module import path or directory).",
		map[string]any{
			"kind": map[string]any{"type": "string", "enum": []string{"type", "def", "callers", "effects", "holes", "api"}},
			"arg":  map[string]any{"type": "string", "description": "the query argument; omitted for holes"},
			"dir":  map[string]any{"type": "string", "description": "directory checked (default .)"},
		}, []string{"kind"}, toolQuery)
	register("edit", "Semantic edit through resolved symbols (core 10.4), atomic across the modules: rename (args [name, new name]), add-field (args [Struct, \"name: Type = default\"]), change-sig (args [fn, \"(params) -> Result ! Err\"]). Rejected, with nothing written, unless the program checks clean before and after. Returns {op, target, ok, written, reason, files, edits, diagnostics, sites}.",
		map[string]any{
			"op":      map[string]any{"type": "string", "enum": []string{"rename", "add-field", "change-sig"}},
			"args":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "the operation's arguments"},
			"dir":     map[string]any{"type": "string", "description": "directory checked and edited (default .)"},
			"dry_run": map[string]any{"type": "boolean", "description": "check the edit without writing files"},
		}, []string{"op", "args"}, toolEdit)
	register("explain", "Long form explanation of a diagnostic code: title, summary, detail, example, fix.",
		map[string]any{"code": map[string]any{"type": "string", "description": "diagnostic code such as XO0201 (or 0201)"}},
		[]string{"code"}, toolExplain)
	register("fmt", "Canonical formatting. By default reports which files are unformatted without changing them; write=true rewrites them. Returns {changed, unchanged, written, diagnostics}.",
		map[string]any{
			"paths": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "files or directories (default [\".\"])"},
			"write": map[string]any{"type": "boolean", "description": "rewrite unformatted files (default false)"},
		}, nil, toolFmt)
	register("test", "Build and run the test blocks, property tests, and contract fuzzing. Returns {ok, passed, failed, skipped, failures: [{name, module, loc, message, seed}]}, plus build errors when the tests do not compile.",
		map[string]any{
			"dirs":  dirsProp,
			"run":   map[string]any{"type": "string", "description": "run only tests whose name contains this text"},
			"seed":  map[string]any{"type": "integer", "description": "scheduler and generator seed"},
			"holes": map[string]any{"type": "boolean", "description": "allow holes (???), which fault when evaluated"},
			"no_gc": noGCProp,
		}, nil, toolTest)
	register("build", "Compile to static binaries. Returns {ok, binaries: {os/arch: path}, diagnostics}.",
		map[string]any{
			"dirs":    dirsProp,
			"targets": map[string]any{"type": "string", "description": "comma separated os/arch list, or all (default: the host)"},
			"out":     map[string]any{"type": "string", "description": "output binary (one target) or directory (several)"},
			"no_gc":   noGCProp,
		}, nil, toolBuild)
}

var noGCProp = map[string]any{"type": "boolean", "description": "build without a garbage collector on a native backend (`--no-gc`, decision 0083): XO1201 diagnostics name what it cannot build, planned or impossible"}

// noGCFlag appends --no-gc to cl when args sets no_gc.
func noGCFlag(cl []string, args map[string]any) ([]string, error) {
	on, err := boolArg(args, "no_gc")
	if err != nil {
		return nil, err
	}
	if on {
		cl = append(cl, "--no-gc")
	}
	return cl, nil
}

// strArg returns a string argument.
func strArg(args map[string]any, name string) (string, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%s: want a string", name)
	}
	return s, nil
}

func strsArg(args map[string]any, name string, def []string) ([]string, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return def, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s: want an array of strings", name)
	}
	var out []string
	for _, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("%s: want an array of strings", name)
		}
		out = append(out, s)
	}
	if len(out) == 0 {
		return def, nil
	}
	return out, nil
}

func boolArg(args map[string]any, name string) (bool, error) {
	v, ok := args[name]
	if !ok || v == nil {
		return false, nil
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%s: want a boolean", name)
	}
	return b, nil
}

// exec runs an xo command line and returns its exit status and output.
func (s *Server) exec(args ...string) (int, []byte, string) {
	var out, errOut bytes.Buffer
	code := s.Exec(args, &out, &errOut)
	return code, out.Bytes(), errOut.String()
}

// usageErr turns an exit status 2 (usage or I/O error) into an error.
func usageErr(code int, stderr string) error {
	if code == 2 {
		msg := strings.TrimSpace(stderr)
		if msg == "" {
			msg = "xo exited with status 2"
		}
		return fmt.Errorf("%s", msg)
	}
	return nil
}

func toolCheck(s *Server, args map[string]any) (any, error) {
	dirs, err := strsArg(args, "dirs", []string{"."})
	if err != nil {
		return nil, err
	}
	cl, err := noGCFlag([]string{"check", "--json"}, args)
	if err != nil {
		return nil, err
	}
	doFix, err := boolArg(args, "fix")
	if err != nil {
		return nil, err
	}
	if doFix {
		cl = append(cl, "--fix")
	}
	code, out, stderr := s.exec(append(cl, dirs...)...)
	if err := usageErr(code, stderr); err != nil {
		return nil, err
	}
	var r struct {
		Modules     []string          `json:"modules"`
		Diagnostics []json.RawMessage `json:"diagnostics"`
		Applied     []json.RawMessage `json:"applied"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("check output: %v", err)
	}
	errs, warns := 0, 0
	for _, d := range r.Diagnostics {
		var sev struct {
			Severity string `json:"severity"`
		}
		_ = json.Unmarshal(d, &sev)
		if sev.Severity == "error" {
			errs++
		} else {
			warns++
		}
	}
	res := map[string]any{"ok": errs == 0, "errors": errs, "warnings": warns, "modules": r.Modules, "diagnostics": r.Diagnostics}
	if doFix {
		if r.Applied == nil {
			r.Applied = []json.RawMessage{}
		}
		res["applied"] = r.Applied
	}
	return res, nil
}

func toolQuery(s *Server, args map[string]any) (any, error) {
	kind, err := strArg(args, "kind")
	if err != nil {
		return nil, err
	}
	arg, err := strArg(args, "arg")
	if err != nil {
		return nil, err
	}
	dir, err := strArg(args, "dir")
	if err != nil {
		return nil, err
	}
	cl := []string{"query", kind, "--json"}
	if dir != "" {
		cl = append(cl, "--dir="+dir)
	}
	if arg != "" {
		cl = append(cl, arg)
	}
	code, out, stderr := s.exec(cl...)
	if code != 0 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	return json.RawMessage(bytes.TrimSpace(out)), nil
}

func toolEdit(s *Server, args map[string]any) (any, error) {
	op, err := strArg(args, "op")
	if err != nil {
		return nil, err
	}
	rest, err := strsArg(args, "args", nil)
	if err != nil {
		return nil, err
	}
	dir, err := strArg(args, "dir")
	if err != nil {
		return nil, err
	}
	dry, err := boolArg(args, "dry_run")
	if err != nil {
		return nil, err
	}
	cl := []string{"edit"}
	if dir != "" {
		cl = append(cl, "--dir="+dir)
	}
	if dry {
		cl = append(cl, "--dry-run")
	}
	cl = append(append(cl, op, "--"), rest...)
	code, out, stderr := s.exec(cl...)
	if code == 2 {
		return nil, usageErr(code, stderr)
	}
	return json.RawMessage(bytes.TrimSpace(out)), nil
}

func toolExplain(s *Server, args map[string]any) (any, error) {
	c, err := strArg(args, "code")
	if err != nil {
		return nil, err
	}
	code, out, stderr := s.exec("explain", "--json", c)
	if code != 0 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	return json.RawMessage(bytes.TrimSpace(out)), nil
}

func toolFmt(s *Server, args map[string]any) (any, error) {
	paths, err := strsArg(args, "paths", []string{"."})
	if err != nil {
		return nil, err
	}
	write, err := boolArg(args, "write")
	if err != nil {
		return nil, err
	}
	cl := []string{"fmt", "--json"}
	if !write {
		cl = append(cl, "--check")
	}
	code, out, stderr := s.exec(append(cl, paths...)...)
	if err := usageErr(code, stderr); err != nil {
		return nil, err
	}
	var r struct {
		Files []struct {
			Path    string `json:"path"`
			Changed bool   `json:"changed"`
		} `json:"files"`
		Diagnostics []json.RawMessage `json:"diagnostics"`
	}
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("fmt output: %v", err)
	}
	changed, unchanged := []string{}, 0
	for _, f := range r.Files {
		if f.Changed {
			changed = append(changed, f.Path)
		} else {
			unchanged++
		}
	}
	if r.Diagnostics == nil {
		r.Diagnostics = []json.RawMessage{}
	}
	return map[string]any{"changed": changed, "unchanged": unchanged, "written": write && len(changed) > 0, "diagnostics": r.Diagnostics}, nil
}

func toolTest(s *Server, args map[string]any) (any, error) {
	dirs, err := strsArg(args, "dirs", []string{"."})
	if err != nil {
		return nil, err
	}
	run, err := strArg(args, "run")
	if err != nil {
		return nil, err
	}
	holes, err := boolArg(args, "holes")
	if err != nil {
		return nil, err
	}
	cl, err := noGCFlag([]string{"test", "--json"}, args)
	if err != nil {
		return nil, err
	}
	if run != "" {
		cl = append(cl, "--run="+run)
	}
	if v, ok := args["seed"]; ok && v != nil {
		f, ok := v.(float64)
		if !ok {
			return nil, fmt.Errorf("seed: want an integer")
		}
		cl = append(cl, "--seed="+strconv.FormatInt(int64(f), 10))
	}
	if holes {
		cl = append(cl, "--holes")
	}
	code, out, stderr := s.exec(append(cl, dirs...)...)
	if err := usageErr(code, stderr); err != nil {
		return nil, err
	}
	type failure struct {
		Name    string `json:"name"`
		Module  string `json:"module,omitempty"`
		Loc     string `json:"loc,omitempty"`
		Message string `json:"message"`
		Seed    int64  `json:"seed,omitempty"`
	}
	passed, failed, skipped := 0, 0, 0
	failures := []failure{}
	var other []string
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var ev struct {
			failure
			Status string `json:"status"`
		}
		if !strings.HasPrefix(line, "{") || json.Unmarshal([]byte(line), &ev) != nil || ev.Status == "" {
			other = append(other, line)
			continue
		}
		switch ev.Status {
		case "pass":
			passed++
		case "fail":
			failed++
			failures = append(failures, ev.failure)
		default:
			skipped++
		}
	}
	res := map[string]any{"ok": code == 0, "passed": passed, "failed": failed, "skipped": skipped, "failures": failures}
	if len(other) > 0 {
		res["output"] = strings.Join(other, "\n")
	}
	if code != 0 && failed == 0 {
		// the tests did not build (or the binary crashed): the checker
		// diagnostics and build errors are on stderr in the human form
		res["errors"] = strings.TrimSpace(stderr)
	}
	return res, nil
}

func toolBuild(s *Server, args map[string]any) (any, error) {
	dirs, err := strsArg(args, "dirs", []string{"."})
	if err != nil {
		return nil, err
	}
	targets, err := strArg(args, "targets")
	if err != nil {
		return nil, err
	}
	out, err := strArg(args, "out")
	if err != nil {
		return nil, err
	}
	cl, err := noGCFlag([]string{"build", "--json"}, args)
	if err != nil {
		return nil, err
	}
	if targets != "" {
		cl = append(cl, "--targets="+targets)
	}
	if out != "" {
		cl = append(cl, "-o", out)
	}
	code, stdout, stderr := s.exec(append(cl, dirs...)...)
	if err := usageErr(code, stderr); err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(stdout)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s", strings.TrimSpace(stderr))
	}
	var r map[string]any
	if err := json.Unmarshal(trimmed, &r); err != nil {
		return nil, fmt.Errorf("build output: %v", err)
	}
	return r, nil
}
