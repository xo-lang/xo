package diag

// Code is a stable diagnostic code (core 10.2). Never change the meaning of
// an existing code; add a new one.
type Code string

// Codes defined by the spec (core.md). The parser does not emit these; they
// are listed so this file is the single registry of every code.
const (
	NamingConvention   Code = "XO0101" // name breaks the enforced naming rules (core 1.5)
	UnusedBinding      Code = "XO0110" // unused local binding or import (core 3.1)
	UnhandledResult    Code = "XO0201" // Result value is never used (core 2.6)
	ResultNeedsOk      Code = "XO0203" // success of a Result typed T must be written Ok(x) (core 2.6)
	OptionInResultFn   Code = "XO0204" // `?` on an Option in a function returning a Result (core 2.5)
	NonExhaustiveMatch Code = "XO0301" // match does not cover every case (core 2.3)
	AmbiguousTypeArgs  Code = "XO0402" // type arguments cannot be inferred (core 2.7)
	VarReceiverNeedVar Code = "XO0501" // var method called on a non var binding (core 3.4)
	UseOfMovedValue    Code = "XO0510" // use of a moved Buf, StrBuilder, or BytesBuilder (core 5.3)
	LostUpdate         Code = "XO0520" // mutation of a copied or stored collection element is never written back (core 5.2)
	UnresolvedName     Code = "XO0601" // name does not resolve (core 4.3)
	ImportNeedsAlias   Code = "XO0602" // import path does not yield a valid local name (core 4.1)
	UndeclaredEffect   Code = "XO0701" // effect performed outside the declared set (core 6.2)
	ScopeErrorConvert  Code = "XO0801" // child or group task error does not convert to the enclosing error type (core 7.1)
	Hole               Code = "XO0900" // typed hole report (core 3.7)
	PlatformAPIOutside Code = "XO1001" // std/sys/<os> used outside the matching target arm (core 12.2)
)

// Lexical and syntax errors (01xx group, 0150 onward). Emitted by
// internal/lexer and internal/parser.
const (
	InvalidChar          Code = "XO0150" // character that cannot start any token
	UnterminatedString   Code = "XO0151" // string, raw string, bytes, or triple quoted string not closed
	InvalidRune          Code = "XO0152" // rune literal that is empty, unterminated, or holds more than one character
	InvalidEscape        Code = "XO0153" // unknown or malformed escape sequence
	MalformedNumber      Code = "XO0154" // bad digit, misplaced `_`, missing digits, or out of range integer
	InvalidDuration      Code = "XO0155" // unknown duration unit, fractional or non decimal duration
	UnterminatedInterp   Code = "XO0156" // `${` without a matching `}` in the same string
	EmptyInterp          Code = "XO0157" // `${}` with no expression
	MalformedImportPath  Code = "XO0158" // `use` not followed by a valid import path
	UnexpectedToken      Code = "XO0160" // a token that does not fit the grammar here
	MissingComma         Code = "XO0161" // list element followed by a newline instead of a comma (core 1.3)
	ElseOnNewLine        Code = "XO0162" // `else` must be on the line of the closing brace (core 1.3)
	StructLitInCondition Code = "XO0163" // unparenthesized struct literal in a condition (core 1.4)
	UnclosedDelimiter    Code = "XO0164" // `(`, `[`, or `{` not closed before end of file
	InvalidPattern       Code = "XO0165" // syntax that is not a pattern (core 3.5)
	WhileCondAsExpr      Code = "XO0166" // `while cond` used as an expression; only `while { }` is one
	DeclInBlock          Code = "XO0167" // top level declaration (fn, struct, const, ...) inside a block
	StmtAtTopLevel       Code = "XO0168" // statement (let, expression, ...) at top level
	MatchArmSeparator    Code = "XO0169" // match or select arms separated by `,` instead of newlines
	FnTypeEffectsParens  Code = "XO0170" // several effects in a function type must be parenthesized (core 15)
	MisplacedRest        Code = "XO0171" // `..` used more than once or not last in pattern fields
	MissingBody          Code = "XO0172" // function declaration without a body
	UseAfterDecl         Code = "XO0173" // `use` after the first declaration (core 15: File)
	MisplacedSelf        Code = "XO0174" // bare `self` parameter outside an interface method signature
	ExpectedNewline      Code = "XO0175" // two statements or arms on one line
	MisplacedPub         Code = "XO0176" // `pub` before something that cannot be exported
	ForeignSyntax        Code = "XO0177" // syntax from another language (Go, Rust, TypeScript, Python, Swift, Kotlin); names the habit and the Xo form (decision 0087)
	LeadingOperator      Code = "XO0178" // a line starts with a binary operator; end the previous line with it (core 1.3, decision 0136)
)

