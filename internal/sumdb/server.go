package sumdb

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
)

// The protocol (decision 0124), read only, over HTTP(S), every path below
// the log's URL:
//
//	GET /latest                          the signed tree head
//	GET /lookup/<path>@<version>         "<record id>\n<record>\n<signed tree head>"
//	GET /proof/inclusion/<id>/<n>        audit path of record id in the tree of n records
//	GET /proof/consistency/<m>/<n>       proof that the tree of m records prefixes that of n
//
// Module paths and versions are escaped as in the module cache and the
// proxy protocol (an upper case letter is '!' and its lower case). A
// record is the two xo.lock lines of a module version:
//
//	<path> <version> h1:<tree hash>
//	<path> <version>/xo.mod h1:<xo.mod hash>
//
// Proofs are base64 hashes, one per line. An unknown module version is
// 404 (410 when the log refuses it).

// Server is a checksum log over HTTP. It keeps the records in memory: it
// is the log tests run, and a starting point for a team's own log.
type Server struct {
	signer *Signer
	// lookup produces the record of a module version the log does not
	// hold yet (by fetching it from its origin); nil serves only the
	// records added with Add.
	lookup func(path, version string) ([]byte, error)

	mu      sync.Mutex
	records [][]byte
	leaves  []Hash
	index   map[string]int64
}

// NewServer is a log signed by signer that adds module versions it is
// asked for with lookup.
func NewServer(signer *Signer, lookup func(path, version string) ([]byte, error)) *Server {
	return &Server{signer: signer, lookup: lookup, index: map[string]int64{}}
}

// Add appends the record of path@version (tests use it to log a hash the
// module does not have) and returns its id; a version already logged
// keeps its first record.
func (s *Server) Add(path, version string, record []byte) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.add(path, version, record)
}

func (s *Server) add(path, version string, record []byte) int64 {
	key := path + "@" + version
	if id, ok := s.index[key]; ok {
		return id
	}
	id := int64(len(s.records))
	s.records = append(s.records, append([]byte(nil), record...))
	s.leaves = append(s.leaves, RecordHash(record))
	s.index[key] = id
	return id
}

// Size is the number of records.
func (s *Server) Size() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(len(s.leaves))
}

func (s *Server) head() []byte {
	return s.signer.SignTree(Tree{N: int64(len(s.leaves)), Hash: TreeHash(s.leaves)})
}

func writeHashes(w http.ResponseWriter, hs []Hash) {
	var b strings.Builder
	for _, h := range hs {
		b.WriteString(h.String() + "\n")
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(b.String()))
}

// ServeHTTP answers the protocol above.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "the checksum log is read only", http.StatusMethodNotAllowed)
		return
	}
	p := r.URL.Path
	switch {
	case p == "/latest":
		s.mu.Lock()
		note := s.head()
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(note)
	case strings.HasPrefix(p, "/lookup/"):
		path, version, ok := strings.Cut(strings.TrimPrefix(p, "/lookup/"), "@")
		path, err1 := UnescapePath(path)
		version, err2 := UnescapePath(version)
		if !ok || err1 != nil || err2 != nil || path == "" || version == "" {
			http.Error(w, "want /lookup/<module path>@<version>", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		id, known := s.index[path+"@"+version]
		s.mu.Unlock()
		if !known {
			if s.lookup == nil {
				http.Error(w, "unknown module version", http.StatusNotFound)
				return
			}
			rec, err := s.lookup(path, version)
			if err != nil {
				http.Error(w, fmt.Sprintf("%s %s: %v", path, version, err), http.StatusNotFound)
				return
			}
			s.mu.Lock()
			id = s.add(path, version, rec)
			s.mu.Unlock()
		}
		s.mu.Lock()
		rec, note := s.records[id], s.head()
		s.mu.Unlock()
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintf(w, "%d\n%s\n%s", id, rec, note)
	case strings.HasPrefix(p, "/proof/inclusion/"), strings.HasPrefix(p, "/proof/consistency/"):
		inclusion := strings.HasPrefix(p, "/proof/inclusion/")
		rest := strings.TrimPrefix(strings.TrimPrefix(p, "/proof/inclusion/"), "/proof/consistency/")
		a, b, ok := strings.Cut(rest, "/")
		x, err1 := strconv.ParseInt(a, 10, 64)
		n, err2 := strconv.ParseInt(b, 10, 64)
		if !ok || err1 != nil || err2 != nil || x < 0 || n < 0 {
			http.Error(w, "want /proof/inclusion/<id>/<n> or /proof/consistency/<m>/<n>", http.StatusBadRequest)
			return
		}
		s.mu.Lock()
		defer s.mu.Unlock()
		if n > int64(len(s.leaves)) || x > n || (inclusion && x >= n) {
			http.Error(w, fmt.Sprintf("the log has %d records", len(s.leaves)), http.StatusNotFound)
			return
		}
		if inclusion {
			writeHashes(w, InclusionProof(s.leaves[:n], x))
		} else {
			writeHashes(w, ConsistencyProof(s.leaves[:n], x))
		}
	default:
		http.NotFound(w, r)
	}
}

// EscapePath escapes a module path or version for URLs and file names:
// an upper case letter becomes '!' and its lower case (as the module
// cache and Go's protocols do).
func EscapePath(p string) string {
	var b strings.Builder
	for _, r := range p {
		if r >= 'A' && r <= 'Z' {
			b.WriteByte('!')
			b.WriteRune(r + ('a' - 'A'))
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// UnescapePath reverses EscapePath; a bare upper case letter or a '!'
// not followed by a lower case letter is an error.
func UnescapePath(p string) (string, error) {
	var b strings.Builder
	bang := false
	for _, r := range p {
		switch {
		case bang:
			if r < 'a' || r > 'z' {
				return "", fmt.Errorf("malformed escaped path %q", p)
			}
			b.WriteRune(r - ('a' - 'A'))
			bang = false
		case r == '!':
			bang = true
		case r >= 'A' && r <= 'Z':
			return "", fmt.Errorf("malformed escaped path %q: upper case letters are escaped", p)
		default:
			b.WriteRune(r)
		}
	}
	if bang {
		return "", fmt.Errorf("malformed escaped path %q", p)
	}
	return b.String(), nil
}
