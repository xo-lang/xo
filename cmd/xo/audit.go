package main

import (
	"flag"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/build"
	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/mod"
	"github.com/xo-lang/xo/internal/query"
)

// auditReport is the result of `xo audit` (decision 0156): what each
// dependency may do and what the consuming module grants it.
type auditReport struct {
	Module       string     `json:"module"`
	Dir          string     `json:"dir"`
	Dependencies []auditDep `json:"dependencies"`
	// Attenuations are the attenuation calls of the consuming modules
	// (`only`, `sub`, `read_only`, `write_only`, `cwd`).
	Attenuations []auditAttenuation `json:"attenuations"`
	// Capabilities are the capability arguments of calls into
	// dependencies.
	Capabilities  []auditCapability `json:"capabilities_passed"`
	UnusedVouches []auditVouch      `json:"unused_vouches"`
	Summary       auditSummary      `json:"summary"`
	Diagnostics   []diag.Diagnostic `json:"diagnostics"`
}

type auditDep struct {
	Path      string `json:"path"`
	Version   string `json:"version,omitempty"`
	Direct    bool   `json:"direct"`
	Workspace bool   `json:"workspace,omitempty"`
	// Effects are the effects the module's API may reach now; Locked
	// what xo.lock records (absent when it has no line).
	Effects []string  `json:"effects"`
	Locked  *[]string `json:"locked_effects,omitempty"`
	// Lock is ok, widened, narrowed, unlocked, or workspace.
	Lock    string        `json:"lock"`
	Added   []string      `json:"added_effects,omitempty"`
	Ceiling string        `json:"ceiling,omitempty"`
	Escapes []auditEscape `json:"escape_hatches"`
}

type auditEscape struct {
	Kind   string `json:"kind"` // c or go
	Target string `json:"target"`
	File   string `json:"file"`
	Line   int    `json:"line"`
	// Claim is the `uses` the dependency wrote on a `use c` line.
	Claim   *[]string `json:"claimed_effects,omitempty"`
	Vouched bool      `json:"vouched"`
	Vouch   string    `json:"vouch,omitempty"` // file:line of the vouch
	Effects []string  `json:"effects"`         // what the build charges
}

type auditAttenuation struct {
	File       string   `json:"file"`
	Line       int      `json:"line"`
	Capability string   `json:"capability"`
	Method     string   `json:"method"`
	Args       []string `json:"args,omitempty"`
	Constant   bool     `json:"constant"`
}

type auditCapability struct {
	File       string `json:"file"`
	Line       int    `json:"line"`
	Callee     string `json:"callee"`
	Dependency string `json:"dependency"`
	Capability string `json:"capability"`
	Attenuated bool   `json:"attenuated"`
	// Via is the attenuation the argument comes from, when it is one.
	Via string `json:"via,omitempty"`
}

type auditVouch struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type auditSummary struct {
	Dependencies     int `json:"dependencies"`
	Unvouched        int `json:"unvouched_escape_hatches"`
	Widened          int `json:"widened"`
	Unlocked         int `json:"unlocked"`
	UnattenuatedCaps int `json:"unattenuated_capabilities_passed"`
	Errors           int `json:"errors"`
}

// capabilityTypes are the capability types a call may hand a dependency.
var capabilityTypes = map[string]bool{"Os": true, "Net": true, "Fs": true, "Clock": true, "Rand": true, "Env": true, "Proc": true, "Stdio": true,
	"TestOs": true, "FakeNet": true, "MemFs": true, "FakeClock": true, "FakeEnv": true, "FakeStdio": true, "FakeProc": true}

// attenuators are the methods that narrow a capability.
var attenuators = map[string]bool{"only": true, "sub": true, "read_only": true, "write_only": true, "cwd": true}

