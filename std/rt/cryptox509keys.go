//go:build !xo_no_crypto

// Copyright 2011 The Go Authors. All rights reserved. Use of this source code is
// governed by a BSD-style license (std/LICENSE).

// cryptox509keys.go is Go 1.26's key encodings from crypto/x509
// (pkcs1.go, pkcs8.go, sec1.go, and the PKIX public key code of x509.go
// and parser.go), so that std/crypto's keys (cryptokeys.go) do not link
// crypto/x509 and what it brings (certificate parsing and verification,
// the system roots; task #516). The exported functions carry an x5
// prefix, and TestX509KeysMatchGo checks them against crypto/x509 on
// every key type and on corrupted encodings, errors included. Changes
// from Go: GODEBUG=x509rsacrt=0 is read with x5godebug, and
// parsePublicKey reads with x5der, the part of
// golang.org/x/crypto/cryptobyte it uses.
package rt

import (
	"bytes"
	"crypto/dsa"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/x509/pkix"
	"encoding/asn1"
	"errors"
	"fmt"
	"math/big"
	"os"
	"strings"
)

// x5godebug is the value of the GODEBUG setting key, read from the
// environment at each call as internal/godebug sees it (the last
// key=value wins).
func x5godebug(key string) string {
	v := ""
	for _, kv := range strings.Split(os.Getenv("GODEBUG"), ",") {
		if k, val, ok := strings.Cut(kv, "="); ok && k == key {
			v = val
		}
	}
	return v
}

// x5der is golang.org/x/crypto/cryptobyte's String with the readers
// parsePublicKey uses (cryptobyte/asn1.go and string.go).
type x5der []byte

const (
	x5tagInteger  = 2
	x5tagOID      = 6
	x5tagSequence = 0x30
)

func (s *x5der) read(n int) []byte {
	if len(*s) < n || n < 0 {
		return nil
	}
	v := (*s)[:n]
	*s = (*s)[n:]
	return v
}

// readASN1 is cryptobyte's ReadASN1: the contents of a DER element with
// the tag, in low-tag-number form and minimal definite length.
func (s *x5der) readASN1(out *x5der, tag byte) bool {
	if len(*s) < 2 {
		return false
	}
	t, lenByte := (*s)[0], (*s)[1]
	if t&0x1f == 0x1f {
		return false
	}
	var length, headerLen uint32
	if lenByte&0x80 == 0 {
		length = uint32(lenByte) + 2
		headerLen = 2
	} else {
		lenLen := lenByte & 0x7f
		if lenLen == 0 || lenLen > 4 || len(*s) < int(2+lenLen) {
			return false
		}
		var len32 uint32
		for _, b := range (*s)[2 : 2+lenLen] {
			len32 = len32<<8 | uint32(b)
		}
		if len32 < 128 || len32>>((lenLen-1)*8) == 0 {
			return false
		}
		headerLen = 2 + uint32(lenLen)
		if headerLen+len32 < len32 {
			return false
		}
		length = headerLen + len32
	}
	if int(length) < 0 {
		return false
	}
	v := s.read(int(length))
	if v == nil {
		return false
	}
	if t != tag {
		return false
	}
	*out = x5der(v[headerLen:])
	return true
}

func x5checkInteger(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	if len(b) == 1 {
		return true
	}
	return !(b[0] == 0 && b[1]&0x80 == 0 || b[0] == 0xff && b[1]&0x80 == 0x80)
}

func (s *x5der) readBigInt(out *big.Int) bool {
	var b x5der
	if !s.readASN1(&b, x5tagInteger) || !x5checkInteger(b) {
		return false
	}
	if b[0]&0x80 == 0x80 {
		neg := make([]byte, len(b))
		for i, c := range b {
			neg[i] = ^c
		}
		out.SetBytes(neg)
		out.Add(out, big.NewInt(1))
		out.Neg(out)
	} else {
		out.SetBytes(b)
	}
	return true
}

// readInt is cryptobyte's ReadASN1Integer into an *int (64 bit).
func (s *x5der) readInt(out *int) bool {
	var b x5der
	if !s.readASN1(&b, x5tagInteger) || !x5checkInteger(b) || len(b) > 8 {
		return false
	}
	var v int64
	for _, c := range b {
		v = v<<8 | int64(c)
	}
	v <<= 64 - uint8(len(b))*8
	v >>= 64 - uint8(len(b))*8
	*out = int(v)
	return true
}

