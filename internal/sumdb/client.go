package sumdb

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Error is a failed check. Security is set when the log, or what was
// fetched, cannot be trusted: the log's record differs from the fetched
// hashes, a signature or proof does not verify, or the log contradicts a
// tree this client saw before (XO1313). Otherwise the log could not be
// consulted (unreachable, malformed answers; XO1314).
type Error struct {
	Security bool
	Msg      string
}

func (e *Error) Error() string { return e.Msg }

func security(format string, args ...any) *Error {
	return &Error{Security: true, Msg: fmt.Sprintf(format, args...)}
}

func unavailable(format string, args ...any) *Error {
	return &Error{Msg: fmt.Sprintf(format, args...)}
}

// Client checks module versions against a log.
type Client struct {
	URL      string    // the log's base URL
	Verifier *Verifier // its public key
	// StateDir keeps the latest tree head this client verified
	// (<StateDir>/<log name>/latest), so a later answer must extend it.
	StateDir string
	HTTP     *http.Client
}

// ParseConfig parses XO_SUMDB: "<verifier key>" (the log is at
// https://<name>) or "<verifier key> <URL>".
func ParseConfig(s string) (*Verifier, string, error) {
	f := strings.Fields(s)
	if len(f) == 0 || len(f) > 2 {
		return nil, "", fmt.Errorf("XO_SUMDB=%q: want off, or <name>+<key id>+<public key> [<URL>]", s)
	}
	v, err := ParseVerifier(f[0])
	if err != nil {
		return nil, "", err
	}
	url := "https://" + v.Name
	if len(f) == 2 {
		url = f[1]
		if !strings.HasPrefix(url, "https://") && !strings.HasPrefix(url, "http://") {
			return nil, "", fmt.Errorf("XO_SUMDB: the log URL %q is not http or https", url)
		}
	}
	return v, strings.TrimRight(url, "/"), nil
}

func (c *Client) get(p string) ([]byte, int, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: time.Minute}
	}
	resp, err := hc.Get(c.URL + p)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode != http.StatusOK {
		return body, resp.StatusCode, fmt.Errorf("%s%s: %s: %s", c.URL, p, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, resp.StatusCode, nil
}

func parseHashes(body []byte) ([]Hash, error) {
	var out []Hash
	sc := bufio.NewScanner(bytes.NewReader(body))
	for sc.Scan() {
		l := strings.TrimSpace(sc.Text())
		if l == "" {
			continue
		}
		h, err := ParseHash(l)
		if err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, nil
}

func (c *Client) statePath() string {
	return filepath.Join(c.StateDir, c.Verifier.Name, "latest")
}

// stored is the latest verified tree head, the zero tree when none.
func (c *Client) stored() (Tree, error) {
	if c.StateDir == "" {
		return Tree{}, nil
	}
	data, err := os.ReadFile(c.statePath())
	if err != nil {
		return Tree{}, nil
	}
	t, err := c.Verifier.VerifyTree(data)
	if err != nil {
		return Tree{}, security("the stored tree head %s does not verify: %v", c.statePath(), err)
	}
	return t, nil
}

func (c *Client) store(note []byte) {
	if c.StateDir == "" {
		return
	}
	p := c.statePath()
	if os.MkdirAll(filepath.Dir(p), 0o755) != nil {
		return
	}
	tmp := p + ".tmp" + strconv.Itoa(os.Getpid())
	if os.WriteFile(tmp, note, 0o644) == nil {
		os.Rename(tmp, p)
	}
}

// Check verifies that the log holds exactly the record lines want for
// path@version, that the record is in the signed tree, and that the tree
// is consistent with the latest one this client verified before (which
// it then replaces when larger).
func (c *Client) Check(path, version string, want []string) error {
	body, status, err := c.get("/lookup/" + EscapePath(path) + "@" + EscapePath(version))
	if err != nil {
		if status == http.StatusNotFound || status == http.StatusGone {
			return unavailable("the checksum log %s has no record of %s %s: %v", c.Verifier.Name, path, version, err)
		}
		return unavailable("cannot reach the checksum log %s: %v", c.Verifier.Name, err)
	}
	idLine, rest, ok := bytes.Cut(body, []byte("\n"))
	record, note, ok2 := bytes.Cut(rest, []byte("\n\n"))
	if !ok || !ok2 {
		return unavailable("malformed lookup answer from the checksum log %s", c.Verifier.Name)
	}
	record = append(record, '\n')
	id, err := strconv.ParseInt(string(idLine), 10, 64)
	if err != nil || id < 0 {
		return unavailable("malformed record id %q from the checksum log %s", idLine, c.Verifier.Name)
	}
	tree, err := c.Verifier.VerifyTree(note)
	if err != nil {
		return security("checksum log %s: %v", c.Verifier.Name, err)
	}
	// the record itself
	got := strings.Split(strings.TrimSuffix(string(record), "\n"), "\n")
	if !sameLines(got, want) {
		return security("%s %s: the checksum log %s records\n%s\nbut the fetched module has\n%s", path, version, c.Verifier.Name, indent(got), indent(want))
	}
	if id >= tree.N {
		return security("checksum log %s: record %d is outside its signed tree of %d records", c.Verifier.Name, id, tree.N)
	}
	pb, _, err := c.get(fmt.Sprintf("/proof/inclusion/%d/%d", id, tree.N))
	if err != nil {
		return unavailable("checksum log %s: inclusion proof: %v", c.Verifier.Name, err)
	}
	proof, err := parseHashes(pb)
	if err != nil {
		return unavailable("checksum log %s: inclusion proof: %v", c.Verifier.Name, err)
	}
	if err := VerifyInclusion(proof, tree.N, id, RecordHash(record), tree.Hash); err != nil {
		return security("checksum log %s: the record of %s %s is not in its signed tree (%v)", c.Verifier.Name, path, version, err)
	}
	// the tree against the latest one seen
	old, err := c.stored()
	if err != nil {
		return err
	}
	if err := c.consistent(old, tree); err != nil {
		return err
	}
	if tree.N > old.N {
		c.store(note)
	}
	return nil
}

// consistent checks that the smaller of two verified trees is a prefix of
// the larger.
func (c *Client) consistent(a, b Tree) error {
	if a.N > b.N {
		a, b = b, a
	}
	if a.N == b.N {
		if a.Hash != b.Hash {
			return security("checksum log %s: two signed trees of %d records differ: the log forked", c.Verifier.Name, a.N)
		}
		return nil
	}
	if a.N == 0 {
		return nil
	}
	pb, _, err := c.get(fmt.Sprintf("/proof/consistency/%d/%d", a.N, b.N))
	if err != nil {
		return security("checksum log %s: cannot prove that its tree of %d records extends the one of %d seen before: %v", c.Verifier.Name, b.N, a.N, err)
	}
	proof, err := parseHashes(pb)
	if err != nil {
		return unavailable("checksum log %s: consistency proof: %v", c.Verifier.Name, err)
	}
	if err := VerifyConsistency(proof, a.N, b.N, a.Hash, b.Hash); err != nil {
		return security("checksum log %s: its tree of %d records does not extend the one of %d seen before: the log forked or rewrote history (%v)", c.Verifier.Name, b.N, a.N, err)
	}
	return nil
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	a, b = append([]string(nil), a...), append([]string(nil), b...)
	sort.Strings(a)
	sort.Strings(b)
	for i := range a {
		if strings.Join(strings.Fields(a[i]), " ") != strings.Join(strings.Fields(b[i]), " ") {
			return false
		}
	}
	return true
}

func indent(lines []string) string {
	return "  " + strings.Join(lines, "\n  ")
}
