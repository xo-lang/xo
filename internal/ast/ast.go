// Package ast declares the syntax tree of xo (core 15). Every node carries
// the span of the source it was parsed from.
package ast

import "github.com/xo-lang/xo/internal/token"

// Node is any syntax tree node.
type Node interface {
	Span() token.Span
}

// Base holds a node's span. It is embedded in every node.
type Base struct {
	Sp token.Span
}

// Span returns the node's source range.
func (b Base) Span() token.Span { return b.Sp }

// Expr is an expression node.
type Expr interface {
	Node
	exprNode()
}

// Stmt is a statement node.
type Stmt interface {
	Node
	stmtNode()
}

// Decl is a top level declaration.
type Decl interface {
	Node
	declNode()
}

// Type is a type expression.
type Type interface {
	Node
	typeNode()
}

// Pattern is a pattern (core 3.5).
type Pattern interface {
	Node
	patternNode()
}

// Ident is a name.
type Ident struct {
	Base
	Name string
}

// Doc is a `///` doc comment attached to a declaration, field, or variant.
type Doc struct {
	Base
	Text string // lines joined with "\n", leading "/// " removed
}

// ---------------------------------------------------------------------------
// File and declarations

// File is one parsed source file.
type File struct {
	Base
	Name  string
	Uses  []*UseDecl
	Decls []Decl
}

// UseDecl is `use path [as name]`, `use go "<Go import path>" [as
// name]` (Go is set; decision 0041), or `use c "<header>" [link "<lib>"]...
// [as name] [uses effects | uses none]` (C is set; decision 0113).
type UseDecl struct {
	Base
	Path  string
	Alias *Ident
	Go    bool
	C     bool
	// Links are the `link` clauses of `use c`: a C source, object, or
	// archive path relative to the module directory, or a library name
	// (`z` links -lz).
	Links []string
	// Uses is the effect ceiling a `use c` declares for every call into
	// the header (`uses none`: Uses non nil and empty); nil means the
	// default effect `ffi`.
	Uses []*Effect
}

// FnDecl is a function or method declaration. In interface bodies and in
// signature mode Body is nil.
type FnDecl struct {
	Base
	Doc        *Doc
	Pub        bool
	Recv       *Receiver
	Name       *Ident
	TypeParams []*TypeParam
	Params     []*Param
	Result     Type // nil when omitted
	Err        *ErrSpec
	Clauses    []Clause
	Body       *Block
}

// Receiver is the `(var name: Type)` before a method name.
type Receiver struct {
	Base
	Var  bool
	Name *Ident
	Type Type
}

// TypeParam is `name`, `name: Constraint`, or `name: Effects`.
type TypeParam struct {
	Base
	Name       *Ident
	Constraint Type // nil when unconstrained or an effect variable
	Effects    bool // declared with the `Effects` kind (core 6.2)
}

// Param is a function, closure, or test parameter. Type is nil for closure
// parameters whose type is inferred and for a bare `self`.
type Param struct {
	Base
	Var     bool
	Self    bool // bare `self` in an interface method signature
	Name    *Ident
	Type    Type
	Default Expr
}

// ErrSpec is `! [Type]`. A nil Type means the builtin `Error`.
type ErrSpec struct {
	Base
	Type Type
}

// Clause is a function clause: uses, requires, or ensures.
type Clause interface {
	Node
	clauseNode()
}

// Effect is an effect label such as `net` or `fs.read`, or an effect
// variable.
type Effect struct {
	Base
	Name *Ident
	Sub  *Ident // nil unless written `name.sub`
}

// UsesClause is `uses e1, e2` or `uses (e1, e2)`.
type UsesClause struct {
	Base
	Effects []*Effect
	Parens  bool
}

// RequiresClause is `requires[(debug)] cond [else err]`.
type RequiresClause struct {
	Base
	Debug bool
	Cond  Expr
	Else  Expr
}

// EnsuresClause is `ensures[(debug)] cond`.
type EnsuresClause struct {
	Base
	Debug bool
	Cond  Expr
}