func (s *x5der) readBase128Int(out *int) bool {
	ret := 0
	for i := 0; len(*s) > 0; i++ {
		if i == 5 || ret >= 1<<(31-7) {
			return false
		}
		ret <<= 7
		b := s.read(1)[0]
		if i == 0 && b == 0x80 {
			return false
		}
		ret |= int(b & 0x7f)
		if b&0x80 == 0 {
			*out = ret
			return true
		}
	}
	return false
}

func (s *x5der) readOID(out *asn1.ObjectIdentifier) bool {
	var b x5der
	if !s.readASN1(&b, x5tagOID) || len(b) == 0 {
		return false
	}
	components := make([]int, len(b)+1)
	var v int
	if !b.readBase128Int(&v) {
		return false
	}
	if v < 80 {
		components[0] = v / 40
		components[1] = v % 40
	} else {
		components[0] = 2
		components[1] = v - 80
	}
	i := 2
	for ; len(b) > 0; i++ {
		if !b.readBase128Int(&v) {
			return false
		}
		components[i] = v
	}
	*out = components[:i]
	return true
}

// parsePublicKey is parser.go's, read with x5der.
func parsePublicKey(keyData *publicKeyInfo) (any, error) {
	oid := keyData.Algorithm.Algorithm
	params := keyData.Algorithm.Parameters
	data := keyData.PublicKey.RightAlign()
	switch {
	case oid.Equal(oidPublicKeyRSA):
		// RSA public keys must have a NULL in the parameters.
		// See RFC 3279, Section 2.3.1.
		if !bytes.Equal(params.FullBytes, asn1.NullBytes) {
			return nil, errors.New("x509: RSA key missing NULL parameters")
		}
		der := x5der(data)
		p := &pkcs1PublicKey{N: new(big.Int)}
		if !der.readASN1(&der, x5tagSequence) {
			return nil, errors.New("x509: invalid RSA public key")
		}
		if !der.readBigInt(p.N) {
			return nil, errors.New("x509: invalid RSA modulus")
		}
		if !der.readInt(&p.E) {
			return nil, errors.New("x509: invalid RSA public exponent")
		}
		if p.N.Sign() <= 0 {
			return nil, errors.New("x509: RSA modulus is not a positive number")
		}
		if p.E <= 0 {
			return nil, errors.New("x509: RSA public exponent is not a positive number")
		}
		return &rsa.PublicKey{E: p.E, N: p.N}, nil
	case oid.Equal(oidPublicKeyECDSA):
		paramsDer := x5der(params.FullBytes)
		namedCurveOID := new(asn1.ObjectIdentifier)
		if !paramsDer.readOID(namedCurveOID) {
			return nil, errors.New("x509: invalid ECDSA parameters")
		}
		namedCurve := namedCurveFromOID(*namedCurveOID)
		if namedCurve == nil {
			return nil, errors.New("x509: unsupported elliptic curve")
		}
		return ecdsa.ParseUncompressedPublicKey(namedCurve, data)
	case oid.Equal(oidPublicKeyEd25519):
		// RFC 8410, Section 3
		// > For all of the OIDs, the parameters MUST be absent.
		if len(params.FullBytes) != 0 {
			return nil, errors.New("x509: Ed25519 key encoded with illegal parameters")
		}
		if len(data) != ed25519.PublicKeySize {
			return nil, errors.New("x509: wrong Ed25519 public key size")
		}
		return ed25519.PublicKey(data), nil
	case oid.Equal(oidPublicKeyX25519):
		if len(params.FullBytes) != 0 {
			return nil, errors.New("x509: X25519 key encoded with illegal parameters")
		}
		return ecdh.X25519().NewPublicKey(data)
	case oid.Equal(oidPublicKeyDSA):
		der := x5der(data)
		y := new(big.Int)
		if !der.readBigInt(y) {
			return nil, errors.New("x509: invalid DSA public key")
		}
		pub := &dsa.PublicKey{
			Y: y,
			Parameters: dsa.Parameters{
				P: new(big.Int),
				Q: new(big.Int),
				G: new(big.Int),
			},
		}
		paramsDer := x5der(params.FullBytes)
		if !paramsDer.readASN1(&paramsDer, x5tagSequence) ||
			!paramsDer.readBigInt(pub.Parameters.P) ||
			!paramsDer.readBigInt(pub.Parameters.Q) ||
			!paramsDer.readBigInt(pub.Parameters.G) {
			return nil, errors.New("x509: invalid DSA parameters")
		}
		if pub.Y.Sign() <= 0 || pub.Parameters.P.Sign() <= 0 ||
			pub.Parameters.Q.Sign() <= 0 || pub.Parameters.G.Sign() <= 0 {
			return nil, errors.New("x509: zero or negative DSA parameter")
		}
		return pub, nil
	default:
		return nil, errors.New("x509: unknown public key algorithm")
	}
}