// Checker codes (internal/check), chosen inside the spec's groups (core 10.2).
const (
	ErrorNoConvert       Code = "XO0202" // error type does not convert to the enclosing error type (core 2.6)
	TryNeedsFailing      Code = "XO0205" // `?` where the enclosing function cannot return the failure (core 2.5, 2.6)
	DiscardNeedsReason   Code = "XO0206" // `let _ =` discards a Result; use `.ignore_err("reason")` (core 2.6)
	PatternTypeMismatch  Code = "XO0302" // pattern does not fit the type of the matched value (core 3.5)
	RefutablePattern     Code = "XO0303" // refutable pattern where an irrefutable one is required (core 3.5)
	OrPatternBindings    Code = "XO0304" // alternatives of an or pattern bind different names or types (core 3.5)
	WildcardOwnEnum      Code = "XO0305" // warning: `_` arm hides variants of an enum declared in this module (core 3.5)
	TypeMismatch         Code = "XO0401" // value of the wrong type; only three implicit conversions exist (core 2.10)
	BadArguments         Code = "XO0403" // wrong arguments or fields: count, names, missing, or duplicates
	NotImplemented       Code = "XO0404" // type does not implement a required interface or constraint (core 2.4)
	CannotInfer          Code = "XO0405" // type cannot be inferred here; annotate it
	BadOperand           Code = "XO0406" // operator, index, slice, or iteration not defined for the type
	WrongKind            Code = "XO0407" // name used as the wrong kind of thing (a type as a value, a value that is not callable)
	ConstNotConstant     Code = "XO0408" // const initializer is not a compile time value (core 3.1)
	BadConversion        Code = "XO0409" // invalid explicit conversion (core 2.1)
	MapIndexRead         Code = "XO0410" // reading a Map by index; use `get` or `get_or_fault` (core 3.6)
	AssignImmutable      Code = "XO0502" // assignment to an immutable binding or to a captured variable (core 5.2, 3.3)
	VarArgsOverlap       Code = "XO0503" // one variable or overlapping places passed to two `var` parameters of one call (core 5.2, decision 0111)
	Duplicate            Code = "XO0603" // name declared twice in one scope, or a predeclared name redefined (core 3.1, 1.4)
	ImportCycle          Code = "XO0604" // cyclic module imports (core 4.1)
	NotVisible           Code = "XO0605" // name is not `pub` in its module, or the std type is an opaque handle (core 1.5)
	ForeignMethod        Code = "XO0606" // method declared outside the module of its type (core 3.4)
	BadJump              Code = "XO0607" // `break` or `continue` outside a loop, or an unknown label (core 3.5)
	UnknownEffect        Code = "XO0702" // unknown effect label (core 6.1)
	ScopeEscape          Code = "XO0802" // scope handle, or a value containing a task group, escapes its scope (core 7.1)
	SpawnWithoutScope    Code = "XO0803" // task started without a scope handle (core 7.1)
	SplitCriticalSection Code = "XO0804" // warning: value read in one `Mutex.with` is written back in a later one (core 5.4)
)

// Go interop codes (11xx group, decision 0041, prototype 2026-10-08).
// Emitted by internal/check for `use go "<path>"` imports.
const (
	GoBindFailed  Code = "XO1101" // Go package cannot be bound (not found, does not build, main package, or binding error)
	GoNameSkipped Code = "XO1102" // Go name exists but its signature does not map to Xo, so the binding skipped it
)