// StructDecl is a struct declaration.
type StructDecl struct {
	Base
	Doc        *Doc
	Pub        bool
	Name       *Ident
	TypeParams []*TypeParam
	Fields     []*Field
	Invariant  Expr
	Derive     []*Ident
}

// Field is a struct field.
type Field struct {
	Base
	Doc     *Doc
	Pub     bool
	Name    *Ident
	Type    Type
	Default Expr
}

// EnumDecl is an enum declaration.
type EnumDecl struct {
	Base
	Doc        *Doc
	Pub        bool
	Name       *Ident
	TypeParams []*TypeParam
	Variants   []*Variant
	Derive     []*Ident
}

// Variant is an enum variant. Fields is nil for a unit variant.
type Variant struct {
	Base
	Doc    *Doc
	Name   *Ident
	Fields []*VariantField
}

// VariantField is `name: Type` or `from Type`.
type VariantField struct {
	Base
	Name *Ident // nil for a from field
	From bool
	Type Type
}

// InterfaceDecl is an interface declaration. Methods have nil bodies.
type InterfaceDecl struct {
	Base
	Doc        *Doc
	Pub        bool
	Name       *Ident
	TypeParams []*TypeParam
	Methods    []*FnDecl
}

// TypeDecl is `type Name = T` (alias) or `type Name struct(T)` (newtype).
type TypeDecl struct {
	Base
	Doc        *Doc
	Pub        bool
	Name       *Ident
	TypeParams []*TypeParam
	Newtype    bool
	Type       Type
}

// ConstDecl is `const NAME [: T] = value`. It is a top level declaration
// and, as a local constant, also a statement (core 3.1).
type ConstDecl struct {
	Base
	Doc   *Doc
	Pub   bool
	Name  *Ident
	Type  Type
	Value Expr
}

// TestDecl is a test block.
type TestDecl struct {
	Base
	Doc  *Doc
	Name *StringLit
	For  []*Param
	Seed *IntLit
	Uses []*Effect
	Body *Block
}

// BadDecl marks a declaration that failed to parse.
type BadDecl struct{ Base }

func (*FnDecl) declNode()        {}
func (*StructDecl) declNode()    {}
func (*EnumDecl) declNode()      {}
func (*InterfaceDecl) declNode() {}
func (*TypeDecl) declNode()      {}
func (*ConstDecl) declNode()     {}
func (*TestDecl) declNode()      {}
func (*BadDecl) declNode()       {}

func (*UsesClause) clauseNode()     {}
func (*RequiresClause) clauseNode() {}
func (*EnsuresClause) clauseNode()  {}

// ---------------------------------------------------------------------------
// Types

// NamedType is a possibly qualified, possibly generic type name:
// `Int`, `http.Response`, `Map[Str, V]`.
type NamedType struct {
	Base
	Path []*Ident // module prefix segments then the type name
	Args []Type
}

// OptionalType is `T?`.
type OptionalType struct {
	Base
	Elem Type
}

// TupleType is `(A, B)`; with no elements it is the unit type `()`.
type TupleType struct {
	Base
	Elems []Type
}

// FnType is `fn(A, var B) -> R ! E uses fx`.
type FnType struct {
	Base
	Params     []*FnTypeParam
	Result     Type
	Err        *ErrSpec
	Uses       []*Effect
	UsesParens bool
}

// FnTypeParam is a parameter type in a function type, optionally `var`.
type FnTypeParam struct {
	Base
	Var  bool
	Type Type
}

// BadType marks a type that failed to parse.
type BadType struct{ Base }

func (*NamedType) typeNode()    {}
func (*OptionalType) typeNode() {}
func (*TupleType) typeNode()    {}
func (*FnType) typeNode()       {}
func (*BadType) typeNode()      {}

// ---------------------------------------------------------------------------
// Statements

// LetStmt is `let pattern [: T] = value`.
type LetStmt struct {
	Base
	Pattern Pattern
	Type    Type
	Value   Expr
}