// runAudit is `xo audit [--json] [--dir=<dir>] [--no-gc]`.
func runAudit(args []string, stdout, stderr io.Writer) int {
	fl := flag.NewFlagSet("audit", flag.ContinueOnError)
	fl.SetOutput(stderr)
	dir := fl.String("dir", ".", "a directory of the main module")
	asJSON := fl.Bool("json", false, "print the report as JSON")
	noGC := fl.Bool("no-gc", false, "check as a native build (needed for `use c` dependencies)")
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() > 0 {
		*dir = fl.Arg(0)
	}
	root, ok := modRootOf(*dir, "audit", stderr)
	if !ok {
		return 2
	}
	var res *check.Result
	var ds []diag.Diagnostic
	if *noGC {
		r, all, err := build.CheckNoGCDirs([]string{root}, true)
		if err != nil {
			fmt.Fprintf(stderr, "xo audit: %v\n", err)
			return 2
		}
		res, ds = r, all
	} else {
		sess, err := query.Load([]string{root})
		if err != nil {
			fmt.Fprintf(stderr, "xo audit: %v\n", err)
			return 2
		}
		res, ds = sess.Res, sess.Res.Diagnostics
	}
	rep := audit(root, res, ds)
	if *asJSON || !isTerminal(stdout) {
		encodeJSON(stdout, rep)
	} else {
		printAudit(rep, stdout)
	}
	if rep.Summary.Errors > 0 {
		return 1
	}
	return 0
}

// audit builds the report of the module at root from its check.
func audit(root string, res *check.Result, ds []diag.Diagnostic) *auditReport {
	rep := &auditReport{Dir: root, Dependencies: []auditDep{}, Attenuations: []auditAttenuation{}, Capabilities: []auditCapability{}, UnusedVouches: []auditVouch{}, Diagnostics: ds}
	if rep.Diagnostics == nil {
		rep.Diagnostics = []diag.Diagnostic{}
	}
	if f, _ := mod.ReadFile(root); f != nil {
		rep.Module = f.Path
	}
	for _, d := range ds {
		if d.Severity == diag.Error {
			rep.Summary.Errors++
		}
	}
	bl := res.Deps()
	ceilings := mod.Ceilings(root)
	hatches := res.EscapeHatches()
	used := map[string]bool{}
	if bl != nil {
		for _, m := range bl.Modules {
			d := auditDep{Path: m.Path, Version: m.Version, Direct: m.Direct, Workspace: m.Workspace, Effects: []string{}, Escapes: []auditEscape{}}
			if cur, ok := bl.Effects[m.Path]; ok {
				d.Effects = cur
			}
			switch locked, ok := bl.Locked[m.Path]; {
			case m.Workspace:
				d.Lock = "workspace"
			case !ok:
				d.Lock = "unlocked"
				rep.Summary.Unlocked++
			default:
				l := append([]string{}, locked...)
				d.Locked = &l
				d.Added = mod.AddedEffects(locked, d.Effects)
				switch {
				case len(d.Added) > 0:
					d.Lock = "widened"
					rep.Summary.Widened++
				case mod.EffectsText(locked) != mod.EffectsText(d.Effects):
					d.Lock = "narrowed"
				default:
					d.Lock = "ok"
				}
			}
			if srcs := ceilings[m.Path]; len(srcs) > 0 {
				d.Ceiling = srcs[0].Ceiling.String()
			}
			for _, h := range hatches {
				if h.Module != m.Path {
					continue
				}
				e := auditEscape{Kind: h.Kind, Target: h.Target, File: h.Span.File, Line: h.Span.Start.Line, Vouched: h.Vouch != nil, Effects: h.Effects}
				if h.Claim != nil {
					c := append([]string{}, h.Claim...)
					e.Claim = &c
				}
				if h.Vouch != nil {
					e.Vouch = fmt.Sprintf("%s:%d", h.Vouch.File, h.Vouch.Line)
					used[e.Vouch] = true
				} else {
					rep.Summary.Unvouched++
				}
				d.Escapes = append(d.Escapes, e)
			}
			rep.Dependencies = append(rep.Dependencies, d)
		}
	}
	rep.Summary.Dependencies = len(rep.Dependencies)
	for _, v := range res.ConsumerVouches() {
		if !used[fmt.Sprintf("%s:%d", v.File, v.Line)] {
			rep.UnusedVouches = append(rep.UnusedVouches, auditVouch{File: v.File, Line: v.Line, Text: v.String()})
		}
	}
	auditCalls(res, rep)
	for _, c := range rep.Capabilities {
		if !c.Attenuated {
			rep.Summary.UnattenuatedCaps++
		}
	}
	return rep
}