// pkcs1PrivateKey is a structure which mirrors the PKCS #1 ASN.1 for an RSA private key.
type pkcs1PrivateKey struct {
	Version int
	N       *big.Int
	E       int
	D       *big.Int
	P       *big.Int
	Q       *big.Int
	Dp      *big.Int `asn1:"optional"`
	Dq      *big.Int `asn1:"optional"`
	Qinv    *big.Int `asn1:"optional"`

	AdditionalPrimes []pkcs1AdditionalRSAPrime `asn1:"optional,omitempty"`
}

type pkcs1AdditionalRSAPrime struct {
	Prime *big.Int

	// We ignore these values because rsa will calculate them.
	Exp   *big.Int
	Coeff *big.Int
}

// pkcs1PublicKey reflects the ASN.1 structure of a PKCS #1 public key.
type pkcs1PublicKey struct {
	N *big.Int
	E int
}

// x509rsacrt, if zero, makes ParsePKCS1PrivateKey ignore and recompute invalid
// CRT values in the RSA private key.

// ParsePKCS1PrivateKey parses an [RSA] private key in PKCS #1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "RSA PRIVATE KEY".
//
// Before Go 1.24, the CRT parameters were ignored and recomputed. To restore
// the old behavior, use the GODEBUG=x509rsacrt=0 environment variable.
func x5ParsePKCS1PrivateKey(der []byte) (*rsa.PrivateKey, error) {
	var priv pkcs1PrivateKey
	rest, err := asn1.Unmarshal(der, &priv)
	if len(rest) > 0 {
		return nil, asn1.SyntaxError{Msg: "trailing data"}
	}
	if err != nil {
		if _, err := asn1.Unmarshal(der, &ecPrivateKey{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParseECPrivateKey instead for this key format)")
		}
		if _, err := asn1.Unmarshal(der, &pkcs8{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS8PrivateKey instead for this key format)")
		}
		return nil, err
	}

	if priv.Version > 1 {
		return nil, errors.New("x509: unsupported private key version")
	}

	if priv.N.Sign() <= 0 || priv.D.Sign() <= 0 || priv.P.Sign() <= 0 || priv.Q.Sign() <= 0 ||
		priv.Dp != nil && priv.Dp.Sign() <= 0 ||
		priv.Dq != nil && priv.Dq.Sign() <= 0 ||
		priv.Qinv != nil && priv.Qinv.Sign() <= 0 {
		return nil, errors.New("x509: private key contains zero or negative value")
	}

	key := new(rsa.PrivateKey)
	key.PublicKey = rsa.PublicKey{
		E: priv.E,
		N: priv.N,
	}

	key.D = priv.D
	key.Primes = make([]*big.Int, 2+len(priv.AdditionalPrimes))
	key.Primes[0] = priv.P
	key.Primes[1] = priv.Q
	key.Precomputed.Dp = priv.Dp
	key.Precomputed.Dq = priv.Dq
	key.Precomputed.Qinv = priv.Qinv
	for i, a := range priv.AdditionalPrimes {
		if a.Prime.Sign() <= 0 {
			return nil, errors.New("x509: private key contains zero or negative prime")
		}
		key.Primes[i+2] = a.Prime
		// We ignore the other two values because rsa will calculate
		// them as needed.
	}

	key.Precompute()
	if err := key.Validate(); err != nil {
		// If x509rsacrt=0 is set, try dropping the CRT values and
		// rerunning precomputation and key validation.
		if x5godebug("x509rsacrt") == "0" {
			key.Precomputed.Dp = nil
			key.Precomputed.Dq = nil
			key.Precomputed.Qinv = nil
			key.Precompute()
			if err := key.Validate(); err == nil {
				return key, nil
			}
		}

		return nil, err
	}

	return key, nil
}