// VarStmt is `var name [: T] = value`.
type VarStmt struct {
	Base
	Name  *Ident
	Type  Type
	Value Expr
}

// AssignStmt is `target op value` where op is `=` or a compound operator.
type AssignStmt struct {
	Base
	Op     token.Kind
	Target Expr
	Value  Expr
}

// DeferStmt is `defer stmt` or `defer { ... }` (Body is then an ExprStmt
// holding the Block).
type DeferStmt struct {
	Base
	Body Stmt
}

// ReturnStmt is `return [value]`.
type ReturnStmt struct {
	Base
	Value Expr
}

// BreakStmt is `break [label]`.
type BreakStmt struct {
	Base
	Label *Ident
}

// ContinueStmt is `continue [label]`.
type ContinueStmt struct {
	Base
	Label *Ident
}

// ForStmt is `[label:] for pattern in iter { ... }`.
type ForStmt struct {
	Base
	Label   *Ident
	Pattern Pattern
	Iter    Expr
	Body    *Block
	// GoRange marks a loop written as Go's `for k, v := range x`. The
	// checker reports it (XO0177, decision 0087) with the Xo form for the
	// type of x and binds the names the way Go would.
	GoRange bool
}

// WhileStmt is `[label:] while [cond] { ... }`. A nil Cond is an infinite
// loop. In statement position every while loop is a WhileStmt; WhileExpr
// only appears where an expression is required.
type WhileStmt struct {
	Base
	Label *Ident
	Cond  Expr
	Body  *Block
}

// ExprStmt is an expression used as a statement.
type ExprStmt struct {
	Base
	X Expr
}

// BadStmt marks a statement that failed to parse.
type BadStmt struct{ Base }

func (*LetStmt) stmtNode()      {}
func (*VarStmt) stmtNode()      {}
func (*AssignStmt) stmtNode()   {}
func (*DeferStmt) stmtNode()    {}
func (*ReturnStmt) stmtNode()   {}
func (*BreakStmt) stmtNode()    {}
func (*ContinueStmt) stmtNode() {}
func (*ForStmt) stmtNode()      {}
func (*WhileStmt) stmtNode()    {}
func (*ExprStmt) stmtNode()     {}
func (*BadStmt) stmtNode()      {}
func (*ConstDecl) stmtNode()    {}

// ---------------------------------------------------------------------------
// Expressions

// IntLit is an integer literal. Value is the decimal value.
type IntLit struct {
	Base
	Raw   string
	Value string
}

// FloatLit is a floating point literal.
type FloatLit struct {
	Base
	Raw string
}

// DurationLit is a duration literal such as `250ms`. Nanos is the value in
// nanoseconds as a decimal string.
type DurationLit struct {
	Base
	Raw   string
	Nanos string
}

// RuneLit is a rune literal.
type RuneLit struct {
	Base
	Raw   string
	Value rune
}

// StringKind distinguishes string literal forms.
type StringKind int

const (
	PlainString StringKind = iota
	TripleString
	RawString
)

// StringLit is a string literal. Parts alternate between literal text and
// interpolated expressions; raw strings have a single literal part.
type StringLit struct {
	Base
	Kind  StringKind
	Parts []StringPart
}

// StringPart is literal text (Expr nil) or an interpolated expression.
type StringPart struct {
	Text string
	Expr Expr
}

// BytesLit is `b"..."`.
type BytesLit struct {
	Base
	Value string
}

// HoleExpr is `???` (core 3.7).
type HoleExpr struct{ Base }

// UnitExpr is `()`.
type UnitExpr struct{ Base }

// ParenExpr is `(x)`.
type ParenExpr struct {
	Base
	X Expr
}

// TupleExpr is `(a, b, ...)`.
type TupleExpr struct {
	Base
	Elems []Expr
}

// ListLit is `[a, b]`.
type ListLit struct {
	Base
	Elems []Expr
}

// MapLit is `{k: v, ...}`; `{}` is an empty map (core 1.6).
type MapLit struct {
	Base
	Entries []*MapEntry
}

