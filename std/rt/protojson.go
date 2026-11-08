package rt

// M_display is `JsonErr.display()` of std/protojson: protojson's message.
func (e Protojson_JsonErr) M_display(*Task) string { return e.F_text }
