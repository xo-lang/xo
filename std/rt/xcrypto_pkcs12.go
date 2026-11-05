// Vendored from golang.org/x/crypto/pkcs12 (v0.58.0) for the Go backend's x509.parse_pkcs12:
// the package and its internal/rc2 in one file, every top level name prefixed xp12_.
// Copyright 2015 The Go Authors. All rights reserved. Use of this source code is
// governed by a BSD-style license (std/LICENSE).

package rt

import (
	"bytes"
	"crypto/cipher"
	"crypto/des"
	"crypto/ecdsa"
	"crypto/hmac"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"math/bits"
	"unicode/utf16"
)

// ---- bmp-string.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// xp12_bmpString returns s encoded in UCS-2 with a zero terminator.
func xp12_bmpString(s string) ([]byte, error) {
	// References:
	// https://tools.ietf.org/html/rfc7292#appendix-B.1
	// https://en.wikipedia.org/wiki/Plane_(Unicode)#Basic_Multilingual_Plane
	//  - non-BMP characters are encoded in UTF 16 by using a surrogate pair of 16-bit codes
	//	  EncodeRune returns 0xfffd if the rune does not need special encoding
	//  - the above RFC provides the info that BMPStrings are NULL terminated.

	ret := make([]byte, 0, 2*len(s)+2)

	for _, r := range s {
		if t, _ := utf16.EncodeRune(r); t != 0xfffd {
			return nil, errors.New("pkcs12: string contains characters that cannot be encoded in UCS-2")
		}
		ret = append(ret, byte(r/256), byte(r%256))
	}

	return append(ret, 0, 0), nil
}

func xp12_decodeBMPString(xp12_bmpString []byte) (string, error) {
	if len(xp12_bmpString)%2 != 0 {
		return "", errors.New("pkcs12: odd-length BMP string")
	}

	// strip terminator if present
	if l := len(xp12_bmpString); l >= 2 && xp12_bmpString[l-1] == 0 && xp12_bmpString[l-2] == 0 {
		xp12_bmpString = xp12_bmpString[:l-2]
	}

	s := make([]uint16, 0, len(xp12_bmpString)/2)
	for len(xp12_bmpString) > 0 {
		s = append(s, uint16(xp12_bmpString[0])<<8+uint16(xp12_bmpString[1]))
		xp12_bmpString = xp12_bmpString[2:]
	}

	return string(utf16.Decode(s)), nil
}

// ---- crypto.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var (
	xp12_oidPBEWithSHAAnd3KeyTripleDESCBC = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 1, 3})
	xp12_oidPBEWithSHAAnd40BitRC2CBC      = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 1, 6})
)

// xp12_pbeCipher is an abstraction of a PKCS#12 cipher.
type xp12_pbeCipher interface {
	// create returns a cipher.Block given a key.
	create(key []byte) (cipher.Block, error)
	// deriveKey returns a key derived from the given password and salt.
	deriveKey(salt, password []byte, iterations int) []byte
	// deriveIV returns an IV derived from the given password and salt.
	deriveIV(salt, password []byte, iterations int) []byte
}

type xp12_shaWithTripleDESCBC struct{}

func (xp12_shaWithTripleDESCBC) create(key []byte) (cipher.Block, error) {
	return des.NewTripleDESCipher(key)
}

func (xp12_shaWithTripleDESCBC) deriveKey(salt, password []byte, iterations int) []byte {
	return xp12_pbkdf(xp12_sha1Sum, 20, 64, salt, password, iterations, 1, 24)
}

func (xp12_shaWithTripleDESCBC) deriveIV(salt, password []byte, iterations int) []byte {
	return xp12_pbkdf(xp12_sha1Sum, 20, 64, salt, password, iterations, 2, 8)
}

type xp12_shaWith40BitRC2CBC struct{}

func (xp12_shaWith40BitRC2CBC) create(key []byte) (cipher.Block, error) {
	return xp12_New(key, len(key)*8)
}

func (xp12_shaWith40BitRC2CBC) deriveKey(salt, password []byte, iterations int) []byte {
	return xp12_pbkdf(xp12_sha1Sum, 20, 64, salt, password, iterations, 1, 5)
}

func (xp12_shaWith40BitRC2CBC) deriveIV(salt, password []byte, iterations int) []byte {
	return xp12_pbkdf(xp12_sha1Sum, 20, 64, salt, password, iterations, 2, 8)
}

type xp12_pbeParams struct {
	Salt       []byte
	Iterations int
}