// MarshalPKCS1PrivateKey converts an [RSA] private key to PKCS #1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "RSA PRIVATE KEY".
// For a more flexible key format which is not [RSA] specific, use
// [MarshalPKCS8PrivateKey].
//
// The key must have passed validation by calling [rsa.PrivateKey.Validate]
// first. MarshalPKCS1PrivateKey calls [rsa.PrivateKey.Precompute], which may
// modify the key if not already precomputed.
func x5MarshalPKCS1PrivateKey(key *rsa.PrivateKey) []byte {
	key.Precompute()

	version := 0
	if len(key.Primes) > 2 {
		version = 1
	}

	priv := pkcs1PrivateKey{
		Version: version,
		N:       key.N,
		E:       key.PublicKey.E,
		D:       key.D,
		P:       key.Primes[0],
		Q:       key.Primes[1],
		Dp:      key.Precomputed.Dp,
		Dq:      key.Precomputed.Dq,
		Qinv:    key.Precomputed.Qinv,
	}

	priv.AdditionalPrimes = make([]pkcs1AdditionalRSAPrime, len(key.Precomputed.CRTValues))
	for i, values := range key.Precomputed.CRTValues {
		priv.AdditionalPrimes[i].Prime = key.Primes[2+i]
		priv.AdditionalPrimes[i].Exp = values.Exp
		priv.AdditionalPrimes[i].Coeff = values.Coeff
	}

	b, _ := asn1.Marshal(priv)
	return b
}

// ParsePKCS1PublicKey parses an [RSA] public key in PKCS #1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "RSA PUBLIC KEY".
func x5ParsePKCS1PublicKey(der []byte) (*rsa.PublicKey, error) {
	var pub pkcs1PublicKey
	rest, err := asn1.Unmarshal(der, &pub)
	if err != nil {
		if _, err := asn1.Unmarshal(der, &publicKeyInfo{}); err == nil {
			return nil, errors.New("x509: failed to parse public key (use ParsePKIXPublicKey instead for this key format)")
		}
		return nil, err
	}
	if len(rest) > 0 {
		return nil, asn1.SyntaxError{Msg: "trailing data"}
	}

	if pub.N.Sign() <= 0 || pub.E <= 0 {
		return nil, errors.New("x509: public key contains zero or negative value")
	}
	if pub.E > 1<<31-1 {
		return nil, errors.New("x509: public key contains large public exponent")
	}

	return &rsa.PublicKey{
		E: pub.E,
		N: pub.N,
	}, nil
}

// MarshalPKCS1PublicKey converts an [RSA] public key to PKCS #1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "RSA PUBLIC KEY".
func x5MarshalPKCS1PublicKey(key *rsa.PublicKey) []byte {
	derBytes, _ := asn1.Marshal(pkcs1PublicKey{
		N: key.N,
		E: key.E,
	})
	return derBytes
}

// pkcs8 reflects an ASN.1, PKCS #8 PrivateKey. See
// ftp://ftp.rsasecurity.com/pub/pkcs/pkcs-8/pkcs-8v1_2.asn
// and RFC 5208.
type pkcs8 struct {
	Version    int
	Algo       pkix.AlgorithmIdentifier
	PrivateKey []byte
	// optional attributes omitted.
}

