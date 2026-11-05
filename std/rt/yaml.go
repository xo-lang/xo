package rt

// std/yaml on the Go backend: compiled from std/native/yaml.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces.

// M_display is yaml.v3's error text: `yaml: ` and the message
// (std/native/yaml.xo yamlerr_display).
func (e Yaml_YamlErr) M_display(*Task) string {
	return "yaml: " + e.F_msg
}