func xp12_pbDecrypterFor(algorithm pkix.AlgorithmIdentifier, password []byte) (cipher.BlockMode, int, error) {
	var cipherType xp12_pbeCipher

	switch {
	case algorithm.Algorithm.Equal(xp12_oidPBEWithSHAAnd3KeyTripleDESCBC):
		cipherType = xp12_shaWithTripleDESCBC{}
	case algorithm.Algorithm.Equal(xp12_oidPBEWithSHAAnd40BitRC2CBC):
		cipherType = xp12_shaWith40BitRC2CBC{}
	default:
		return nil, 0, xp12_NotImplementedError("algorithm " + algorithm.Algorithm.String() + " is not supported")
	}

	var params xp12_pbeParams
	if err := xp12_unmarshal(algorithm.Parameters.FullBytes, &params); err != nil {
		return nil, 0, err
	}

	if params.Iterations < 0 || params.Iterations > xp12_maxIterations {
		return nil, 0, xp12_NotImplementedError("iteration count is invalid or too high")
	}

	key := cipherType.deriveKey(params.Salt, password, params.Iterations)
	iv := cipherType.deriveIV(params.Salt, password, params.Iterations)

	block, err := cipherType.create(key)
	if err != nil {
		return nil, 0, err
	}

	return cipher.NewCBCDecrypter(block, iv), block.BlockSize(), nil
}

func xp12_pbDecrypt(info xp12_decryptable, password []byte) (decrypted []byte, err error) {
	cbc, blockSize, err := xp12_pbDecrypterFor(info.Algorithm(), password)
	if err != nil {
		return nil, err
	}

	encrypted := info.Data()
	if len(encrypted) == 0 {
		return nil, errors.New("pkcs12: empty encrypted data")
	}
	if len(encrypted)%blockSize != 0 {
		return nil, errors.New("pkcs12: input is not a multiple of the block size")
	}
	decrypted = make([]byte, len(encrypted))
	cbc.CryptBlocks(decrypted, encrypted)

	psLen := int(decrypted[len(decrypted)-1])
	if psLen == 0 || psLen > blockSize {
		return nil, xp12_ErrDecryption
	}

	if len(decrypted) < psLen {
		return nil, xp12_ErrDecryption
	}
	ps := decrypted[len(decrypted)-psLen:]
	decrypted = decrypted[:len(decrypted)-psLen]
	if !bytes.Equal(ps, bytes.Repeat([]byte{byte(psLen)}, psLen)) {
		return nil, xp12_ErrDecryption
	}

	return
}

// xp12_decryptable abstracts an object that contains ciphertext.
type xp12_decryptable interface {
	Algorithm() pkix.AlgorithmIdentifier
	Data() []byte
}

// ---- errors.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var (
	// xp12_ErrDecryption represents a failure to decrypt the input.
	xp12_ErrDecryption = errors.New("pkcs12: decryption error, incorrect padding")

	// xp12_ErrIncorrectPassword is returned when an incorrect password is detected.
	// Usually, P12/PFX data is signed to be able to verify the password.
	xp12_ErrIncorrectPassword = errors.New("pkcs12: decryption password incorrect")
)

// xp12_NotImplementedError indicates that the input is not currently supported.
type xp12_NotImplementedError string

func (e xp12_NotImplementedError) Error() string {
	return "pkcs12: " + string(e)
}

// ---- mac.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

type xp12_macData struct {
	Mac        xp12_digestInfo
	MacSalt    []byte
	Iterations int `asn1:"optional,default:1"`
}

// from PKCS#7:
type xp12_digestInfo struct {
	Algorithm pkix.AlgorithmIdentifier
	Digest    []byte
}

var (
	xp12_oidSHA1 = asn1.ObjectIdentifier([]int{1, 3, 14, 3, 2, 26})
)

// xp12_maxIterations is a safety limit to prevent CPU exhaustion from
// crafted PKCS#12 files with unreasonable iteration counts.
const xp12_maxIterations = 1 << 20 // ~1 million

func xp12_verifyMac(xp12_macData *xp12_macData, message, password []byte) error {
	if !xp12_macData.Mac.Algorithm.Algorithm.Equal(xp12_oidSHA1) {
		return xp12_NotImplementedError("unknown digest algorithm: " + xp12_macData.Mac.Algorithm.Algorithm.String())
	}

	if xp12_macData.Iterations < 0 || xp12_macData.Iterations > xp12_maxIterations {
		return xp12_NotImplementedError("iteration count is invalid or too high")
	}

	key := xp12_pbkdf(xp12_sha1Sum, 20, 64, xp12_macData.MacSalt, password, xp12_macData.Iterations, 3, 20)

	mac := hmac.New(sha1.New, key)
	mac.Write(message)
	expectedMAC := mac.Sum(nil)

	if !hmac.Equal(xp12_macData.Mac.Digest, expectedMAC) {
		return xp12_ErrIncorrectPassword
	}
	return nil
}

// ---- pbkdf.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var (
	xp12_one = big.NewInt(1)
)

// xp12_sha1Sum returns the SHA-1 hash of in.
func xp12_sha1Sum(in []byte) []byte {
	sum := sha1.Sum(in)
	return sum[:]
}

// xp12_fillWithRepeats returns v*ceiling(len(pattern) / v) bytes consisting of
// repeats of pattern.
func xp12_fillWithRepeats(pattern []byte, v int) []byte {
	if len(pattern) == 0 {
		return nil
	}
	outputLen := v * ((len(pattern) + v - 1) / v)
	return bytes.Repeat(pattern, (outputLen+len(pattern)-1)/len(pattern))[:outputLen]
}