// C interop codes (decision 0116). Emitted by internal/check for `use c
// "<header>"` imports.
const (
	CBindFailed  Code = "XO1104" // C header cannot be bound (not found, clang cannot read it, a `link` file is missing, or an unknown effect in its `uses`)
	CNameSkipped Code = "XO1105" // C function exists in the header but its signature does not map to Xo, so the binding skipped it
)

// Native builds without a garbage collector (12xx group, decision 0083).
// Emitted by internal/ssa's no GC check for `--no-gc` builds.
const (
	NoGCUnavailable Code = "XO1201" // construct, std API, or effect not available without a garbage collector (`--no-gc`); planned or impossible
)

// Build options (15xx group, decision 0165). Emitted by internal/build.
const (
	StampTarget Code = "XO1501" // `--stamp NAME=VALUE` names no `const NAME: Str = "..."` (a plain string literal) of the main module
	GoTooOld    Code = "XO1502" // the go command the Go backend runs is older than the oldest Go Xo supports (decision 0176)
)

// Real-time functions (14xx group, decision 0133). Emitted by the checker
// for functions declared `uses realtime` (core 6.5) and for loop bounds.
const (
	RealtimeAlloc     Code = "XO1401" // allocation in a real-time function (literal, interpolation, `+` on Str or List, closure, growth method)
	RealtimeLoop      Code = "XO1402" // loop without a provable bound in a real-time function, or a `bound N` that is not a constant or is exceeded by a constant range
	RealtimeCall      Code = "XO1403" // call in a real-time function of a function that is not real-time, a function value, or an interface method
	RealtimeRecursion Code = "XO1404" // recursion among real-time functions
	RealtimeBlocking  Code = "XO1405" // blocking operation (channel, Mutex, scope, select, task) or capability effect in a real-time function
)

// Module and dependency codes (13xx group, decision 0086). Emitted by
// internal/mod through the checker (on the xo.mod, xo.lock, or xo.work
// line), and by `xo get`, `xo mod tidy`, `xo vendor`, and `xo build`.
const (
	ModFileInvalid     Code = "XO1301" // malformed xo.mod require or deny line, xo.lock, or xo.work line, or a deny rule naming a package that does not exist (core 4.2, 4.3)
	ModuleFetch        Code = "XO1302" // module version cannot be fetched from git or a module proxy (XO_PROXY): repository, tag, or version missing, or its xo.mod declares another path
	ModuleOffline      Code = "XO1303" // module version is not in the module cache and the build is offline (XO_OFFLINE, --offline)
	ChecksumMismatch   Code = "XO1304" // module tree does not match its xo.lock content hash (core 4.3)
	LockMissing        Code = "XO1305" // xo.lock has no hash for a module version the build uses; run `xo mod tidy`
	ModuleNotRequired  Code = "XO1306" // import of a package from a module that xo.mod does not require
	VendorInconsistent Code = "XO1307" // vendor/ does not match xo.mod; run `xo vendor`
	WorkspaceRelease   Code = "XO1308" // release build (`xo build --release`, or `--ci`) while an xo.work workspace overrides required modules
	EffectCeiling      Code = "XO1309" // call into a dependency performs an effect above its ceiling (`require ... uses ...` in xo.mod, `use ... uses ...` in xo.work)
	BreakingRelease    Code = "XO1310" // `xo publish`: a minor or patch release removes or changes an export, or adds an effect to one
	InvalidRelease     Code = "XO1311" // `xo publish`: the release cannot be made (not in a git repository, a subdirectory module path not ending in its directory, uncommitted changes, tag exists, version not above the last, wrong /vN path)
	ImportDenied       Code = "XO1312" // `use` of a package that a `deny` rule of xo.mod forbids (decision 0095)
	ChecksumLog        Code = "XO1313" // a fetched module version disagrees with the checksum log (XO_SUMDB), or the log's signature or proofs do not verify (decision 0124)
	FetchSettings      Code = "XO1314" // the checksum log cannot be consulted, or XO_PROXY, XO_SUMDB, XO_NOPROXY, or XO_NOSUMDB is malformed (decisions 0124, 0126)
	EditionAlias       Code = "XO1315" // note: `edition 2026` in xo.mod (or `xo 2026` in xo.work) is the old name of `edition preview`; the fix rewrites it (core 13, decision 0155)
	UnvouchedEscape    Code = "XO1316" // `use c` or `use go` of an impure Go package in a required module without the consumer's `vouch` for that version (decision 0156)
	EffectsWidened     Code = "XO1317" // a dependency's effects are wider than its `effects` line in xo.lock (decision 0156)
)