// ParsePKCS8PrivateKey parses an unencrypted private key in PKCS #8, ASN.1 DER form.
//
// It returns a *[rsa.PrivateKey], an *[ecdsa.PrivateKey], an [ed25519.PrivateKey] (not
// a pointer), or an *[ecdh.PrivateKey] (for X25519). More types might be supported
// in the future.
//
// This kind of key is commonly encoded in PEM blocks of type "PRIVATE KEY".
//
// Before Go 1.24, the CRT parameters of RSA keys were ignored and recomputed.
// To restore the old behavior, use the GODEBUG=x509rsacrt=0 environment variable.
func x5ParsePKCS8PrivateKey(der []byte) (key any, err error) {
	var privKey pkcs8
	if _, err := asn1.Unmarshal(der, &privKey); err != nil {
		if _, err := asn1.Unmarshal(der, &ecPrivateKey{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParseECPrivateKey instead for this key format)")
		}
		if _, err := asn1.Unmarshal(der, &pkcs1PrivateKey{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS1PrivateKey instead for this key format)")
		}
		return nil, err
	}
	switch {
	case privKey.Algo.Algorithm.Equal(oidPublicKeyRSA):
		key, err = x5ParsePKCS1PrivateKey(privKey.PrivateKey)
		if err != nil {
			return nil, errors.New("x509: failed to parse RSA private key embedded in PKCS#8: " + err.Error())
		}
		return key, nil

	case privKey.Algo.Algorithm.Equal(oidPublicKeyECDSA):
		bytes := privKey.Algo.Parameters.FullBytes
		namedCurveOID := new(asn1.ObjectIdentifier)
		if _, err := asn1.Unmarshal(bytes, namedCurveOID); err != nil {
			namedCurveOID = nil
		}
		key, err = parseECPrivateKey(namedCurveOID, privKey.PrivateKey)
		if err != nil {
			return nil, errors.New("x509: failed to parse EC private key embedded in PKCS#8: " + err.Error())
		}
		return key, nil

	case privKey.Algo.Algorithm.Equal(oidPublicKeyEd25519):
		if l := len(privKey.Algo.Parameters.FullBytes); l != 0 {
			return nil, errors.New("x509: invalid Ed25519 private key parameters")
		}
		var curvePrivateKey []byte
		if _, err := asn1.Unmarshal(privKey.PrivateKey, &curvePrivateKey); err != nil {
			return nil, fmt.Errorf("x509: invalid Ed25519 private key: %v", err)
		}
		if l := len(curvePrivateKey); l != ed25519.SeedSize {
			return nil, fmt.Errorf("x509: invalid Ed25519 private key length: %d", l)
		}
		return ed25519.NewKeyFromSeed(curvePrivateKey), nil

	case privKey.Algo.Algorithm.Equal(oidPublicKeyX25519):
		if l := len(privKey.Algo.Parameters.FullBytes); l != 0 {
			return nil, errors.New("x509: invalid X25519 private key parameters")
		}
		var curvePrivateKey []byte
		if _, err := asn1.Unmarshal(privKey.PrivateKey, &curvePrivateKey); err != nil {
			return nil, fmt.Errorf("x509: invalid X25519 private key: %v", err)
		}
		return ecdh.X25519().NewPrivateKey(curvePrivateKey)

	default:
		return nil, fmt.Errorf("x509: PKCS#8 wrapping contained private key with unknown algorithm: %v", privKey.Algo.Algorithm)
	}
}

// MarshalPKCS8PrivateKey converts a private key to PKCS #8, ASN.1 DER form.
//
// The following key types are currently supported: *[rsa.PrivateKey],
// *[ecdsa.PrivateKey], [ed25519.PrivateKey] (not a pointer), and *[ecdh.PrivateKey].
// Unsupported key types result in an error.
//
// This kind of key is commonly encoded in PEM blocks of type "PRIVATE KEY".
//
// MarshalPKCS8PrivateKey runs [rsa.PrivateKey.Precompute] on RSA keys.
func x5MarshalPKCS8PrivateKey(key any) ([]byte, error) {
	var privKey pkcs8

	switch k := key.(type) {
	case *rsa.PrivateKey:
		privKey.Algo = pkix.AlgorithmIdentifier{
			Algorithm:  oidPublicKeyRSA,
			Parameters: asn1.NullRawValue,
		}
		k.Precompute()
		if err := k.Validate(); err != nil {
			return nil, err
		}
		privKey.PrivateKey = x5MarshalPKCS1PrivateKey(k)

	case *ecdsa.PrivateKey:
		oid, ok := oidFromNamedCurve(k.Curve)
		if !ok {
			return nil, errors.New("x509: unknown curve while marshaling to PKCS#8")
		}
		oidBytes, err := asn1.Marshal(oid)
		if err != nil {
			return nil, errors.New("x509: failed to marshal curve OID: " + err.Error())
		}
		privKey.Algo = pkix.AlgorithmIdentifier{
			Algorithm: oidPublicKeyECDSA,
			Parameters: asn1.RawValue{
				FullBytes: oidBytes,
			},
		}
		if privKey.PrivateKey, err = marshalECPrivateKeyWithOID(k, nil); err != nil {
			return nil, errors.New("x509: failed to marshal EC private key while building PKCS#8: " + err.Error())
		}

	case ed25519.PrivateKey:
		privKey.Algo = pkix.AlgorithmIdentifier{
			Algorithm: oidPublicKeyEd25519,
		}
		curvePrivateKey, err := asn1.Marshal(k.Seed())
		if err != nil {
			return nil, fmt.Errorf("x509: failed to marshal private key: %v", err)
		}
		privKey.PrivateKey = curvePrivateKey

	case *ecdh.PrivateKey:
		if k.Curve() == ecdh.X25519() {
			privKey.Algo = pkix.AlgorithmIdentifier{
				Algorithm: oidPublicKeyX25519,
			}
			var err error
			if privKey.PrivateKey, err = asn1.Marshal(k.Bytes()); err != nil {
				return nil, fmt.Errorf("x509: failed to marshal private key: %v", err)
			}
		} else {
			oid, ok := oidFromECDHCurve(k.Curve())
			if !ok {
				return nil, errors.New("x509: unknown curve while marshaling to PKCS#8")
			}
			oidBytes, err := asn1.Marshal(oid)
			if err != nil {
				return nil, errors.New("x509: failed to marshal curve OID: " + err.Error())
			}
			privKey.Algo = pkix.AlgorithmIdentifier{
				Algorithm: oidPublicKeyECDSA,
				Parameters: asn1.RawValue{
					FullBytes: oidBytes,
				},
			}
			if privKey.PrivateKey, err = marshalECDHPrivateKey(k); err != nil {
				return nil, errors.New("x509: failed to marshal EC private key while building PKCS#8: " + err.Error())
			}
		}

	default:
		return nil, fmt.Errorf("x509: unknown key type while marshaling PKCS#8: %T", key)
	}

	return asn1.Marshal(privKey)
}