func xp12_pbkdf(hash func([]byte) []byte, u, v int, salt, password []byte, r int, ID byte, size int) (key []byte) {
	// implementation of https://tools.ietf.org/html/rfc7292#appendix-B.2 , RFC text verbatim in comments

	//    Let H be a hash function built around a compression function f:

	//       Z_2^u x Z_2^v -> Z_2^u

	//    (that is, H has a chaining variable and output of length u bits, and
	//    the message input to the compression function of H is v bits).  The
	//    values for u and v are as follows:

	//            HASH FUNCTION     VALUE u        VALUE v
	//              MD2, MD5          128            512
	//                SHA-1           160            512
	//               SHA-224          224            512
	//               SHA-256          256            512
	//               SHA-384          384            1024
	//               SHA-512          512            1024
	//             SHA-512/224        224            1024
	//             SHA-512/256        256            1024

	//    Furthermore, let r be the iteration count.

	//    We assume here that u and v are both multiples of 8, as are the
	//    lengths of the password and salt strings (which we denote by p and s,
	//    respectively) and the number n of pseudorandom bits required.  In
	//    addition, u and v are of course non-zero.

	//    For information on security considerations for MD5 [19], see [25] and
	//    [1], and on those for MD2, see [18].

	//    The following procedure can be used to produce pseudorandom bits for
	//    a particular "purpose" that is identified by a byte called "ID".
	//    This standard specifies 3 different values for the ID byte:

	//    1.  If ID=1, then the pseudorandom bits being produced are to be used
	//        as key material for performing encryption or decryption.

	//    2.  If ID=2, then the pseudorandom bits being produced are to be used
	//        as an IV (Initial Value) for encryption or decryption.

	//    3.  If ID=3, then the pseudorandom bits being produced are to be used
	//        as an integrity key for MACing.

	//    1.  Construct a string, D (the "diversifier"), by concatenating v/8
	//        copies of ID.
	var D []byte
	for i := 0; i < v; i++ {
		D = append(D, ID)
	}

	//    2.  Concatenate copies of the salt together to create a string S of
	//        length v(ceiling(s/v)) bits (the final copy of the salt may be
	//        truncated to create S).  Note that if the salt is the empty
	//        string, then so is S.

	S := xp12_fillWithRepeats(salt, v)

	//    3.  Concatenate copies of the password together to create a string P
	//        of length v(ceiling(p/v)) bits (the final copy of the password
	//        may be truncated to create P).  Note that if the password is the
	//        empty string, then so is P.

	P := xp12_fillWithRepeats(password, v)

	//    4.  Set I=S||P to be the concatenation of S and P.
	I := append(S, P...)

	//    5.  Set c=ceiling(n/u).
	c := (size + u - 1) / u

	//    6.  For i=1, 2, ..., c, do the following:
	A := make([]byte, c*20)
	var IjBuf []byte
	for i := 0; i < c; i++ {
		//        A.  Set A2=H^r(D||I). (i.e., the r-th hash of D||1,
		//            H(H(H(... H(D||I))))
		Ai := hash(append(D, I...))
		for j := 1; j < r; j++ {
			Ai = hash(Ai)
		}
		copy(A[i*20:], Ai[:])

		if i < c-1 { // skip on last iteration
			// B.  Concatenate copies of Ai to create a string B of length v
			//     bits (the final copy of Ai may be truncated to create B).
			var B []byte
			for len(B) < v {
				B = append(B, Ai[:]...)
			}
			B = B[:v]

			// C.  Treating I as a concatenation I_0, I_1, ..., I_(k-1) of v-bit
			//     blocks, where k=ceiling(s/v)+ceiling(p/v), modify I by
			//     setting I_j=(I_j+B+1) mod 2^v for each j.
			{
				Bbi := new(big.Int).SetBytes(B)
				Ij := new(big.Int)

				for j := 0; j < len(I)/v; j++ {
					Ij.SetBytes(I[j*v : (j+1)*v])
					Ij.Add(Ij, Bbi)
					Ij.Add(Ij, xp12_one)
					Ijb := Ij.Bytes()
					// We expect Ijb to be exactly v bytes,
					// if it is longer or shorter we must
					// adjust it accordingly.
					if len(Ijb) > v {
						Ijb = Ijb[len(Ijb)-v:]
					}
					if len(Ijb) < v {
						if IjBuf == nil {
							IjBuf = make([]byte, v)
						}
						bytesShort := v - len(Ijb)
						for i := 0; i < bytesShort; i++ {
							IjBuf[i] = 0
						}
						copy(IjBuf[bytesShort:], Ijb)
						Ijb = IjBuf
					}
					copy(I[j*v:(j+1)*v], Ijb)
				}
			}
		}
	}
	//    7.  Concatenate A_1, A_2, ..., A_c together to form a pseudorandom
	//        bit string, A.

	//    8.  Use the first n bits of A as the output of this entire process.
	return A[:size]

	//    If the above process is being used to generate a DES key, the process
	//    should be used to create 64 random bits, and the key's parity bits
	//    should be set after the 64 bits have been produced.  Similar concerns
	//    hold for 2-key and 3-key triple-DES keys, for CDMF keys, and for any
	//    similar keys with parity bits "built into them".
}

