package rt

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"crypto/sha3"
	"strconv"
	"strings"
)

// cryptoSHA3 is sha3.xo's op with Go's crypto/sha3.
func cryptoSHA3(what string, a, b Bytes) (Bytes, bool) {
	n, _ := strconv.Atoi(string(b))
	switch what {
	case "sha3_256":
		s := sha3.Sum256([]byte(a))
		return Bytes(s[:]), true
	case "sha3_512":
		s := sha3.Sum512([]byte(a))
		return Bytes(s[:]), true
	case "shake128":
		return Bytes(sha3.SumSHAKE128([]byte(a), n)), true
	case "shake256":
		return Bytes(sha3.SumSHAKE256([]byte(a), n)), true
	}
	return "", false
}

// cryptoMLKEM is mlkem.xo's op with Go's crypto/mlkem (and
// mlkemtest.Encapsulate768 for a fixed message) and crypto/ecdh's X25519:
// the hybrid as crypto/tls key_schedule.go's hybridKeyExchange.
func cryptoMLKEM(what string, a, b, c Bytes) (Bytes, bool) {
	if strings.HasPrefix(what, "p256_hybrid_") || strings.HasPrefix(what, "p384_hybrid_") {
		return cryptoSecpHybrid(what[:4] == "p384", what[5:], []byte(a), []byte(b), []byte(c))
	}
	if n, ok := strings.CutSuffix(what, "_1024"); ok {
		return cryptoMLKEM1024(n, a, b)
	}
	switch what {
	case "public":
		dk, err := mlkem.NewDecapsulationKey768([]byte(a))
		if err != nil {
			return "", true
		}
		return Bytes(dk.EncapsulationKey().Bytes()), true
	case "encaps":
		ek, err := mlkem.NewEncapsulationKey768([]byte(a))
		if err != nil {
			return "", true
		}
		k, ct, err := mlkemtest.Encapsulate768(ek, []byte(b))
		if err != nil {
			return "", true
		}
		return Bytes(append(k, ct...)), true
	case "decaps":
		dk, err := mlkem.NewDecapsulationKey768([]byte(a))
		if err != nil {
			return "", true
		}
		k, err := dk.Decapsulate([]byte(b))
		if err != nil {
			return "", true
		}
		return Bytes(k), true
	case "hybrid_share":
		dk, err := mlkem.NewDecapsulationKey768([]byte(a[:64]))
		if err != nil {
			return "", true
		}
		x, err := ecdh.X25519().NewPrivateKey([]byte(a[64:96]))
		if err != nil {
			return "", true
		}
		return Bytes(append(dk.EncapsulationKey().Bytes(), x.PublicKey().Bytes()...)), true
	case "hybrid_server":
		share := []byte(a)
		if len(share) != mlkem.EncapsulationKeySize768+32 {
			return "", true
		}
		x, err := ecdh.X25519().NewPrivateKey([]byte(b))
		if err != nil {
			return "", true
		}
		peer, err := ecdh.X25519().NewPublicKey(share[mlkem.EncapsulationKeySize768:])
		if err != nil {
			return "", true
		}
		xs, err := x.ECDH(peer)
		if err != nil {
			return "", true
		}
		ek, err := mlkem.NewEncapsulationKey768(share[:mlkem.EncapsulationKeySize768])
		if err != nil {
			return "", true
		}
		k, ct, err := mlkemtest.Encapsulate768(ek, []byte(c))
		if err != nil {
			return "", true
		}
		out := append(append(append(k, xs...), ct...), x.PublicKey().Bytes()...)
		return Bytes(out), true
	case "accumulated":
		n, _ := strconv.Atoi(string(a))
		s, o := sha3.NewSHAKE128(), sha3.NewSHAKE128()
		seed, msg, ct1 := make([]byte, mlkem.SeedSize), make([]byte, 32), make([]byte, mlkem.CiphertextSize768)
		for i := 0; i < n; i++ {
			s.Read(seed)
			dk, err := mlkem.NewDecapsulationKey768(seed)
			if err != nil {
				return "", true
			}
			ek := dk.EncapsulationKey()
			o.Write(ek.Bytes())
			s.Read(msg)
			k, ct, err := mlkemtest.Encapsulate768(ek, msg)
			if err != nil {
				return "", true
			}
			o.Write(ct)
			o.Write(k)
			if kk, _ := dk.Decapsulate(ct); string(kk) != string(k) {
				return "decapsulation differs", true
			}
			s.Read(ct1)
			k1, _ := dk.Decapsulate(ct1)
			o.Write(k1)
		}
		out := make([]byte, 32)
		o.Read(out)
		return Bytes(out), true
	case "hybrid_client":
		share := []byte(b)
		if len(share) != mlkem.CiphertextSize768+32 {
			return "", true
		}
		x, err := ecdh.X25519().NewPrivateKey([]byte(a[64:96]))
		if err != nil {
			return "", true
		}
		peer, err := ecdh.X25519().NewPublicKey(share[mlkem.CiphertextSize768:])
		if err != nil {
			return "", true
		}
		xs, err := x.ECDH(peer)
		if err != nil {
			return "", true
		}
		dk, err := mlkem.NewDecapsulationKey768([]byte(a[:64]))
		if err != nil {
			return "", true
		}
		k, err := dk.Decapsulate(share[:mlkem.CiphertextSize768])
		if err != nil {
			return "", true
		}
		return Bytes(append(k, xs...)), true
	}
	return "", false
}