const ecPrivKeyVersion = 1

// ecPrivateKey reflects an ASN.1 Elliptic Curve Private Key Structure.
// References:
//
//	RFC 5915
//	SEC1 - http://www.secg.org/sec1-v2.pdf
//
// Per RFC 5915 the NamedCurveOID is marked as ASN.1 OPTIONAL, however in
// most cases it is not.
type ecPrivateKey struct {
	Version       int
	PrivateKey    []byte
	NamedCurveOID asn1.ObjectIdentifier `asn1:"optional,explicit,tag:0"`
	PublicKey     asn1.BitString        `asn1:"optional,explicit,tag:1"`
}

// ParseECPrivateKey parses an EC private key in SEC 1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "EC PRIVATE KEY".
func x5ParseECPrivateKey(der []byte) (*ecdsa.PrivateKey, error) {
	return parseECPrivateKey(nil, der)
}

// MarshalECPrivateKey converts an EC private key to SEC 1, ASN.1 DER form.
//
// This kind of key is commonly encoded in PEM blocks of type "EC PRIVATE KEY".
// For a more flexible key format which is not EC specific, use
// [MarshalPKCS8PrivateKey].
func x5MarshalECPrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	oid, ok := oidFromNamedCurve(key.Curve)
	if !ok {
		return nil, errors.New("x509: unknown elliptic curve")
	}

	return marshalECPrivateKeyWithOID(key, oid)
}

// marshalECPrivateKeyWithOID marshals an EC private key into ASN.1, DER format and
// sets the curve ID to the given OID, or omits it if OID is nil.
func marshalECPrivateKeyWithOID(key *ecdsa.PrivateKey, oid asn1.ObjectIdentifier) ([]byte, error) {
	privateKey, err := key.Bytes()
	if err != nil {
		return nil, err
	}
	publicKey, err := key.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	return asn1.Marshal(ecPrivateKey{
		Version:       1,
		PrivateKey:    privateKey,
		NamedCurveOID: oid,
		PublicKey:     asn1.BitString{Bytes: publicKey},
	})
}

// marshalECDHPrivateKey marshals an EC private key into ASN.1, DER format
// suitable for NIST curves.
func marshalECDHPrivateKey(key *ecdh.PrivateKey) ([]byte, error) {
	return asn1.Marshal(ecPrivateKey{
		Version:    1,
		PrivateKey: key.Bytes(),
		PublicKey:  asn1.BitString{Bytes: key.PublicKey().Bytes()},
	})
}