// ---- pkcs12.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package pkcs12 implements some of PKCS#12.
//
// This implementation is distilled from [RFC 7292] and referenced documents.
// It is intended for decoding P12/PFX-stored certificates and keys for use
// with the crypto/tls package.
//
// The pkcs12 package is [frozen] and is not accepting new features.
// If it's missing functionality you need, consider an alternative like
// software.sslmate.com/src/go-pkcs12.
//
// [RFC 7292]: https://datatracker.ietf.org/doc/html/rfc7292
// [frozen]: https://go.dev/wiki/Frozen

var (
	xp12_oidDataContentType          = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 7, 1})
	xp12_oidEncryptedDataContentType = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 7, 6})

	xp12_oidFriendlyName     = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 20})
	xp12_oidLocalKeyID       = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 21})
	xp12_oidMicrosoftCSPName = asn1.ObjectIdentifier([]int{1, 3, 6, 1, 4, 1, 311, 17, 1})

	xp12_errUnknownAttributeOID = errors.New("pkcs12: unknown attribute OID")
)

type xp12_pfxPdu struct {
	Version  int
	AuthSafe xp12_contentInfo
	MacData  xp12_macData `asn1:"optional"`
}

type xp12_contentInfo struct {
	ContentType asn1.ObjectIdentifier
	Content     asn1.RawValue `asn1:"tag:0,explicit,optional"`
}

type xp12_encryptedData struct {
	Version              int
	EncryptedContentInfo xp12_encryptedContentInfo
}

type xp12_encryptedContentInfo struct {
	ContentType                asn1.ObjectIdentifier
	ContentEncryptionAlgorithm pkix.AlgorithmIdentifier
	EncryptedContent           []byte `asn1:"tag:0,optional"`
}

func (i xp12_encryptedContentInfo) Algorithm() pkix.AlgorithmIdentifier {
	return i.ContentEncryptionAlgorithm
}

func (i xp12_encryptedContentInfo) Data() []byte { return i.EncryptedContent }

type xp12_safeBag struct {
	Id         asn1.ObjectIdentifier
	Value      asn1.RawValue          `asn1:"tag:0,explicit"`
	Attributes []xp12_pkcs12Attribute `asn1:"set,optional"`
}