// MapEntry is one `key: value` pair.
type MapEntry struct {
	Base
	Key   Expr
	Value Expr
}

// SetLit is `#{a, b}`.
type SetLit struct {
	Base
	Elems []Expr
}

// StructLit is `Type{field: value, ...}`. Type is an Ident, a SelectorExpr,
// or a TypeArgsExpr.
type StructLit struct {
	Base
	Type   Expr
	Fields []*FieldInit
}

// FieldInit is `name: value` in a struct literal or `with` update.
type FieldInit struct {
	Base
	Name  *Ident
	Value Expr
}

// UnaryExpr is `op x` for `-`, `!`, `~`.
type UnaryExpr struct {
	Base
	Op token.Kind
	X  Expr
}

// BinaryExpr is `x op y`.
type BinaryExpr struct {
	Base
	Op token.Kind
	X  Expr
	Y  Expr
}

// IsExpr is `x is Pattern`, a Bool test of a pattern that binds no names
// (core 3.5).
type IsExpr struct {
	Base
	X       Expr
	Pattern Pattern
}

// RangeExpr is `lo..hi` or `lo..=hi`. Lo and Hi may be nil only inside an
// index (slicing, core 3.6).
type RangeExpr struct {
	Base
	Lo        Expr
	Hi        Expr
	Inclusive bool
}

// SelectorExpr is `x.name`, or a tuple field `t.0` (Sel.Name is then the
// decimal index, core 3.5).
type SelectorExpr struct {
	Base
	X   Expr
	Sel *Ident
}

// CallExpr is `fun(args)`. Explicit type arguments appear as a TypeArgsExpr
// in Fun.
type CallExpr struct {
	Base
	Fun  Expr
	Args []*Arg
}

// Arg is a call argument, optionally named.
type Arg struct {
	Base
	Name  *Ident
	Value Expr
}

// TypeArgsExpr is `x[T, U]` where the brackets hold type arguments:
// `json.decode[User]`, `Mutex[Map[Str, Int]]`.
type TypeArgsExpr struct {
	Base
	X     Expr
	Types []Type
}

// IndexExpr is `x[i]` or a slice `x[a..b]` (Index is then a RangeExpr).
type IndexExpr struct {
	Base
	X     Expr
	Index Expr
}

// TryExpr is postfix `x?`.
type TryExpr struct {
	Base
	X Expr
}

// WithExpr is `x with {field: value}`.
type WithExpr struct {
	Base
	X      Expr
	Fields []*FieldInit
}

// IfExpr is `if cond { } else ...` or `if let pat = value { } else ...`.
// Else is nil, a *Block, or an *IfExpr.
type IfExpr struct {
	Base
	Cond    Expr    // nil for if let
	Pattern Pattern // if let
	Value   Expr    // if let
	Then    *Block
	Else    Expr
}

// MatchExpr is `match subject { arms }`.
type MatchExpr struct {
	Base
	Subject Expr
	Arms    []*MatchArm
}

// MatchArm is `pattern [if guard] => body`. Body is an ExprStmt for an
// expression body, or another statement such as an assignment or return.
type MatchArm struct {
	Base
	Pattern Pattern
	Guard   Expr
	Body    Stmt
}

// ScopeExpr is `scope name { ... }`.
type ScopeExpr struct {
	Base
	Name *Ident
	Body *Block
}

// SelectExpr is `select { arms }`.
type SelectExpr struct {
	Base
	Arms []*SelectArm
}

// SelectArm is one of `name = op => body`, `op => body`, or
// `after d => body`. Op is the channel operation; After is the timeout.
type SelectArm struct {
	Base
	Bind  *Ident
	Op    Expr
	After Expr
	Body  Stmt
}

// Block is `{ stmts }`. Its value is the value of its last expression
// statement (core 3.2).
type Block struct {
	Base
	Stmts []Stmt
}

// FnLit is a closure literal.
type FnLit struct {
	Base
	Params []*Param
	Result Type
	Err    *ErrSpec
	Uses   []*Effect
	Body   *Block
}