// Vet codes (core 10.1 `xo vet`, 2026-10-08). Emitted only by
// internal/vet, never by `xo check`: advisory warnings and review notes
// that never fail a build.
const (
	CoalesceChainTrap  Code = "XO0207" // warning: method chain on the right of `??` starts at a literal, so it applies only to the default (core 15 precedence)
	IgnoredErrorReview Code = "XO0208" // note: `.ignore_err("reason")` discards an error; listed with its reason for review (core 2.6)
	UnusedEffect       Code = "XO0703" // warning: effect declared in `uses` is never performed (core 6.2)
	DebugLeft          Code = "XO0901" // warning: `dbg(...)` left in code (core 11.4)
	GoUsageReport      Code = "XO1103" // note: per module report of `use go` packages and the functions that perform `go` (decision 0041)
)

// std/time layout code (decision 0094). Emitted by internal/check for a
// constant layout passed to time.format, time.parse, or time.check_layout.
const (
	TimeLayout Code = "XO0420" // constant std/time layout is invalid: unknown field, bad field form, or stray brace (std.md 13.3)
)

// Descriptions maps every code to its one line description. Kept in sync
// with the constants above by TestCodesDocumented.
var Descriptions = map[Code]string{
	WildcardOwnEnum:      "warning: `_` arm hides variants of an enum declared in this module (core 3.5)",
	MapIndexRead:         "reading a Map by index; use `get` or `get_or_fault` (core 3.6)",
	SplitCriticalSection: "warning: value read in one `Mutex.with` is written back in a later one (core 5.4)",
	NamingConvention:     "name breaks the enforced naming rules (core 1.5)",
	UnusedBinding:        "unused local binding or import (core 3.1)",
	UnhandledResult:      "Result value is never used (core 2.6)",
	ResultNeedsOk:        "success of a Result typed T must be written Ok(x) (core 2.6)",
	OptionInResultFn:     "`?` on an Option in a function returning a Result (core 2.5)",
	NonExhaustiveMatch:   "match does not cover every case (core 2.3)",
	AmbiguousTypeArgs:    "type arguments cannot be inferred (core 2.7)",
	VarReceiverNeedVar:   "var method called on a non var binding (core 3.4)",
	UseOfMovedValue:      "use of a moved Buf, StrBuilder, or BytesBuilder (core 5.3)",
	LostUpdate:           "mutation of a copied or stored collection element is never written back (core 5.2)",
	UnresolvedName:       "name does not resolve (core 4.3)",
	ImportNeedsAlias:     "import path does not yield a valid local name (core 4.1)",
	UndeclaredEffect:     "effect performed outside the declared set (core 6.2)",
	ScopeErrorConvert:    "child or group task error does not convert to the enclosing error type (core 7.1)",
	Hole:                 "typed hole report (core 3.7)",
	PlatformAPIOutside:   "std/sys/<os> used outside the matching target arm (core 12.2)",

	ErrorNoConvert:      "error type does not convert to the enclosing error type (core 2.6)",
	TryNeedsFailing:     "`?` where the enclosing function cannot return the failure (core 2.5, 2.6)",
	DiscardNeedsReason:  "`let _ =` discards a Result; use `.ignore_err(\"reason\")` (core 2.6)",
	PatternTypeMismatch: "pattern does not fit the type of the matched value (core 3.5)",
	RefutablePattern:    "refutable pattern where an irrefutable one is required (core 3.5)",
	OrPatternBindings:   "alternatives of an or pattern bind different names or types (core 3.5)",
	TypeMismatch:        "value of the wrong type; only three implicit conversions exist (core 2.10)",
	BadArguments:        "wrong arguments or fields: count, names, missing, or duplicates",
	NotImplemented:      "type does not implement a required interface or constraint (core 2.4)",
	CannotInfer:         "type cannot be inferred here; annotate it",
	BadOperand:          "operator, index, slice, or iteration not defined for the type",
	WrongKind:           "name used as the wrong kind of thing (a type as a value, a value that is not callable)",
	ConstNotConstant:    "const initializer is not a compile time value (core 3.1)",
	BadConversion:       "invalid explicit conversion (core 2.1)",
	AssignImmutable:     "assignment to an immutable binding or to a captured variable (core 5.2, 3.3)",
	VarArgsOverlap:      "one variable or overlapping places passed to two `var` parameters of one call (core 5.2, decision 0111)",
	Duplicate:           "name declared twice in one scope, or a predeclared name redefined (core 3.1, 1.4)",
	ImportCycle:         "cyclic module imports (core 4.1)",
	NotVisible:          "name is not `pub` in its module, or the std type is an opaque handle (core 1.5)",
	ForeignMethod:       "method declared outside the module of its type (core 3.4)",
	BadJump:             "`break` or `continue` outside a loop, or an unknown label (core 3.5)",
	UnknownEffect:       "unknown effect label (core 6.1)",
	ScopeEscape:         "scope handle, or a value containing a task group, escapes its scope (core 7.1)",
	SpawnWithoutScope:   "task started without a scope handle (core 7.1)",

	CoalesceChainTrap:  "warning: method chain on the right of `??` starts at a literal, so it applies only to the default (core 15 precedence)",
	IgnoredErrorReview: "note: `.ignore_err(\"reason\")` discards an error; listed with its reason for review (core 2.6)",
	UnusedEffect:       "warning: effect declared in `uses` is never performed (core 6.2)",
	DebugLeft:          "warning: `dbg(...)` left in code (core 11.4)",
	GoUsageReport:      "note: per module report of `use go` packages and the functions that perform `go` (decision 0041)",

	GoBindFailed:  "Go package cannot be bound (not found, does not build, main package, or binding error)",
	GoNameSkipped: "Go name exists but its signature does not map to Xo, so the binding skipped it",

	CBindFailed:  "C header cannot be bound (not found, clang cannot read it, a `link` file is missing, or an unknown effect in its `uses`)",
	CNameSkipped: "C function exists in the header but its signature does not map to Xo, so the binding skipped it",

	ModFileInvalid:     "malformed xo.mod require or deny line, xo.lock, or xo.work line, or a deny rule naming a package that does not exist (core 4.2, 4.3)",
	ModuleFetch:        "module version cannot be fetched from git or a module proxy (XO_PROXY): repository, tag, or version missing, or its xo.mod declares another path",
	ModuleOffline:      "module version is not in the module cache and the build is offline (XO_OFFLINE, --offline)",
	ChecksumMismatch:   "module tree does not match its xo.lock content hash (core 4.3)",
	LockMissing:        "xo.lock has no hash for a module version the build uses; run `xo mod tidy`",
	ModuleNotRequired:  "import of a package from a module that xo.mod does not require",
	VendorInconsistent: "vendor/ does not match xo.mod; run `xo vendor`",
	WorkspaceRelease:   "release build (`xo build --release`, or `--ci`) while an xo.work workspace overrides required modules",
	EffectCeiling:      "call into a dependency performs an effect above its ceiling (`require ... uses ...` in xo.mod, `use ... uses ...` in xo.work)",
	BreakingRelease:    "`xo publish`: a minor or patch release removes or changes an export, or adds an effect to one",
	InvalidRelease:     "`xo publish`: the release cannot be made (not in a git repository, a subdirectory module path not ending in its directory, uncommitted changes, tag exists, version not above the last, wrong /vN path)",
	ImportDenied:       "`use` of a package that a `deny` rule of xo.mod forbids (decision 0095)",
	ChecksumLog:        "a fetched module version disagrees with the checksum log (XO_SUMDB), or the log's signature or proofs do not verify (decision 0124)",
	FetchSettings:      "the checksum log cannot be consulted, or XO_PROXY, XO_SUMDB, XO_NOPROXY, or XO_NOSUMDB is malformed (decisions 0124, 0126)",
	EditionAlias:       "note: `edition 2026` in xo.mod (or `xo 2026` in xo.work) is the old name of `edition preview`; the fix rewrites it (core 13, decision 0155)",
	UnvouchedEscape:    "`use c` or `use go` of an impure Go package in a required module without the consumer's `vouch` for that version (decision 0156)",
	EffectsWidened:     "a dependency's effects are wider than its `effects` line in xo.lock (decision 0156)",

	NoGCUnavailable: "construct, std API, or effect not available without a garbage collector (`--no-gc`); planned or impossible",

	RealtimeAlloc:     "allocation in a real-time function (literal, interpolation, `+` on Str or List, closure, growth method)",
	RealtimeLoop:      "loop without a provable bound in a real-time function, or a `bound N` that is not a constant or is exceeded by a constant range",
	RealtimeCall:      "call in a real-time function of a function that is not real-time, a function value, or an interface method",
	RealtimeRecursion: "recursion among real-time functions",
	RealtimeBlocking:  "blocking operation (channel, Mutex, scope, select, task) or capability effect in a real-time function",
	StampTarget:       "`--stamp NAME=VALUE` names no `const NAME: Str = \"...\"` (a plain string literal) of the main module",
	GoTooOld:          "the go command the Go backend runs is older than the oldest Go Xo supports (decision 0176)",

	InvalidChar:          "character that cannot start any token",
	UnterminatedString:   "string, raw string, bytes, or triple quoted string not closed",
	InvalidRune:          "rune literal that is empty, unterminated, or holds more than one character",
	InvalidEscape:        "unknown or malformed escape sequence",
	MalformedNumber:      "bad digit, misplaced `_`, missing digits, or out of range integer",
	InvalidDuration:      "unknown duration unit, fractional or non decimal duration",
	UnterminatedInterp:   "`${` without a matching `}` in the same string",
	EmptyInterp:          "`${}` with no expression",
	MalformedImportPath:  "`use` not followed by a valid import path",
	UnexpectedToken:      "a token that does not fit the grammar here",
	MissingComma:         "list element followed by a newline instead of a comma (core 1.3)",
	ElseOnNewLine:        "`else` must be on the line of the closing brace (core 1.3)",
	StructLitInCondition: "unparenthesized struct literal in a condition (core 1.4)",
	UnclosedDelimiter:    "`(`, `[`, or `{` not closed before end of file",
	InvalidPattern:       "syntax that is not a pattern (core 3.5)",
	WhileCondAsExpr:      "`while cond` used as an expression; only `while { }` is one",
	DeclInBlock:          "top level declaration (fn, struct, const, ...) inside a block",
	StmtAtTopLevel:       "statement (let, expression, ...) at top level",
	MatchArmSeparator:    "match or select arms separated by `,` instead of newlines",
	FnTypeEffectsParens:  "several effects in a function type must be parenthesized (core 15)",
	MisplacedRest:        "`..` used more than once or not last in pattern fields",
	MissingBody:          "function declaration without a body",
	UseAfterDecl:         "`use` after the first declaration (core 15: File)",
	MisplacedSelf:        "bare `self` parameter outside an interface method signature",
	ExpectedNewline:      "two statements or arms on one line",
	MisplacedPub:         "`pub` before something that cannot be exported",
	ForeignSyntax:        "syntax from another language (Go, Rust, TypeScript, Python, Swift, Kotlin); names the habit and the Xo form (decision 0087)",
	LeadingOperator:      "a line starts with a binary operator (`||`, `&&`, `+`, `==`, ...); end the previous line with it (core 1.3, decision 0136)",
	TimeLayout:           "constant std/time layout is invalid: unknown field, bad field form, or stray brace (std.md 13.3)",
}
