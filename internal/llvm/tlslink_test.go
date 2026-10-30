package llvm

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/xotool"
)

// TestTLSLinkedOnlyWithTLS (decision 0244): a plain http.server links no
// TLS server (tlssrv, tls13srv, tls12srv) and no HTTP/2 (h2srv); a server
// with with_tls does. The code is reachable only through the table
// Server.with_tls installs (srvtls.xo).
func TestTLSLinkedOnlyWithTLS(t *testing.T) {
	if testing.Short() {
		t.Skip("builds two programs per native backend")
	}
	if runtime.GOARCH != "arm64" {
		t.Skip("the symbol names checked are the arm64 backend's")
	}
	for _, c := range []struct {
		dir  string
		want bool
	}{{"plain", false}, {"tls", true}} {
		t.Run(c.dir, func(t *testing.T) {
			res, err := xotool.Build([]string{filepath.Join("testdata", "tlslink", c.dir)}, xotool.Options{Backend: xotool.BackendArm64})
			if err != nil || res.Failed {
				t.Fatalf("build: %v %v", err, res)
			}
			out, err := exec.Command("nm", res.Binaries[xotool.Host()]).Output()
			if err != nil {
				t.Fatalf("nm: %v", err)
			}
			for _, mod := range []string{"tlssrv", "tls13srv", "tls12srv", "h2srv"} {
				// (Mach-O names carry a leading underscore, ELF names do not)
				has := strings.Contains(string(out), "xo_f_"+mod+"_2e")
				if has != c.want {
					t.Errorf("%s: symbols of %s: %v, want %v", c.dir, mod, has, c.want)
				}
			}
		})
	}
}