type xp12_pkcs12Attribute struct {
	Id    asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

type xp12_encryptedPrivateKeyInfo struct {
	AlgorithmIdentifier pkix.AlgorithmIdentifier
	EncryptedData       []byte
}

func (i xp12_encryptedPrivateKeyInfo) Algorithm() pkix.AlgorithmIdentifier {
	return i.AlgorithmIdentifier
}

func (i xp12_encryptedPrivateKeyInfo) Data() []byte {
	return i.EncryptedData
}

// PEM block types
const (
	xp12_certificateType = "CERTIFICATE"
	xp12_privateKeyType  = "PRIVATE KEY"
)

// xp12_unmarshal calls asn1.Unmarshal, but also returns an error if there is any
// trailing data after unmarshaling.
func xp12_unmarshal(in []byte, out interface{}) error {
	trailing, err := asn1.Unmarshal(in, out)
	if err != nil {
		return err
	}
	if len(trailing) != 0 {
		return errors.New("pkcs12: trailing data found")
	}
	return nil
}

// xp12_ToPEM converts all "safe bags" contained in pfxData to PEM blocks.
// Unknown attributes are discarded.
//
// Note that although the returned PEM blocks for private keys have type
// "PRIVATE KEY", the bytes are not encoded according to PKCS #8, but according
// to PKCS #1 for RSA keys and SEC 1 for ECDSA keys.
func xp12_ToPEM(pfxData []byte, password string) ([]*pem.Block, error) {
	encodedPassword, err := xp12_bmpString(password)
	if err != nil {
		return nil, xp12_ErrIncorrectPassword
	}

	bags, encodedPassword, err := xp12_getSafeContents(pfxData, encodedPassword)

	if err != nil {
		return nil, err
	}

	blocks := make([]*pem.Block, 0, len(bags))
	for _, bag := range bags {
		block, err := xp12_convertBag(&bag, encodedPassword)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

func xp12_convertBag(bag *xp12_safeBag, password []byte) (*pem.Block, error) {
	block := &pem.Block{
		Headers: make(map[string]string),
	}

	for _, attribute := range bag.Attributes {
		k, v, err := xp12_convertAttribute(&attribute)
		if err == xp12_errUnknownAttributeOID {
			continue
		}
		if err != nil {
			return nil, err
		}
		block.Headers[k] = v
	}

	switch {
	case bag.Id.Equal(xp12_oidCertBag):
		block.Type = xp12_certificateType
		certsData, err := xp12_decodeCertBag(bag.Value.Bytes)
		if err != nil {
			return nil, err
		}
		block.Bytes = certsData
	case bag.Id.Equal(xp12_oidPKCS8ShroundedKeyBag):
		block.Type = xp12_privateKeyType

		key, err := xp12_decodePkcs8ShroudedKeyBag(bag.Value.Bytes, password)
		if err != nil {
			return nil, err
		}

		switch key := key.(type) {
		case *rsa.PrivateKey:
			block.Bytes = x509.MarshalPKCS1PrivateKey(key)
		case *ecdsa.PrivateKey:
			block.Bytes, err = x509.MarshalECPrivateKey(key)
			if err != nil {
				return nil, err
			}
		default:
			return nil, errors.New("found unknown private key type in PKCS#8 wrapping")
		}
	default:
		return nil, errors.New("don't know how to convert a safe bag of type " + bag.Id.String())
	}
	return block, nil
}

func xp12_convertAttribute(attribute *xp12_pkcs12Attribute) (key, value string, err error) {
	isString := false

	switch {
	case attribute.Id.Equal(xp12_oidFriendlyName):
		key = "friendlyName"
		isString = true
	case attribute.Id.Equal(xp12_oidLocalKeyID):
		key = "localKeyId"
	case attribute.Id.Equal(xp12_oidMicrosoftCSPName):
		// This key is chosen to match OpenSSL.
		key = "Microsoft CSP Name"
		isString = true
	default:
		return "", "", xp12_errUnknownAttributeOID
	}

	if isString {
		if err := xp12_unmarshal(attribute.Value.Bytes, &attribute.Value); err != nil {
			return "", "", err
		}
		if value, err = xp12_decodeBMPString(attribute.Value.Bytes); err != nil {
			return "", "", err
		}
	} else {
		var id []byte
		if err := xp12_unmarshal(attribute.Value.Bytes, &id); err != nil {
			return "", "", err
		}
		value = hex.EncodeToString(id)
	}

	return key, value, nil
}

// xp12_Decode extracts a certificate and private key from pfxData. This function
// assumes that there is only xp12_one certificate and only xp12_one private key in the
// pfxData; if there are more use xp12_ToPEM instead.
func xp12_Decode(pfxData []byte, password string) (privateKey interface{}, certificate *x509.Certificate, err error) {
	encodedPassword, err := xp12_bmpString(password)
	if err != nil {
		return nil, nil, err
	}

	bags, encodedPassword, err := xp12_getSafeContents(pfxData, encodedPassword)
	if err != nil {
		return nil, nil, err
	}

	if len(bags) != 2 {
		err = errors.New("pkcs12: expected exactly two safe bags in the PFX PDU")
		return
	}

	for _, bag := range bags {
		switch {
		case bag.Id.Equal(xp12_oidCertBag):
			if certificate != nil {
				err = errors.New("pkcs12: expected exactly xp12_one certificate bag")
			}

			certsData, err := xp12_decodeCertBag(bag.Value.Bytes)
			if err != nil {
				return nil, nil, err
			}
			certs, err := x509.ParseCertificates(certsData)
			if err != nil {
				return nil, nil, err
			}
			if len(certs) != 1 {
				err = errors.New("pkcs12: expected exactly xp12_one certificate in the xp12_certBag")
				return nil, nil, err
			}
			certificate = certs[0]

		case bag.Id.Equal(xp12_oidPKCS8ShroundedKeyBag):
			if privateKey != nil {
				err = errors.New("pkcs12: expected exactly xp12_one key bag")
				return nil, nil, err
			}

			if privateKey, err = xp12_decodePkcs8ShroudedKeyBag(bag.Value.Bytes, encodedPassword); err != nil {
				return nil, nil, err
			}
		}
	}

	if certificate == nil {
		return nil, nil, errors.New("pkcs12: certificate missing")
	}
	if privateKey == nil {
		return nil, nil, errors.New("pkcs12: private key missing")
	}

	return
}

func xp12_getSafeContents(p12Data, password []byte) (bags []xp12_safeBag, updatedPassword []byte, err error) {
	pfx := new(xp12_pfxPdu)
	if err := xp12_unmarshal(p12Data, pfx); err != nil {
		return nil, nil, errors.New("pkcs12: error reading P12 data: " + err.Error())
	}

	if pfx.Version != 3 {
		return nil, nil, xp12_NotImplementedError("can only decode v3 PFX PDU's")
	}

	if !pfx.AuthSafe.ContentType.Equal(xp12_oidDataContentType) {
		return nil, nil, xp12_NotImplementedError("only password-protected PFX is implemented")
	}

	// xp12_unmarshal the explicit bytes in the content for type 'data'
	if err := xp12_unmarshal(pfx.AuthSafe.Content.Bytes, &pfx.AuthSafe.Content); err != nil {
		return nil, nil, err
	}

	if len(pfx.MacData.Mac.Algorithm.Algorithm) == 0 {
		return nil, nil, errors.New("pkcs12: no MAC in data")
	}

	if err := xp12_verifyMac(&pfx.MacData, pfx.AuthSafe.Content.Bytes, password); err != nil {
		if err == xp12_ErrIncorrectPassword && len(password) == 2 && password[0] == 0 && password[1] == 0 {
			// some implementations use an empty byte array
			// for the empty string password try xp12_one more
			// time with empty-empty password
			password = nil
			err = xp12_verifyMac(&pfx.MacData, pfx.AuthSafe.Content.Bytes, password)
		}
		if err != nil {
			return nil, nil, err
		}
	}

	var authenticatedSafe []xp12_contentInfo
	if err := xp12_unmarshal(pfx.AuthSafe.Content.Bytes, &authenticatedSafe); err != nil {
		return nil, nil, err
	}

	if len(authenticatedSafe) != 2 {
		return nil, nil, xp12_NotImplementedError("expected exactly two items in the authenticated safe")
	}

	for _, ci := range authenticatedSafe {
		var data []byte

		switch {
		case ci.ContentType.Equal(xp12_oidDataContentType):
			if err := xp12_unmarshal(ci.Content.Bytes, &data); err != nil {
				return nil, nil, err
			}
		case ci.ContentType.Equal(xp12_oidEncryptedDataContentType):
			var xp12_encryptedData xp12_encryptedData
			if err := xp12_unmarshal(ci.Content.Bytes, &xp12_encryptedData); err != nil {
				return nil, nil, err
			}
			if xp12_encryptedData.Version != 0 {
				return nil, nil, xp12_NotImplementedError("only version 0 of EncryptedData is supported")
			}
			if data, err = xp12_pbDecrypt(xp12_encryptedData.EncryptedContentInfo, password); err != nil {
				return nil, nil, err
			}
		default:
			return nil, nil, xp12_NotImplementedError("only data and xp12_encryptedData content types are supported in authenticated safe")
		}

		var safeContents []xp12_safeBag
		if err := xp12_unmarshal(data, &safeContents); err != nil {
			return nil, nil, err
		}
		bags = append(bags, safeContents...)
	}

	return bags, password, nil
}

// ---- safebags.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

var (
	// see https://tools.ietf.org/html/rfc7292#appendix-D
	xp12_oidCertTypeX509Certificate = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 9, 22, 1})
	xp12_oidPKCS8ShroundedKeyBag    = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 10, 1, 2})
	xp12_oidCertBag                 = asn1.ObjectIdentifier([]int{1, 2, 840, 113549, 1, 12, 10, 1, 3})
)