// parseECPrivateKey parses an ASN.1 Elliptic Curve Private Key Structure.
// The OID for the named curve may be provided from another source (such as
// the PKCS8 container) - if it is provided then use this instead of the OID
// that may exist in the EC private key structure.
func parseECPrivateKey(namedCurveOID *asn1.ObjectIdentifier, der []byte) (key *ecdsa.PrivateKey, err error) {
	var privKey ecPrivateKey
	if _, err := asn1.Unmarshal(der, &privKey); err != nil {
		if _, err := asn1.Unmarshal(der, &pkcs8{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS8PrivateKey instead for this key format)")
		}
		if _, err := asn1.Unmarshal(der, &pkcs1PrivateKey{}); err == nil {
			return nil, errors.New("x509: failed to parse private key (use ParsePKCS1PrivateKey instead for this key format)")
		}
		return nil, errors.New("x509: failed to parse EC private key: " + err.Error())
	}
	if privKey.Version != ecPrivKeyVersion {
		return nil, fmt.Errorf("x509: unknown EC private key version %d", privKey.Version)
	}

	var curve elliptic.Curve
	if namedCurveOID != nil {
		curve = namedCurveFromOID(*namedCurveOID)
	} else {
		curve = namedCurveFromOID(privKey.NamedCurveOID)
	}
	if curve == nil {
		return nil, errors.New("x509: unknown elliptic curve")
	}

	size := (curve.Params().N.BitLen() + 7) / 8
	privateKey := make([]byte, size)

	// Some private keys have leading zero padding. This is invalid
	// according to [SEC1], but this code will ignore it.
	for len(privKey.PrivateKey) > len(privateKey) {
		if privKey.PrivateKey[0] != 0 {
			return nil, errors.New("x509: invalid private key length")
		}
		privKey.PrivateKey = privKey.PrivateKey[1:]
	}

	// Some private keys remove all leading zeros, this is also invalid
	// according to [SEC1] but since OpenSSL used to do this, we ignore
	// this too.
	copy(privateKey[len(privateKey)-len(privKey.PrivateKey):], privKey.PrivateKey)

	return ecdsa.ParseRawPrivateKey(curve, privateKey)
}

// pkixPublicKey reflects a PKIX public key structure. See SubjectPublicKeyInfo
// in RFC 3280.
type pkixPublicKey struct {
	Algo      pkix.AlgorithmIdentifier
	BitString asn1.BitString
}

// ParsePKIXPublicKey parses a public key in PKIX, ASN.1 DER form. The encoded
// public key is a SubjectPublicKeyInfo structure (see RFC 5280, Section 4.1).
//
// It returns a *[rsa.PublicKey], *[dsa.PublicKey], *[ecdsa.PublicKey],
// [ed25519.PublicKey] (not a pointer), or *[ecdh.PublicKey] (for X25519).
// More types might be supported in the future.
//
// This kind of key is commonly encoded in PEM blocks of type "PUBLIC KEY".
func x5ParsePKIXPublicKey(derBytes []byte) (pub any, err error) {
	var pki publicKeyInfo
	if rest, err := asn1.Unmarshal(derBytes, &pki); err != nil {
		if _, err := asn1.Unmarshal(derBytes, &pkcs1PublicKey{}); err == nil {
			return nil, errors.New("x509: failed to parse public key (use ParsePKCS1PublicKey instead for this key format)")
		}
		return nil, err
	} else if len(rest) != 0 {
		return nil, errors.New("x509: trailing data after ASN.1 of public-key")
	}
	return parsePublicKey(&pki)
}

func marshalPublicKey(pub any) (publicKeyBytes []byte, publicKeyAlgorithm pkix.AlgorithmIdentifier, err error) {
	switch pub := pub.(type) {
	case *rsa.PublicKey:
		publicKeyBytes, err = asn1.Marshal(pkcs1PublicKey{
			N: pub.N,
			E: pub.E,
		})
		if err != nil {
			return nil, pkix.AlgorithmIdentifier{}, err
		}
		publicKeyAlgorithm.Algorithm = oidPublicKeyRSA
		// This is a NULL parameters value which is required by
		// RFC 3279, Section 2.3.1.
		publicKeyAlgorithm.Parameters = asn1.NullRawValue
	case *ecdsa.PublicKey:
		oid, ok := oidFromNamedCurve(pub.Curve)
		if !ok {
			return nil, pkix.AlgorithmIdentifier{}, errors.New("x509: unsupported elliptic curve")
		}
		publicKeyBytes, err = pub.Bytes()
		if err != nil {
			return nil, pkix.AlgorithmIdentifier{}, err
		}
		publicKeyAlgorithm.Algorithm = oidPublicKeyECDSA
		var paramBytes []byte
		paramBytes, err = asn1.Marshal(oid)
		if err != nil {
			return
		}
		publicKeyAlgorithm.Parameters.FullBytes = paramBytes
	case ed25519.PublicKey:
		publicKeyBytes = pub
		publicKeyAlgorithm.Algorithm = oidPublicKeyEd25519
	case *ecdh.PublicKey:
		publicKeyBytes = pub.Bytes()
		if pub.Curve() == ecdh.X25519() {
			publicKeyAlgorithm.Algorithm = oidPublicKeyX25519
		} else {
			oid, ok := oidFromECDHCurve(pub.Curve())
			if !ok {
				return nil, pkix.AlgorithmIdentifier{}, errors.New("x509: unsupported elliptic curve")
			}
			publicKeyAlgorithm.Algorithm = oidPublicKeyECDSA
			var paramBytes []byte
			paramBytes, err = asn1.Marshal(oid)
			if err != nil {
				return
			}
			publicKeyAlgorithm.Parameters.FullBytes = paramBytes
		}
	default:
		return nil, pkix.AlgorithmIdentifier{}, fmt.Errorf("x509: unsupported public key type: %T", pub)
	}

	return publicKeyBytes, publicKeyAlgorithm, nil
}

