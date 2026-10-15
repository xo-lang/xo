package rt

import (
	"crypto/ecdh"
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"crypto/sha3"
	"strconv"
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
