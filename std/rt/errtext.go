package rt

// Plain error messages (decision 0358): the Display of the prelude's error
// enums, the same texts as std/native/prelude.xo's fserr_display and
// the others. Variants without fields are told by Tag (declaration order,
// from 1).

// fsText is path and what is wrong with it, or the text alone for an empty path.
func fsText(path, text string) string {
	if path == "" {
		return text
	}
	return path + ": " + text
}

// M_display is `e.display()`.
func (v FsErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case FsErr_NotFound:
		return fsText(p.F_path, "no such file or directory")
	case FsErr_Exists:
		return fsText(p.F_path, "file exists")
	case FsErr_NotEmpty:
		return fsText(p.F_path, "directory not empty")
	case FsErr_NotDir:
		return fsText(p.F_path, "not a directory")
	case FsErr_IsDir:
		return fsText(p.F_path, "is a directory")
	case FsErr_Denied:
		return fsText(p.F_path, "permission denied")
	case FsErr_Io:
		return fsText(p.F_path, p.F_msg)
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v NetErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case NetErr_InvalidAddr:
		return "invalid address: " + p.F_msg
	case NetErr_Io:
		return p.F_msg
	case NetErr_Denied:
		return "denied by Net.only: " + p.F_addr
	}
	switch v.Tag {
	case 2:
		return "connection refused"
	case 3:
		return "i/o timeout"
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v IoErr) M_display(*Task) string {
	if p, ok := v.P.(IoErr_Io); ok {
		return p.F_msg
	}
	if v.Tag == 1 {
		return "closed"
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v EnvErr) M_display(*Task) string {
	return v.P.(EnvErr_Missing).F_name + " is not set"
}

// M_display is `e.display()`.
func (v ProcErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case ProcErr_NotFound:
		return p.F_cmd + ": executable file not found"
	case ProcErr_Io:
		return p.F_msg
	case ProcErr_Denied:
		return "denied by Proc.only: " + p.F_cmd
	}
	return "canceled"
}

// M_display is `e.display()`.
func (v ClockErr) M_display(*Task) string { return "canceled" }
