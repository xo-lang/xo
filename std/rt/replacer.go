package rt

import "strings"

// Replacer is the prelude's Replacer (decision 0594): Go's
// strings.Replacer itself.
type Replacer struct{ r *strings.Replacer }

// Replacer_new is `Replacer.new(pairs)`.
func Replacer_new(_ *Task, pairs List[Tuple2[string, string]]) Replacer {
	var oldnew []string
	for p := range List_All(pairs) {
		oldnew = append(oldnew, p.F0, p.F1)
	}
	return Replacer{strings.NewReplacer(oldnew...)}
}

// M_replace is `r.replace(s)`.
func (r Replacer) M_replace(_ *Task, s string) string { return r.r.Replace(s) }

func (r Replacer) XoEq(x any) bool {
	o, ok := x.(Replacer)
	return ok && o.r == r.r
}

func (Replacer) XoDebug(b *strings.Builder) { b.WriteString("<replacer>") }
