// Package seed replaces crypto/rand.Reader with a constant stream before
// go-jose captures it: Go initializes packages in order of import path
// once their imports are, and "a/jweoracle/seed" sorts before
// "github.com/go-jose/go-jose/v4", which keeps rand.Reader in an unexported
// variable (symmetric.go randReader) when it initializes. With
// cryptocustomrand=1 (main.go) RSA-OAEP and ECDSA key generation read it
// too, so every regeneration writes the same tokens (stdcorpus -check).
package seed

import crand "crypto/rand"

type constReader byte

func (c constReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(c)
	}
	return len(p), nil
}

func init() { crand.Reader = constReader(0x5a) }
