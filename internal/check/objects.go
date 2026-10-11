package check

import (
	"github.com/xo-lang/xo/internal/ast"
	"github.com/xo-lang/xo/internal/cbind"
	"github.com/xo-lang/xo/internal/gobind"
	"github.com/xo-lang/xo/internal/token"
)

// Module is a directory of .xo files sharing one namespace (core 1.1), or a
// std module loaded from the embedded declaration files.
type Module struct {
	Path      string // import path: "std/http", "example.com/notes/domain", or the directory
	Name      string // default local name (core 4.1)
	Dir       string
	Files     []*File
	Scope     map[string]Object
	IsStd     bool
	IsPrelude bool
	// Native marks an implementation module of the native backends
	// (std/native/<name>.xo, decision 0074): a module with bodies that
	// implements std module Implements ("prelude" or a std module path
	// such as "std/http"). Only native modules may import the intrinsics
	// module "native/rt".
	Native     bool
	Implements string
	// Go is the binding of a `use go "<path>"` module (decision 0041). Such
	// a module is also IsStd: its declaration file is a bodiless signature
	// listing like std/*.xo, and its functions are adapters in package rt.
	Go *gobind.Binding
	// C is the binding of a `use c "<header>"` module (decision 0116),
	// also IsStd: its functions are adapters of the native runtime.
	C *cbind.Binding
	// Root is true for modules named on the command line (diagnostics are
	// reported for them and for user modules they import).
	Root bool
	// Edition is the `edition` of the module's xo.mod (core 13), or
	// CurrentEdition when there is none.
	Edition string
	// GC is the `gc` lines of the module's xo.mod (decision 0065), used
	// when the module is a program's main module.
	GC GCConfig
	// Dependency is "<module path>@<version>" for a module loaded from a
	// required module version (the module cache or vendor/, decision
	// 0086); "" for the main module and workspace modules. Its files are
	// not edited.
	Dependency string

	state int // 0 new, 1 loading, 2 loaded (cycle detection)
}

// File is one parsed source file with its imports.
type File struct {
	AST     *ast.File
	Path    string
	Module  *Module
	Imports map[string]*Import
	IsTest  bool // name ends in _test.xo
	Src     []byte
}

// Import is one `use` declaration in a file.
type Import struct {
	Name   string
	Module *Module // nil when the path did not resolve
	Decl   *ast.UseDecl
	Used   bool
}

// Object is a top level or local named entity.
type Object interface {
	objName() string
}

// TypeKind distinguishes type declarations.
type TypeKind int

const (
	KStruct TypeKind = iota
	KEnum
	KInterface
	KAlias
	KNewtype
)

// TypeObj is a declared type.
type TypeObj struct {
	Name    string
	Kind    TypeKind
	Module  *Module
	Pub     bool
	Span    token.Span
	TParams []*TParam

	Fields     []*FieldObj   // struct
	Variants   []*VariantObj // enum
	IfaceMeths []*FuncObj    // interface
	Target     Type          // alias target or newtype underlying type
	Derives    map[string]bool
	Invariant  ast.Expr
	Opaque     bool // empty std struct: an opaque handle (std encoding rule 2)

	Methods map[string]*FuncObj
	Statics map[string]*FuncObj

	structDecl *ast.StructDecl
	enumDecl   *ast.EnumDecl
	ifaceDecl  *ast.InterfaceDecl
	typeDecl   *ast.TypeDecl
	resolving  bool
	resolved   bool
}

func (t *TypeObj) objName() string { return t.Name }

// QualName is the name used in messages: std types outside the prelude are
// qualified with their module name.
func (t *TypeObj) QualName() string {
	if t.Module != nil && t.Module.Native {
		return "native." + t.Module.Name + "." + t.Name
	}
	if t.Module != nil && t.Module.IsStd && !t.Module.IsPrelude {
		return t.Module.Name + "." + t.Name
	}
	return t.Name
}