type xp12_certBag struct {
	Id   asn1.ObjectIdentifier
	Data []byte `asn1:"tag:0,explicit"`
}

func xp12_decodePkcs8ShroudedKeyBag(asn1Data, password []byte) (privateKey interface{}, err error) {
	pkinfo := new(xp12_encryptedPrivateKeyInfo)
	if err = xp12_unmarshal(asn1Data, pkinfo); err != nil {
		return nil, errors.New("pkcs12: error decoding PKCS#8 shrouded key bag: " + err.Error())
	}

	pkData, err := xp12_pbDecrypt(pkinfo, password)
	if err != nil {
		return nil, errors.New("pkcs12: error decrypting PKCS#8 shrouded key bag: " + err.Error())
	}

	ret := new(asn1.RawValue)
	if err = xp12_unmarshal(pkData, ret); err != nil {
		return nil, errors.New("pkcs12: error unmarshaling decrypted private key: " + err.Error())
	}

	if privateKey, err = x509.ParsePKCS8PrivateKey(pkData); err != nil {
		return nil, errors.New("pkcs12: error parsing PKCS#8 private key: " + err.Error())
	}

	return privateKey, nil
}

func xp12_decodeCertBag(asn1Data []byte) (x509Certificates []byte, err error) {
	bag := new(xp12_certBag)
	if err := xp12_unmarshal(asn1Data, bag); err != nil {
		return nil, errors.New("pkcs12: error decoding cert bag: " + err.Error())
	}
	if !bag.Id.Equal(xp12_oidCertTypeX509Certificate) {
		return nil, xp12_NotImplementedError("only X509 certificates are supported")
	}
	return bag.Data, nil
}

// ---- internal/rc2/rc2.go
// Copyright 2015 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package rc2 implements the RC2 cipher
/*
https://www.ietf.org/rfc/rfc2268.txt
http://people.csail.mit.edu/rivest/pubs/KRRR98.pdf

This code is licensed under the MIT license.
*/

// The rc2 block size in bytes
const xp12_BlockSize = 8

type xp12_rc2Cipher struct {
	k [64]uint16
}

// xp12_New returns a new rc2 cipher with the given key and effective key length t1
func xp12_New(key []byte, t1 int) (cipher.Block, error) {
	// TODO(dgryski): error checking for key length
	return &xp12_rc2Cipher{
		k: xp12_expandKey(key, t1),
	}, nil
}

func (*xp12_rc2Cipher) BlockSize() int { return xp12_BlockSize }

