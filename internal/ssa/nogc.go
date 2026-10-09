package ssa

import (
	"errors"

	"github.com/xo-lang/xo/internal/check"
	"github.com/xo-lang/xo/internal/diag"
	"github.com/xo-lang/xo/internal/token"
)

// The check of `--no-gc` builds (decision 0083). A native build runs it
// after the checker and before any code generation: every construct,
// std API, or effect the native backends do not support is reported as
// an XO1201 diagnostic at its site, naming the feature and saying
// whether support is planned or impossible.
//
// The check is the builder itself: every place the builder cannot lower
// something records it with unsupported (planned) or impossible (never,
// such as a call into a `use go` package), so the check and the code
// generator cannot disagree about the builder's subset. The backends'
// own lowering (internal/llvm, internal/arm64) accepts every module the
// builder produces; TestNoGCCheckAgrees in internal/build holds them to
// it on every golden program.

// CheckNoGC lowers the program as a native build would and returns the
// XO1201 diagnostics of everything that could not be lowered (none when
// the program builds natively).
func CheckNoGC(res *check.Result, opts Options) []diag.Diagnostic {
	_, err := BuildOpts(res, opts)
	return NoGCDiagnostics(err)
}

// NoGCDiagnostics converts an error of Build into XO1201 diagnostics.
// An error that is not the builder's (none is expected) becomes one
// diagnostic without a position.
func NoGCDiagnostics(err error) []diag.Diagnostic {
	if err == nil {
		return nil
	}
	var es Errors
	if !errors.As(err, &es) {
		var e *Error
		if errors.As(err, &e) {
			es = Errors{e}
		} else {
			es = Errors{{Msg: err.Error(), Feature: err.Error()}}
		}
	}
	var out []diag.Diagnostic
	var kept []*Error
	for _, e := range es {
		if cascades(e, kept) {
			continue
		}
		kept = append(kept, e)
		out = append(out, NoGCDiagnostic(e))
	}
	diag.Sort(out)
	return out
}

// cascades reports whether e overlaps the source range of an error
// recorded before it: the builder goes on after an unsupported
// construct with a placeholder value, and what it then fails to lower
// around that value (a conversion of the placeholder, say) is a
// consequence, not a second problem (core 10.2: never cascade).
func cascades(e *Error, kept []*Error) bool {
	if e.Span.File == "" {
		return false
	}
	before := func(a, b token.Pos) bool {
		return a.Line < b.Line || (a.Line == b.Line && a.Col < b.Col)
	}
	for _, k := range kept {
		if k.Span.File != e.Span.File {
			continue
		}
		// overlap: neither ends before the other starts
		if before(e.Span.End, k.Span.Start) || before(k.Span.End, e.Span.Start) {
			continue
		}
		if e.Span.End == k.Span.Start || k.Span.End == e.Span.Start {
			continue
		}
		return true
	}
	return false
}

// NoGCDiagnostic is the XO1201 diagnostic of one unsupported construct.
func NoGCDiagnostic(e *Error) diag.Diagnostic {
	feature := e.Feature
	if feature == "" {
		feature = e.Msg
	}
	d := diag.Diagnostic{
		Code:     string(diag.NoGCUnavailable),
		Severity: diag.Error,
		Span:     diag.FromSpan(e.Span),
	}
	if e.Impossible {
		d.Message = "not available without a garbage collector (`--no-gc`): " + feature + " (impossible: Go code needs Go's runtime)"
		d.Notes = []diag.NoteItem{{Message: "support: impossible; build without --no-gc to use the Go backend"}}
	} else {
		d.Message = "not available without a garbage collector (`--no-gc`) yet: " + feature + " (support is planned)"
		d.Notes = []diag.NoteItem{{Message: "support: planned; build without --no-gc to use the Go backend until then"}}
	}
	return d
}
