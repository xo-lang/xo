package sumdb

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Keys and signed tree heads (decision 0124). They follow the shape of
// Go's signed notes, with Xo's own text so neither is mistaken for the
// other:
//
//	verifier key  <name>+<key id, 8 hex digits>+<base64 Ed25519 public key>
//	signer key    PRIVATE+KEY+<name>+<key id>+<base64 Ed25519 seed>
//
// The key id is the first four bytes of SHA-256("<name>\n" || public key),
// so a client names the key it trusts and a log cannot swap it. A signed
// tree head is
//
//	xo checksum log
//	<name>
//	<number of records>
//	<base64 tree hash>
//
//	sig <name> <base64(key id || Ed25519 signature of the four lines)>

// Verifier is the public key of a log: what XO_SUMDB names.
type Verifier struct {
	Name string
	ID   uint32
	Key  ed25519.PublicKey
}

// Signer is the private key of a log (servers only).
type Signer struct {
	Name string
	ID   uint32
	Key  ed25519.PrivateKey
}

func keyID(name string, pub ed25519.PublicKey) uint32 {
	h := sha256.Sum256(append([]byte(name+"\n"), pub...))
	return binary.BigEndian.Uint32(h[:4])
}

func validName(name string) bool {
	return name != "" && !strings.ContainsAny(name, "+ \t\r\n")
}

// GenerateKey makes a new key pair for the log name: the signer key the
// server keeps and the verifier key clients put in XO_SUMDB.
func GenerateKey(name string, rand io.Reader) (signer, verifier string, err error) {
	if !validName(name) {
		return "", "", fmt.Errorf("invalid log name %q: no '+' or white space", name)
	}
	pub, priv, err := ed25519.GenerateKey(rand)
	if err != nil {
		return "", "", err
	}
	id := keyID(name, pub)
	verifier = fmt.Sprintf("%s+%08x+%s", name, id, base64.StdEncoding.EncodeToString(pub))
	signer = fmt.Sprintf("PRIVATE+KEY+%s+%08x+%s", name, id, base64.StdEncoding.EncodeToString(priv.Seed()))
	return signer, verifier, nil
}

func parseID(s string) (uint32, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 4 {
		return 0, fmt.Errorf("key id %q is not 8 hex digits", s)
	}
	return binary.BigEndian.Uint32(b), nil
}

// ParseVerifier parses a verifier key.
func ParseVerifier(s string) (*Verifier, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "+", 3)
	if len(parts) != 3 || !validName(parts[0]) {
		return nil, fmt.Errorf("malformed checksum log key %q: want <name>+<key id>+<base64 public key>", s)
	}
	id, err := parseID(parts[1])
	if err != nil {
		return nil, err
	}
	pub, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("malformed checksum log key %q: the key is not a base64 Ed25519 public key", s)
	}
	if keyID(parts[0], pub) != id {
		return nil, fmt.Errorf("checksum log key %q: the key id does not match the name and key", s)
	}
	return &Verifier{Name: parts[0], ID: id, Key: pub}, nil
}

// ParseSigner parses a signer key.
func ParseSigner(s string) (*Signer, error) {
	parts := strings.SplitN(strings.TrimSpace(s), "+", 5)
	if len(parts) != 5 || parts[0] != "PRIVATE" || parts[1] != "KEY" || !validName(parts[2]) {
		return nil, fmt.Errorf("malformed signer key: want PRIVATE+KEY+<name>+<key id>+<base64 seed>")
	}
	id, err := parseID(parts[3])
	if err != nil {
		return nil, err
	}
	seed, err := base64.StdEncoding.DecodeString(parts[4])
	if err != nil || len(seed) != ed25519.SeedSize {
		return nil, fmt.Errorf("malformed signer key: the seed is not 32 base64 bytes")
	}
	priv := ed25519.NewKeyFromSeed(seed)
	if keyID(parts[2], priv.Public().(ed25519.PublicKey)) != id {
		return nil, fmt.Errorf("signer key: the key id does not match the name and key")
	}
	return &Signer{Name: parts[2], ID: id, Key: priv}, nil
}

// Tree is a tree head: the number of records and the tree hash.
type Tree struct {
	N    int64
	Hash Hash
}

const noteHeader = "xo checksum log\n"

func treeText(name string, t Tree) string {
	return fmt.Sprintf("%s%s\n%d\n%s\n", noteHeader, name, t.N, t.Hash)
}

// SignTree is the signed tree head of t.
func (s *Signer) SignTree(t Tree) []byte {
	text := treeText(s.Name, t)
	sig := ed25519.Sign(s.Key, []byte(text))
	var id [4]byte
	binary.BigEndian.PutUint32(id[:], s.ID)
	return []byte(fmt.Sprintf("%s\nsig %s %s\n", text, s.Name, base64.StdEncoding.EncodeToString(append(id[:], sig...))))
}

// VerifyTree checks a signed tree head with the verifier key and returns
// the tree. A note not signed by the key, or for another log, fails.
func (v *Verifier) VerifyTree(note []byte) (Tree, error) {
	text, sigs, ok := bytes.Cut(note, []byte("\n\n"))
	if !ok {
		return Tree{}, fmt.Errorf("malformed signed tree head")
	}
	text = append(text, '\n')
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if len(lines) != 4 || lines[0]+"\n" != noteHeader {
		return Tree{}, fmt.Errorf("malformed signed tree head")
	}
	if lines[1] != v.Name {
		return Tree{}, fmt.Errorf("signed tree head is from the log %q, not %q", lines[1], v.Name)
	}
	n, err := strconv.ParseInt(lines[2], 10, 64)
	if err != nil || n < 0 {
		return Tree{}, fmt.Errorf("malformed tree size %q", lines[2])
	}
	h, err := ParseHash(lines[3])
	if err != nil {
		return Tree{}, err
	}
	for _, l := range strings.Split(strings.TrimSpace(string(sigs)), "\n") {
		f := strings.Fields(l)
		if len(f) != 3 || f[0] != "sig" || f[1] != v.Name {
			continue
		}
		raw, err := base64.StdEncoding.DecodeString(f[2])
		if err != nil || len(raw) != 4+ed25519.SignatureSize || binary.BigEndian.Uint32(raw[:4]) != v.ID {
			continue
		}
		if ed25519.Verify(v.Key, text, raw[4:]) {
			return Tree{N: n, Hash: h}, nil
		}
		return Tree{}, fmt.Errorf("the signature of the tree head does not verify with the key of %s", v.Name)
	}
	return Tree{}, fmt.Errorf("the tree head carries no signature by the key %s+%08x", v.Name, v.ID)
}
