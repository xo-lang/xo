package gen

import "github.com/xo-lang/xo/internal/check"

// genSqlRowStruct emits XoSqlRowDec for a struct with `derive SqlRow`
// (std.md 10.4): each field is read from the column of the same name by
// rt.SqlRowField, which applies the rules of the row accessors. The checker
// allows the derive only on structs without type parameters whose fields
// are Int, Float, Str, Bool, Time, or an Option of one, so no field is
// boxed.
func (pg *pkgGen) genSqlRowStruct(t *check.TypeObj) {
	rt := pg.rt()
	pg.p("func (v *%s) XoSqlRowDec(tk *%sTask, r %sSql_Row) *%sSql_SqlErr {", selfType(t), rt, rt, rt)
	for _, f := range t.Fields {
		pg.p("\tif e := %sSqlRowField(r, %q, &v.%s); e != nil {\n\t\treturn e\n\t}", rt, f.Name, goField(f.Name))
	}
	pg.p("\treturn nil\n}\n")
}