func (t *TypeObj) field(name string) *FieldObj {
	for _, f := range t.Fields {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func (t *TypeObj) variant(name string) *VariantObj {
	for _, v := range t.Variants {
		if v.Name == name {
			return v
		}
	}
	return nil
}

// FieldObj is a struct field.
type FieldObj struct {
	Name    string
	Type    Type
	Pub     bool
	Default ast.Expr
	Span    token.Span
}

// VariantObj is an enum variant.
type VariantObj struct {
	Name   string
	Enum   *TypeObj
	Index  int
	Fields []*VFieldObj
	Span   token.Span
	// Unit is true when the variant was declared without parentheses.
	Unit bool
}

// VFieldObj is a variant field; From fields have no name.
type VFieldObj struct {
	Name string
	From bool
	Type Type
	// Span is the field's name, or empty for a `from` field.
	Span token.Span
}

// ParamInfo is a declared function parameter.
type ParamInfo struct {
	Name    string
	Type    Type
	Var     bool
	Default ast.Expr
	Span    token.Span
	// ref is what a named argument for this parameter names, for the
	// names section of `xo typedump`: a paramRef or a fieldRef.
	ref any
}

// FuncObj is a function, method, or interface method.
type FuncObj struct {
	Name    string
	Module  *Module
	File    *File
	Pub     bool
	Decl    *ast.FnDecl
	Span    token.Span
	Recv    *TypeObj // method receiver type, nil for functions
	RecvVar bool
	Static  bool // receiver named `static` (std encoding rule 4)
	Const   bool // static associated constant: `Time.UNIX_EPOCH`
	Moves   bool // `@moves` doc marker
	Iface   bool // interface method

	// RecvType is the receiver type pattern, with implicit parameters.
	RecvType Type
	TParams  []*TParam // implicit receiver parameters first, then declared
	Params   []*ParamInfo
	Result   Type
	Err      Type    // nil: cannot fail
	Declared *EffSet // declared effects; nil when inferred (non pub)
	// Realtime marks a function declared with `uses realtime` (core 6.5,
	// decision 0133); RealtimeSpan is the word.
	Realtime     bool
	RealtimeSpan token.Span

	sigDone bool
	// effects performed by the body (filled while checking it)
	body *EffSet
}

func (f *FuncObj) objName() string { return f.Name }

// DisplayName is `Type.name` for methods and `name` otherwise.
func (f *FuncObj) DisplayName() string {
	if f.Recv != nil {
		return f.Recv.Name + "." + f.Name
	}
	return f.Name
}

// sig returns the function type of f (without receiver).
func (f *FuncObj) sig() *Func {
	ps := make([]FParam, len(f.Params))
	for i, p := range f.Params {
		ps[i] = FParam{Var: p.Var, T: p.Type}
	}
	eff := f.Declared
	if eff == nil {
		eff = &EffSet{Items: []EffItem{{Fn: f}}}
	}
	return &Func{Params: ps, Result: f.Result, Err: f.Err, Eff: eff}
}

// ConstObj is a top level constant.
type ConstObj struct {
	Name     string
	Module   *Module
	File     *File
	Pub      bool
	Decl     *ast.ConstDecl
	Type     Type
	checking bool
	done     bool
}

func (c *ConstObj) objName() string { return c.Name }

// VarKind classifies local bindings.
type VarKind int

const (
	VLocal VarKind = iota // let, var, pattern binding
	VParam
	VRecv
	VPredeclared
)

// VarObj is a local binding or parameter.
type VarObj struct {
	Name    string
	Type    Type
	Mutable bool
	Kind    VarKind
	Span    token.Span
	Used    bool
	ctx     *fnCtx
	// scopeHandle is set for the name bound by `scope s`.
	scopeHandle *scopeFrame
	// groupFrame is set for a binding of the task group `s.group()`: the
	// frame of scope s, which owns the group's tasks (core 7.1).
	groupFrame *scopeFrame
	awaited    bool
	isVarStmt  bool
	// isConst marks a local `const` (core 3.1), which may appear in other
	// constant initializers.
	isConst bool
	decl    ast.Node
}

func (v *VarObj) objName() string { return v.Name }