var xp12_piTable = [256]byte{
	0xd9, 0x78, 0xf9, 0xc4, 0x19, 0xdd, 0xb5, 0xed, 0x28, 0xe9, 0xfd, 0x79, 0x4a, 0xa0, 0xd8, 0x9d,
	0xc6, 0x7e, 0x37, 0x83, 0x2b, 0x76, 0x53, 0x8e, 0x62, 0x4c, 0x64, 0x88, 0x44, 0x8b, 0xfb, 0xa2,
	0x17, 0x9a, 0x59, 0xf5, 0x87, 0xb3, 0x4f, 0x13, 0x61, 0x45, 0x6d, 0x8d, 0x09, 0x81, 0x7d, 0x32,
	0xbd, 0x8f, 0x40, 0xeb, 0x86, 0xb7, 0x7b, 0x0b, 0xf0, 0x95, 0x21, 0x22, 0x5c, 0x6b, 0x4e, 0x82,
	0x54, 0xd6, 0x65, 0x93, 0xce, 0x60, 0xb2, 0x1c, 0x73, 0x56, 0xc0, 0x14, 0xa7, 0x8c, 0xf1, 0xdc,
	0x12, 0x75, 0xca, 0x1f, 0x3b, 0xbe, 0xe4, 0xd1, 0x42, 0x3d, 0xd4, 0x30, 0xa3, 0x3c, 0xb6, 0x26,
	0x6f, 0xbf, 0x0e, 0xda, 0x46, 0x69, 0x07, 0x57, 0x27, 0xf2, 0x1d, 0x9b, 0xbc, 0x94, 0x43, 0x03,
	0xf8, 0x11, 0xc7, 0xf6, 0x90, 0xef, 0x3e, 0xe7, 0x06, 0xc3, 0xd5, 0x2f, 0xc8, 0x66, 0x1e, 0xd7,
	0x08, 0xe8, 0xea, 0xde, 0x80, 0x52, 0xee, 0xf7, 0x84, 0xaa, 0x72, 0xac, 0x35, 0x4d, 0x6a, 0x2a,
	0x96, 0x1a, 0xd2, 0x71, 0x5a, 0x15, 0x49, 0x74, 0x4b, 0x9f, 0xd0, 0x5e, 0x04, 0x18, 0xa4, 0xec,
	0xc2, 0xe0, 0x41, 0x6e, 0x0f, 0x51, 0xcb, 0xcc, 0x24, 0x91, 0xaf, 0x50, 0xa1, 0xf4, 0x70, 0x39,
	0x99, 0x7c, 0x3a, 0x85, 0x23, 0xb8, 0xb4, 0x7a, 0xfc, 0x02, 0x36, 0x5b, 0x25, 0x55, 0x97, 0x31,
	0x2d, 0x5d, 0xfa, 0x98, 0xe3, 0x8a, 0x92, 0xae, 0x05, 0xdf, 0x29, 0x10, 0x67, 0x6c, 0xba, 0xc9,
	0xd3, 0x00, 0xe6, 0xcf, 0xe1, 0x9e, 0xa8, 0x2c, 0x63, 0x16, 0x01, 0x3f, 0x58, 0xe2, 0x89, 0xa9,
	0x0d, 0x38, 0x34, 0x1b, 0xab, 0x33, 0xff, 0xb0, 0xbb, 0x48, 0x0c, 0x5f, 0xb9, 0xb1, 0xcd, 0x2e,
	0xc5, 0xf3, 0xdb, 0x47, 0xe5, 0xa5, 0x9c, 0x77, 0x0a, 0xa6, 0x20, 0x68, 0xfe, 0x7f, 0xc1, 0xad,
}

func xp12_expandKey(key []byte, t1 int) [64]uint16 {

	l := make([]byte, 128)
	copy(l, key)

	var t = len(key)
	var t8 = (t1 + 7) / 8
	var tm = byte(255 % uint(1<<(8+uint(t1)-8*uint(t8))))

	for i := len(key); i < 128; i++ {
		l[i] = xp12_piTable[l[i-1]+l[uint8(i-t)]]
	}

	l[128-t8] = xp12_piTable[l[128-t8]&tm]

	for i := 127 - t8; i >= 0; i-- {
		l[i] = xp12_piTable[l[i+1]^l[i+t8]]
	}

	var k [64]uint16

	for i := range k {
		k[i] = uint16(l[2*i]) + uint16(l[2*i+1])*256
	}

	return k
}

