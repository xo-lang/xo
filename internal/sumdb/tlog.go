// Package sumdb is Xo's checksum log (decision 0124): an append-only
// Merkle tree of module version hashes, as Go's checksum database
// (sum.golang.org) and Certificate Transparency (RFC 6962) keep them. A
// client checks, on the first fetch of each module version, that the
// log holds the same hashes the fetched tree has, that the record is in
// the tree the log signed (an inclusion proof), and that every tree it
// has seen extends the earlier ones (a consistency proof), so a log
// cannot show one history to one client and another to the next.
//
// The package holds the tree arithmetic (this file), signed tree heads
// and keys (note.go), a client (client.go), and a server (server.go)
// that tests and small deployments run.
package sumdb

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"math/bits"
)

// Hash is a node of the Merkle tree: SHA-256.
type Hash [sha256.Size]byte

// String is the standard base64 of the hash (signed notes and proofs).
func (h Hash) String() string { return base64.StdEncoding.EncodeToString(h[:]) }

// ParseHash parses a base64 hash.
func ParseHash(s string) (Hash, error) {
	var h Hash
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil || len(b) != len(h) {
		return h, fmt.Errorf("malformed hash %q", s)
	}
	copy(h[:], b)
	return h, nil
}

// RecordHash is the leaf hash of a record: SHA-256(0x00 || data)
// (RFC 6962 2.1).
func RecordHash(data []byte) Hash {
	h := sha256.New()
	h.Write([]byte{0})
	h.Write(data)
	var out Hash
	h.Sum(out[:0])
	return out
}

// NodeHash is the hash of an inner node: SHA-256(0x01 || left || right).
func NodeHash(left, right Hash) Hash {
	h := sha256.New()
	h.Write([]byte{1})
	h.Write(left[:])
	h.Write(right[:])
	var out Hash
	h.Sum(out[:0])
	return out
}

// emptyHash is the root of the empty tree: SHA-256 of nothing.
var emptyHash = Hash(sha256.Sum256(nil))

// split is the largest power of two below n (n >= 2).
func split(n int64) int64 {
	return int64(1) << (bits.Len64(uint64(n-1)) - 1)
}

// TreeHash is the Merkle tree hash of the leaves (RFC 6962 2.1, MTH).
func TreeHash(leaves []Hash) Hash {
	switch len(leaves) {
	case 0:
		return emptyHash
	case 1:
		return leaves[0]
	}
	k := split(int64(len(leaves)))
	return NodeHash(TreeHash(leaves[:k]), TreeHash(leaves[k:]))
}

// InclusionProof is the audit path of leaf i in the tree of the leaves
// (RFC 6962 2.1.1, PATH).
func InclusionProof(leaves []Hash, i int64) []Hash {
	n := int64(len(leaves))
	if n <= 1 {
		return nil
	}
	k := split(n)
	if i < k {
		return append(InclusionProof(leaves[:k], i), TreeHash(leaves[k:]))
	}
	return append(InclusionProof(leaves[k:], i-k), TreeHash(leaves[:k]))
}

// ConsistencyProof proves that the tree of the first m leaves is a prefix
// of the tree of all of them (RFC 6962 2.1.2, PROOF).
func ConsistencyProof(leaves []Hash, m int64) []Hash {
	if m <= 0 || m >= int64(len(leaves)) {
		return nil
	}
	return subproof(leaves, m, true)
}

func subproof(leaves []Hash, m int64, whole bool) []Hash {
	n := int64(len(leaves))
	if m == n {
		if whole {
			return nil
		}
		return []Hash{TreeHash(leaves)}
	}
	k := split(n)
	if m <= k {
		return append(subproof(leaves[:k], m, whole), TreeHash(leaves[k:]))
	}
	return append(subproof(leaves[k:], m-k, false), TreeHash(leaves[:k]))
}

// ErrProof is returned when a proof does not verify.
var ErrProof = errors.New("proof does not verify")

// VerifyInclusion checks that leaf is the i-th leaf of the tree of size n
// whose hash is root (RFC 9162 2.1.3.2).
func VerifyInclusion(proof []Hash, n, i int64, leaf, root Hash) error {
	if i < 0 || i >= n {
		return fmt.Errorf("record %d is not in a tree of %d records", i, n)
	}
	fn, sn := uint64(i), uint64(n-1)
	r := leaf
	for _, p := range proof {
		if sn == 0 {
			return ErrProof
		}
		if fn&1 == 1 || fn == sn {
			r = NodeHash(p, r)
			for fn&1 == 0 && fn != 0 {
				fn >>= 1
				sn >>= 1
			}
		} else {
			r = NodeHash(r, p)
		}
		fn >>= 1
		sn >>= 1
	}
	if sn != 0 || r != root {
		return ErrProof
	}
	return nil
}

// VerifyConsistency checks that the tree of size m with hash oldRoot is a
// prefix of the tree of size n with hash newRoot (RFC 9162 2.1.4.2).
func VerifyConsistency(proof []Hash, m, n int64, oldRoot, newRoot Hash) error {
	switch {
	case m < 0 || m > n:
		return fmt.Errorf("a tree of %d records cannot extend one of %d", n, m)
	case m == n:
		if len(proof) != 0 || oldRoot != newRoot {
			return ErrProof
		}
		return nil
	case m == 0:
		// the empty tree is a prefix of every tree
		if len(proof) != 0 {
			return ErrProof
		}
		return nil
	}
	if len(proof) == 0 {
		return ErrProof
	}
	path := proof
	if m&(m-1) == 0 {
		// m is a power of two: the old root is a node of the new tree
		path = append([]Hash{oldRoot}, proof...)
	}
	fn, sn := uint64(m-1), uint64(n-1)
	for fn&1 == 1 {
		fn >>= 1
		sn >>= 1
	}
	fr, sr := path[0], path[0]
	for _, c := range path[1:] {
		if sn == 0 {
			return ErrProof
		}
		if fn&1 == 1 || fn == sn {
			fr = NodeHash(c, fr)
			sr = NodeHash(c, sr)
			for fn&1 == 0 && fn != 0 {
				fn >>= 1
				sn >>= 1
			}
		} else {
			sr = NodeHash(sr, c)
		}
		fn >>= 1
		sn >>= 1
	}
	if sn != 0 || fr != oldRoot || sr != newRoot {
		return ErrProof
	}
	return nil
}
