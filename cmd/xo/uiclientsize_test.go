package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

// TestUIClientSize guards std/ui's wasm bundle (decision 0659 stage 3,
// std/evidence/ui/browser.md): examples/ui-todo's client, the module a
// predicting page downloads, stays under a size budget. On 2026-11-17 it
// was 270,737 bytes, 91,190 gzipped; on Chrome's slow 4G preset its
// download is most of the 866 ms until the page resumes. The budgets
// leave about 20% room; raise them only with a new measurement in
// browser.md.
func TestUIClientSize(t *testing.T) {
	t.Parallel()
	const maxRaw, maxGzip = 325_000, 110_000
	out := filepath.Join(t.TempDir(), "client.wasm")
	var stdout, stderr bytes.Buffer
	client := filepath.Join(repoRoot, "examples", "ui-todo", "client")
	if code := run(t, []string{"build", "--targets=wasip1/wasm", "-o", out, client}, &stdout, &stderr); code != 0 {
		t.Fatalf("build: exit %d\n%s%s", code, stdout.String(), stderr.String())
	}
	raw, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var z bytes.Buffer
	w, _ := gzip.NewWriterLevel(&z, gzip.BestCompression)
	w.Write(raw)
	w.Close()
	t.Logf("ui-todo client: %d bytes, %d gzipped", len(raw), z.Len())
	if len(raw) > maxRaw || z.Len() > maxGzip {
		t.Errorf("ui-todo client is %d bytes (%d gzipped), over the budget of %d (%d gzipped): measure it in a browser (std/evidence/ui/browser.md) before raising the budget", len(raw), z.Len(), maxRaw, maxGzip)
	}
}