func (c *xp12_rc2Cipher) Encrypt(dst, src []byte) {

	r0 := binary.LittleEndian.Uint16(src[0:])
	r1 := binary.LittleEndian.Uint16(src[2:])
	r2 := binary.LittleEndian.Uint16(src[4:])
	r3 := binary.LittleEndian.Uint16(src[6:])

	var j int

	for j <= 16 {
		// mix r0
		r0 = r0 + c.k[j] + (r3 & r2) + ((^r3) & r1)
		r0 = bits.RotateLeft16(r0, 1)
		j++

		// mix r1
		r1 = r1 + c.k[j] + (r0 & r3) + ((^r0) & r2)
		r1 = bits.RotateLeft16(r1, 2)
		j++

		// mix r2
		r2 = r2 + c.k[j] + (r1 & r0) + ((^r1) & r3)
		r2 = bits.RotateLeft16(r2, 3)
		j++

		// mix r3
		r3 = r3 + c.k[j] + (r2 & r1) + ((^r2) & r0)
		r3 = bits.RotateLeft16(r3, 5)
		j++

	}

	r0 = r0 + c.k[r3&63]
	r1 = r1 + c.k[r0&63]
	r2 = r2 + c.k[r1&63]
	r3 = r3 + c.k[r2&63]

	for j <= 40 {
		// mix r0
		r0 = r0 + c.k[j] + (r3 & r2) + ((^r3) & r1)
		r0 = bits.RotateLeft16(r0, 1)
		j++

		// mix r1
		r1 = r1 + c.k[j] + (r0 & r3) + ((^r0) & r2)
		r1 = bits.RotateLeft16(r1, 2)
		j++

		// mix r2
		r2 = r2 + c.k[j] + (r1 & r0) + ((^r1) & r3)
		r2 = bits.RotateLeft16(r2, 3)
		j++

		// mix r3
		r3 = r3 + c.k[j] + (r2 & r1) + ((^r2) & r0)
		r3 = bits.RotateLeft16(r3, 5)
		j++

	}

	r0 = r0 + c.k[r3&63]
	r1 = r1 + c.k[r0&63]
	r2 = r2 + c.k[r1&63]
	r3 = r3 + c.k[r2&63]

	for j <= 60 {
		// mix r0
		r0 = r0 + c.k[j] + (r3 & r2) + ((^r3) & r1)
		r0 = bits.RotateLeft16(r0, 1)
		j++

		// mix r1
		r1 = r1 + c.k[j] + (r0 & r3) + ((^r0) & r2)
		r1 = bits.RotateLeft16(r1, 2)
		j++

		// mix r2
		r2 = r2 + c.k[j] + (r1 & r0) + ((^r1) & r3)
		r2 = bits.RotateLeft16(r2, 3)
		j++

		// mix r3
		r3 = r3 + c.k[j] + (r2 & r1) + ((^r2) & r0)
		r3 = bits.RotateLeft16(r3, 5)
		j++
	}

	binary.LittleEndian.PutUint16(dst[0:], r0)
	binary.LittleEndian.PutUint16(dst[2:], r1)
	binary.LittleEndian.PutUint16(dst[4:], r2)
	binary.LittleEndian.PutUint16(dst[6:], r3)
}

func (c *xp12_rc2Cipher) Decrypt(dst, src []byte) {

	r0 := binary.LittleEndian.Uint16(src[0:])
	r1 := binary.LittleEndian.Uint16(src[2:])
	r2 := binary.LittleEndian.Uint16(src[4:])
	r3 := binary.LittleEndian.Uint16(src[6:])

	j := 63

	for j >= 44 {
		// unmix r3
		r3 = bits.RotateLeft16(r3, 16-5)
		r3 = r3 - c.k[j] - (r2 & r1) - ((^r2) & r0)
		j--

		// unmix r2
		r2 = bits.RotateLeft16(r2, 16-3)
		r2 = r2 - c.k[j] - (r1 & r0) - ((^r1) & r3)
		j--

		// unmix r1
		r1 = bits.RotateLeft16(r1, 16-2)
		r1 = r1 - c.k[j] - (r0 & r3) - ((^r0) & r2)
		j--

		// unmix r0
		r0 = bits.RotateLeft16(r0, 16-1)
		r0 = r0 - c.k[j] - (r3 & r2) - ((^r3) & r1)
		j--
	}

	r3 = r3 - c.k[r2&63]
	r2 = r2 - c.k[r1&63]
	r1 = r1 - c.k[r0&63]
	r0 = r0 - c.k[r3&63]

	for j >= 20 {
		// unmix r3
		r3 = bits.RotateLeft16(r3, 16-5)
		r3 = r3 - c.k[j] - (r2 & r1) - ((^r2) & r0)
		j--

		// unmix r2
		r2 = bits.RotateLeft16(r2, 16-3)
		r2 = r2 - c.k[j] - (r1 & r0) - ((^r1) & r3)
		j--

		// unmix r1
		r1 = bits.RotateLeft16(r1, 16-2)
		r1 = r1 - c.k[j] - (r0 & r3) - ((^r0) & r2)
		j--

		// unmix r0
		r0 = bits.RotateLeft16(r0, 16-1)
		r0 = r0 - c.k[j] - (r3 & r2) - ((^r3) & r1)
		j--

	}

	r3 = r3 - c.k[r2&63]
	r2 = r2 - c.k[r1&63]
	r1 = r1 - c.k[r0&63]
	r0 = r0 - c.k[r3&63]

	for j >= 0 {
		// unmix r3
		r3 = bits.RotateLeft16(r3, 16-5)
		r3 = r3 - c.k[j] - (r2 & r1) - ((^r2) & r0)
		j--

		// unmix r2
		r2 = bits.RotateLeft16(r2, 16-3)
		r2 = r2 - c.k[j] - (r1 & r0) - ((^r1) & r3)
		j--

		// unmix r1
		r1 = bits.RotateLeft16(r1, 16-2)
		r1 = r1 - c.k[j] - (r0 & r3) - ((^r0) & r2)
		j--

		// unmix r0
		r0 = bits.RotateLeft16(r0, 16-1)
		r0 = r0 - c.k[j] - (r3 & r2) - ((^r3) & r1)
		j--

	}

	binary.LittleEndian.PutUint16(dst[0:], r0)
	binary.LittleEndian.PutUint16(dst[2:], r1)
	binary.LittleEndian.PutUint16(dst[4:], r2)
	binary.LittleEndian.PutUint16(dst[6:], r3)
}
