package rt

// M_display is `e.display()`: plain text (decision 0358), the same as
// std/native/sql.xo's sqlerr_display.
func (v Sql_SqlErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case Sql_SqlErr_Syntax:
		return "syntax error: " + p.F_msg
	case Sql_SqlErr_Constraint:
		return p.F_msg
	case Sql_SqlErr_Param:
		return p.F_msg
	case Sql_SqlErr_NoTable:
		return "no such table: " + p.F_name
	case Sql_SqlErr_NoColumn:
		return "no such column: " + p.F_name
	case Sql_SqlErr_Mismatch:
		return "column " + p.F_column + ": " + p.F_msg
	case Sql_SqlErr_Conn:
		return "connection: " + p.F_msg
	case Sql_SqlErr_Driver:
		return p.F_msg
	case Sql_SqlErr_Server:
		return p.F_msg + " (SQLSTATE " + p.F_code + ")"
	}
	switch v.Tag {
	case sqlTagNoRows:
		return "no rows in result set"
	case sqlTagTxDone:
		return "transaction already committed or rolled back"
	case sqlTagClosed:
		return "database is closed"
	}
	return "canceled"
}