// auditCalls walks the consuming modules (not std, not dependencies) for
// attenuation calls and capability arguments of calls into dependencies.
func auditCalls(res *check.Result, rep *auditReport) {
	info := res.Info()
	for _, m := range info.AllModules() {
		if m.IsStd || m.Native || m.Dependency != "" {
			continue
		}
		for _, f := range m.Files {
			if f.IsTest || f.AST == nil {
				continue
			}
			// locals bound once to an expression: let x = <expr>
			bound := map[*check.VarObj]ast.Expr{}
			check.Inspect(f.AST, func(n ast.Node) bool {
				if l, ok := n.(*ast.LetStmt); ok {
					if p, ok := l.Pattern.(*ast.BindPat); ok && l.Value != nil {
						if v := info.VarOf(p.Name); v != nil {
							bound[v] = l.Value
						}
					}
				}
				return true
			})
			check.Inspect(f.AST, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				ci := info.CallOf(call)
				if ci == nil || ci.Fn == nil || ci.Fn.Module == nil {
					return true
				}
				fo := ci.Fn
				sp := call.Span()
				if a, ok := attenuation(fo, call); ok {
					a.File, a.Line = sp.File, sp.Start.Line
					rep.Attenuations = append(rep.Attenuations, a)
				}
				if fo.Module.Dependency == "" {
					return true
				}
				for _, arg := range call.Args {
					t := info.TypeOf(arg.Value)
					name := capName(t)
					if name == "" {
						continue
					}
					c := auditCapability{File: sp.File, Line: sp.Start.Line, Callee: fo.DisplayName(), Dependency: fo.Module.Dependency, Capability: name}
					if via, ok := attenuatedVia(info, arg.Value, bound); ok {
						c.Attenuated, c.Via = true, via
					}
					rep.Capabilities = append(rep.Capabilities, c)
				}
				return true
			})
		}
	}
	sort.SliceStable(rep.Attenuations, func(i, j int) bool {
		a, b := rep.Attenuations[i], rep.Attenuations[j]
		return a.File < b.File || a.File == b.File && a.Line < b.Line
	})
}

// capName is the capability type name of t, or "".
func capName(t check.Type) string {
	if t == nil {
		return ""
	}
	n, ok := check.Resolve(t).(*check.Named)
	if !ok || n.Obj == nil || n.Obj.Module == nil || !n.Obj.Module.IsStd {
		return ""
	}
	if capabilityTypes[n.Obj.Name] {
		return n.Obj.Name
	}
	return ""
}

// attenuation describes call when it narrows a capability.
func attenuation(fo *check.FuncObj, call *ast.CallExpr) (auditAttenuation, bool) {
	if fo.Recv == nil || !fo.Module.IsStd || !attenuators[fo.Name] || !capabilityTypes[fo.Recv.Name] {
		return auditAttenuation{}, false
	}
	a := auditAttenuation{Capability: fo.Recv.Name, Method: fo.Name, Constant: true}
	for _, arg := range call.Args {
		vals, ok := constStrings(arg.Value)
		if !ok {
			a.Constant = false
			a.Args = nil
			break
		}
		a.Args = append(a.Args, vals...)
	}
	return a, true
}

// constStrings reads a string literal without interpolation, or a list
// literal of them.
func constStrings(e ast.Expr) ([]string, bool) {
	switch x := e.(type) {
	case *ast.StringLit:
		var b strings.Builder
		for _, p := range x.Parts {
			if p.Expr != nil {
				return nil, false
			}
			b.WriteString(p.Text)
		}
		return []string{b.String()}, true
	case *ast.ListLit:
		out := []string{}
		for _, el := range x.Elems {
			v, ok := constStrings(el)
			if !ok {
				return nil, false
			}
			out = append(out, v...)
		}
		return out, true
	}
	return nil, false
}