// MarshalPKIXPublicKey converts a public key to PKIX, ASN.1 DER form.
// The encoded public key is a SubjectPublicKeyInfo structure
// (see RFC 5280, Section 4.1).
//
// The following key types are currently supported: *[rsa.PublicKey],
// *[ecdsa.PublicKey], [ed25519.PublicKey] (not a pointer), and *[ecdh.PublicKey].
// Unsupported key types result in an error.
//
// This kind of key is commonly encoded in PEM blocks of type "PUBLIC KEY".
func x5MarshalPKIXPublicKey(pub any) ([]byte, error) {
	var publicKeyBytes []byte
	var publicKeyAlgorithm pkix.AlgorithmIdentifier
	var err error

	if publicKeyBytes, publicKeyAlgorithm, err = marshalPublicKey(pub); err != nil {
		return nil, err
	}

	pkix := pkixPublicKey{
		Algo: publicKeyAlgorithm,
		BitString: asn1.BitString{
			Bytes:     publicKeyBytes,
			BitLength: 8 * len(publicKeyBytes),
		},
	}

	ret, _ := asn1.Marshal(pkix)
	return ret, nil
}

type publicKeyInfo struct {
	Raw       asn1.RawContent
	Algorithm pkix.AlgorithmIdentifier
	PublicKey asn1.BitString
}

type dsaAlgorithmParameters struct {
	P, Q, G *big.Int
}

var oidPublicKeyRSA = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 1, 1}

var oidPublicKeyDSA = asn1.ObjectIdentifier{1, 2, 840, 10040, 4, 1}

var oidPublicKeyECDSA = asn1.ObjectIdentifier{1, 2, 840, 10045, 2, 1}

var oidPublicKeyX25519 = asn1.ObjectIdentifier{1, 3, 101, 110}

var oidPublicKeyEd25519 = asn1.ObjectIdentifier{1, 3, 101, 112}

var oidNamedCurveP224 = asn1.ObjectIdentifier{1, 3, 132, 0, 33}

var oidNamedCurveP256 = asn1.ObjectIdentifier{1, 2, 840, 10045, 3, 1, 7}

var oidNamedCurveP384 = asn1.ObjectIdentifier{1, 3, 132, 0, 34}

var oidNamedCurveP521 = asn1.ObjectIdentifier{1, 3, 132, 0, 35}

func namedCurveFromOID(oid asn1.ObjectIdentifier) elliptic.Curve {
	switch {
	case oid.Equal(oidNamedCurveP224):
		return elliptic.P224()
	case oid.Equal(oidNamedCurveP256):
		return elliptic.P256()
	case oid.Equal(oidNamedCurveP384):
		return elliptic.P384()
	case oid.Equal(oidNamedCurveP521):
		return elliptic.P521()
	}
	return nil
}

func oidFromNamedCurve(curve elliptic.Curve) (asn1.ObjectIdentifier, bool) {
	switch curve {
	case elliptic.P224():
		return oidNamedCurveP224, true
	case elliptic.P256():
		return oidNamedCurveP256, true
	case elliptic.P384():
		return oidNamedCurveP384, true
	case elliptic.P521():
		return oidNamedCurveP521, true
	}

	return nil, false
}

func oidFromECDHCurve(curve ecdh.Curve) (asn1.ObjectIdentifier, bool) {
	switch curve {
	case ecdh.X25519():
		return oidPublicKeyX25519, true
	case ecdh.P256():
		return oidNamedCurveP256, true
	case ecdh.P384():
		return oidNamedCurveP384, true
	case ecdh.P521():
		return oidNamedCurveP521, true
	}

	return nil, false
}
