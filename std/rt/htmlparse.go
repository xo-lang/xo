package rt

// std/htmlparse (decision 0570): the tree builder is Xo source (std/native/htmlparse.xo,
// compiled for this backend through std/go_compiled.txt); only the error display methods
// of its declared structs live here.

// M_display is the parse error's text (Go's: the 512 element nesting limit).
func (e Htmlparse_ParseErr) M_display(*Task) string { return e.F_msg }

// M_display is the render error's text.
func (e Htmlparse_RenderErr) M_display(*Task) string { return e.F_msg }