// attenuatedVia reports whether e is an attenuation call, or a local
// bound once to one, and names it.
func attenuatedVia(info *check.Info, e ast.Expr, bound map[*check.VarObj]ast.Expr) (string, bool) {
	for depth := 0; depth < 5; depth++ {
		switch x := e.(type) {
		case *ast.CallExpr:
			ci := info.CallOf(x)
			if ci == nil || ci.Fn == nil {
				return "", false
			}
			a, ok := attenuation(ci.Fn, x)
			if !ok {
				return "", false
			}
			args := "..."
			if a.Constant {
				args = strings.Join(a.Args, ", ")
			}
			return fmt.Sprintf("%s.%s(%s)", a.Capability, a.Method, args), true
		case *ast.Ident:
			v := info.VarOf(x)
			if v == nil {
				return "", false
			}
			init, ok := bound[v]
			if !ok {
				return "", false
			}
			e = init
		default:
			return "", false
		}
	}
	return "", false
}

// printAudit prints the report for a terminal.
func printAudit(rep *auditReport, w io.Writer) {
	fmt.Fprintf(w, "xo audit %s (%s)\n", rep.Module, rep.Dir)
	if len(rep.Dependencies) == 0 {
		fmt.Fprintln(w, "no dependencies")
	}
	for _, d := range rep.Dependencies {
		v := d.Version
		if d.Workspace {
			v = "(workspace)"
		}
		kind := "indirect"
		if d.Direct {
			kind = "direct"
		}
		fmt.Fprintf(w, "\n%s %s [%s]\n", d.Path, v, kind)
		fmt.Fprintf(w, "  effects: %s\n", mod.EffectsText(d.Effects))
		lock := d.Lock
		if d.Locked != nil {
			lock += " (locked: " + mod.EffectsText(*d.Locked) + ")"
		}
		if len(d.Added) > 0 {
			lock += ", added: " + strings.Join(d.Added, ", ")
		}
		fmt.Fprintf(w, "  lock: %s\n", lock)
		if d.Ceiling != "" {
			fmt.Fprintf(w, "  ceiling: %s\n", d.Ceiling)
		}
		for _, e := range d.Escapes {
			st := "UNVOUCHED"
			if e.Vouched {
				st = "vouched at " + e.Vouch
			}
			fmt.Fprintf(w, "  use %s %q (%s:%d): %s, charged %s\n", e.Kind, e.Target, filepath.Base(e.File), e.Line, st, mod.EffectsText(e.Effects))
		}
	}
	if len(rep.Attenuations) > 0 {
		fmt.Fprintln(w, "\nattenuations:")
		for _, a := range rep.Attenuations {
			args := "(not constant)"
			if a.Constant {
				args = "[" + strings.Join(a.Args, ", ") + "]"
			}
			fmt.Fprintf(w, "  %s:%d %s.%s %s\n", a.File, a.Line, a.Capability, a.Method, args)
		}
	}
	if len(rep.Capabilities) > 0 {
		fmt.Fprintln(w, "\ncapabilities passed to dependencies:")
		for _, c := range rep.Capabilities {
			st := "UNATTENUATED"
			if c.Attenuated {
				st = c.Via
			}
			fmt.Fprintf(w, "  %s:%d %s -> %s (%s): %s\n", c.File, c.Line, c.Capability, c.Callee, c.Dependency, st)
		}
	}
	for _, v := range rep.UnusedVouches {
		fmt.Fprintf(w, "\nunused vouch %s:%d: %s\n", v.File, v.Line, v.Text)
	}
	s := rep.Summary
	fmt.Fprintf(w, "\n%d dependencies, %d unvouched escape hatches, %d widened, %d unlocked, %d unattenuated capabilities passed, %d errors\n",
		s.Dependencies, s.Unvouched, s.Widened, s.Unlocked, s.UnattenuatedCaps, s.Errors)
	if s.Errors > 0 {
		printDiags(rep.Diagnostics, w)
	}
}
