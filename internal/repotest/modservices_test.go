package repotest

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestModServicesConsistent: scripts/modservices (decision 0568) stays in
// step with itself and with xo: the tunnel's ingress sends both hostnames
// to the address the xo-sumdb unit listens on (loopback only) and 404s
// the rest, every flag the unit passes is one `xo mod sumdb serve`'s usage
// lists, and install.sh installs both files. The owner's first install is
// on a host no test reaches, so a drift found there costs a debugging
// session over ssh.
func TestModServicesConsistent(t *testing.T) {
	dir := filepath.Join(root, "scripts", "modservices")
	read := func(p string) string {
		t.Helper()
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	unit := read(filepath.Join(dir, "xo-sumdb.service"))
	var exec string
	for _, l := range strings.Split(unit, "\n") {
		if strings.HasPrefix(l, "ExecStart=") {
			exec = l
		}
	}
	m := regexp.MustCompile(`--addr=(\S+)`).FindStringSubmatch(exec)
	if m == nil || !strings.HasPrefix(m[1], "127.0.0.1:") {
		t.Fatalf("xo-sumdb.service listens on %q, want a 127.0.0.1 address: %s", m, exec)
	}
	addr := m[1]

	// the usage line of xo mod sumdb serve in the help golden
	var usage string
	for _, l := range strings.Split(read(filepath.Join(root, "cmd", "xo", "testdata", "clihelp", "mod.txt")), "\n") {
		if strings.Contains(l, "xo mod sumdb serve ") {
			usage = l
		}
	}
	if usage == "" {
		t.Fatal("no `xo mod sumdb serve` usage line in cmd/xo/testdata/clihelp/mod.txt")
	}
	if !strings.Contains(exec, " mod sumdb serve ") {
		t.Fatalf("ExecStart does not run xo mod sumdb serve: %s", exec)
	}
	for _, f := range regexp.MustCompile(`--([a-z-]+)`).FindAllStringSubmatch(exec, -1) {
		if !strings.Contains(usage, "--"+f[1]+"=") && !strings.Contains(usage, "[--"+f[1]+"]") {
			t.Errorf("xo-sumdb.service passes --%s, which xo mod sumdb serve's usage does not list:\n%s", f[1], usage)
		}
	}

	cf := read(filepath.Join(dir, "cloudflared-config.yml"))
	hosts := map[string]bool{}
	var services []string
	for _, l := range strings.Split(cf, "\n") {
		l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- "))
		if h, ok := strings.CutPrefix(l, "hostname: "); ok {
			hosts[h] = true
		}
		if s, ok := strings.CutPrefix(l, "service: "); ok {
			services = append(services, s)
		}
	}
	if !hosts["sum.xolang.org"] || !hosts["index.xolang.org"] || len(hosts) != 2 {
		t.Errorf("the ingress hostnames are %v, want sum.xolang.org and index.xolang.org", hosts)
	}
	if len(services) < 2 || services[len(services)-1] != "http_status:404" {
		t.Errorf("the ingress's last rule is not http_status:404: %v", services)
	}
	for _, s := range services[:max(len(services)-1, 0)] {
		if s != "http://"+addr {
			t.Errorf("an ingress rule goes to %s, but the unit listens on %s", s, addr)
		}
	}

	install := read(filepath.Join(dir, "install.sh"))
	for _, f := range []string{"xo-sumdb.service", "cloudflared-config.yml"} {
		if !strings.Contains(install, f) {
			t.Errorf("install.sh does not install %s", f)
		}
	}
}