// WhileExpr is `while { ... }` in expression position: an infinite loop of
// type Never unless it contains a break.
type WhileExpr struct {
	Base
	Body *Block
}

// BadExpr marks an expression that failed to parse.
type BadExpr struct{ Base }

func (*Ident) exprNode()        {}
func (*IntLit) exprNode()       {}
func (*FloatLit) exprNode()     {}
func (*DurationLit) exprNode()  {}
func (*RuneLit) exprNode()      {}
func (*StringLit) exprNode()    {}
func (*BytesLit) exprNode()     {}
func (*HoleExpr) exprNode()     {}
func (*UnitExpr) exprNode()     {}
func (*ParenExpr) exprNode()    {}
func (*TupleExpr) exprNode()    {}
func (*ListLit) exprNode()      {}
func (*MapLit) exprNode()       {}
func (*SetLit) exprNode()       {}
func (*StructLit) exprNode()    {}
func (*UnaryExpr) exprNode()    {}
func (*BinaryExpr) exprNode()   {}
func (*IsExpr) exprNode()       {}
func (*RangeExpr) exprNode()    {}
func (*SelectorExpr) exprNode() {}
func (*CallExpr) exprNode()     {}
func (*TypeArgsExpr) exprNode() {}
func (*IndexExpr) exprNode()    {}
func (*TryExpr) exprNode()      {}
func (*WithExpr) exprNode()     {}
func (*IfExpr) exprNode()       {}
func (*MatchExpr) exprNode()    {}
func (*ScopeExpr) exprNode()    {}
func (*SelectExpr) exprNode()   {}
func (*Block) exprNode()        {}
func (*FnLit) exprNode()        {}
func (*WhileExpr) exprNode()    {}
func (*BadExpr) exprNode()      {}

// ---------------------------------------------------------------------------
// Patterns

// WildcardPat is `_`.
type WildcardPat struct{ Base }

// BindPat is a snake_case name that binds the matched value.
type BindPat struct {
	Base
	Name *Ident
}

// LitPat is a literal pattern: Int, Float, Duration, Rune, String, Bytes,
// `true`/`false` (as an Ident), or a negated number (UnaryExpr).
type LitPat struct {
	Base
	Value Expr
}

// RangePat is `lo..=hi` or `lo..hi` over literals.
type RangePat struct {
	Base
	Lo        Expr
	Hi        Expr
	Inclusive bool
}

// OrPat is `p1 | p2 | ...`.
type OrPat struct {
	Base
	Alts []Pattern
}

// TuplePat is `(p1, p2, ...)`.
type TuplePat struct {
	Base
	Elems []Pattern
}

// VariantPat is a variant, bare or qualified: `None`, `Some(x)`,
// `http.ClientErr.Canceled`, `Db(msg: m, ..)`, `Variant(..)`.
// Fields is nil and Parens false for a unit variant.
type VariantPat struct {
	Base
	Path   []*Ident
	Parens bool
	Fields []*PatField
	Rest   bool
}

// StructPat is `Type{field: p, ..}`.
type StructPat struct {
	Base
	Path   []*Ident
	Fields []*PatField
	Rest   bool
}

// PatField is `[name:] pattern`.
type PatField struct {
	Base
	Name    *Ident
	Pattern Pattern
}

// ListPat is `[p1, .., pn]`. Elems excludes the `..`; Rest is the number
// of elements before it, or -1 when there is no `..`.
type ListPat struct {
	Base
	Elems []Pattern
	Rest  int
}

// BadPat marks a pattern that failed to parse.
type BadPat struct{ Base }

func (*WildcardPat) patternNode() {}
func (*BindPat) patternNode()     {}
func (*LitPat) patternNode()      {}
func (*RangePat) patternNode()    {}
func (*OrPat) patternNode()       {}
func (*TuplePat) patternNode()    {}
func (*VariantPat) patternNode()  {}
func (*StructPat) patternNode()   {}
func (*ListPat) patternNode()     {}
func (*BadPat) patternNode()      {}