// cryptoMLKEM1024 is mlkem.xo's `_1024` ops with crypto/mlkem.
func cryptoMLKEM1024(what string, a, b Bytes) (Bytes, bool) {
	switch what {
	case "public":
		dk, err := mlkem.NewDecapsulationKey1024([]byte(a))
		if err != nil {
			return "", true
		}
		return Bytes(dk.EncapsulationKey().Bytes()), true
	case "encaps":
		ek, err := mlkem.NewEncapsulationKey1024([]byte(a))
		if err != nil {
			return "", true
		}
		k, ct, err := mlkemtest.Encapsulate1024(ek, []byte(b))
		if err != nil {
			return "", true
		}
		return Bytes(append(k, ct...)), true
	case "decaps":
		dk, err := mlkem.NewDecapsulationKey1024([]byte(a))
		if err != nil {
			return "", true
		}
		k, err := dk.Decapsulate([]byte(b))
		if err != nil {
			return "", true
		}
		return Bytes(k), true
	case "accumulated":
		n, _ := strconv.Atoi(string(a))
		s, o := sha3.NewSHAKE128(), sha3.NewSHAKE128()
		seed, msg, ct1 := make([]byte, mlkem.SeedSize), make([]byte, 32), make([]byte, mlkem.CiphertextSize1024)
		for i := 0; i < n; i++ {
			s.Read(seed)
			dk, err := mlkem.NewDecapsulationKey1024(seed)
			if err != nil {
				return "", true
			}
			ek := dk.EncapsulationKey()
			o.Write(ek.Bytes())
			s.Read(msg)
			k, ct, err := mlkemtest.Encapsulate1024(ek, msg)
			if err != nil {
				return "", true
			}
			o.Write(ct)
			o.Write(k)
			if kk, _ := dk.Decapsulate(ct); string(kk) != string(k) {
				return "decapsulation differs", true
			}
			s.Read(ct1)
			k1, _ := dk.Decapsulate(ct1)
			o.Write(k1)
		}
		out := make([]byte, 32)
		o.Read(out)
		return Bytes(out), true
	}
	return "", false
}

// cryptoSecpHybrid is mlkem.xo's p256_ and p384_ hybrid ops: crypto/tls
// key_schedule.go's hybridKeyExchange for SecP256r1MLKEM768 and
// SecP384r1MLKEM1024 (the ECDH part first). A private key is the
// ML-KEM seed, then the ECDH key.
func cryptoSecpHybrid(p384 bool, what string, a, b, c []byte) (Bytes, bool) {
	curve, ekSize, ctSize := ecdh.P256(), mlkem.EncapsulationKeySize768, mlkem.CiphertextSize768
	pubSize := 65
	if p384 {
		curve, ekSize, ctSize, pubSize = ecdh.P384(), mlkem.EncapsulationKeySize1024, mlkem.CiphertextSize1024, 97
	}
	type dkey interface {
		Decapsulate([]byte) ([]byte, error)
	}
	newDK := func(seed []byte) (dkey, []byte, error) {
		if p384 {
			dk, err := mlkem.NewDecapsulationKey1024(seed)
			if err != nil {
				return nil, nil, err
			}
			return dk, dk.EncapsulationKey().Bytes(), nil
		}
		dk, err := mlkem.NewDecapsulationKey768(seed)
		if err != nil {
			return nil, nil, err
		}
		return dk, dk.EncapsulationKey().Bytes(), nil
	}
	ecdhOf := func(priv, peer []byte) ([]byte, []byte, bool) {
		k, err := curve.NewPrivateKey(priv)
		if err != nil {
			return nil, nil, false
		}
		if peer == nil {
			return nil, k.PublicKey().Bytes(), true
		}
		p, err := curve.NewPublicKey(peer)
		if err != nil {
			return nil, nil, false
		}
		x, err := k.ECDH(p)
		if err != nil {
			return nil, nil, false
		}
		return x, k.PublicKey().Bytes(), true
	}
	switch what {
	case "hybrid_share":
		_, ek, err := newDK(a[:64])
		if err != nil {
			return "", true
		}
		_, pub, ok := ecdhOf(a[64:], nil)
		if !ok {
			return "", true
		}
		return Bytes(append(pub, ek...)), true
	case "hybrid_server":
		if len(a) != pubSize+ekSize {
			return "", true
		}
		x, pub, ok := ecdhOf(b, a[:pubSize])
		if !ok {
			return "", true
		}
		var k, ct []byte
		var err error
		if p384 {
			ek, e := mlkem.NewEncapsulationKey1024(a[pubSize:])
			if e != nil {
				return "", true
			}
			k, ct, err = mlkemtest.Encapsulate1024(ek, c)
		} else {
			ek, e := mlkem.NewEncapsulationKey768(a[pubSize:])
			if e != nil {
				return "", true
			}
			k, ct, err = mlkemtest.Encapsulate768(ek, c)
		}
		if err != nil {
			return "", true
		}
		return Bytes(append(append(append(x, k...), pub...), ct...)), true
	case "hybrid_client":
		if len(b) != pubSize+ctSize {
			return "", true
		}
		x, _, ok := ecdhOf(a[64:], b[:pubSize])
		if !ok {
			return "", true
		}
		dk, _, err := newDK(a[:64])
		if err != nil {
			return "", true
		}
		k, err := dk.Decapsulate(b[pubSize:])
		if err != nil {
			return "", true
		}
		return Bytes(append(x, k...)), true
	}
	return "", false
}
