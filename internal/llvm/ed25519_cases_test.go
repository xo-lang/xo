package llvm

// The Ed25519 cases of TestCryptoXoDiff (consttime_test.go): the RFC 8032
// vectors and random ones against Go's crypto/ed25519 (they were
// crypto_diff_test.go's until its C test went, decision 0241).

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/big"
	"math/rand"
)

// ed25519Cases are the Ed25519 operations of TestCryptoXoDiff
// (decision 0189): the RFC 8032 section 7.1 vectors, then random seeds
// and messages signed and verified as Go's crypto/ed25519 does, with
// mutated signatures, keys, and messages, random public keys (about
// half are no point), non canonical encodings of A (y of p or more,
// x = 0 with the sign bit), S of L or more, and wrong lengths.
func ed25519Cases(r *rand.Rand, n int) []struct{ in, want string } {
	type kase = struct{ in, want string }
	hx := func(b []byte) string {
		if len(b) == 0 {
			return "-"
		}
		return hex.EncodeToString(b)
	}
	bit := func(ok bool) string {
		if ok {
			return "1"
		}
		return "0"
	}
	rb := func(k int) []byte {
		b := make([]byte, k)
		r.Read(b)
		return b
	}
	var cs []kase
	for _, v := range [][4]string{
		{"9d61b19deffd5a60ba844af492ec2cc44449c5697b326919703bac031cae7f60", "d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a", "-", "e5564300c360ac729086e2cc806e828a84877f1eb8e5d974d873e065224901555fb8821590a33bacc61e39701cf9b46bd25bf5f0595bbe24655141438e7a100b"},
		{"4ccd089b28ff96da9db6c346ec114e0f5b8a319f35aba624da8cf6ed4fb8a6fb", "3d4017c3e843895a92b70aa74d1b7ebc9c982ccf2ec4968cc0cd55f12af4660c", "72", "92a009a9f0d4cab8720e820b5f642540a2b27b5416503f8fb3762223ebdb69da085ac1e43e15996e458f3613d0f11d8c387b2eaeb4302aeeb00d291612bb0c00"},
		{"c5aa8df43f9f837bedb7442f31dcb7b166d38535076f094b85ce3a2e0b4458f7", "fc51cd8e6218a1a38da47ed00230f0580816ed13ba3303ac5deb911548908025", "af82", "6291d657deec24024827e69c3abe01a30ce548a284743a445e3680d7db5ac3ac18ff9b538d16f290ae67f760984dc6594a7c15e9716ed28dc027beceea1ec40a"},
	} {
		cs = append(cs, kase{"edpub " + v[0], v[1]}, kase{"edsign " + v[0] + " " + v[2], v[3]}, kase{"edverify " + v[1] + " " + v[2] + " " + v[3], "1"})
	}
	// messages whose SHA-512 inputs (32 byte prefix + message, 64 bytes
	// of R and A + message) end around the block and padding boundaries
	// of SHA-512 (std/native/llrt/digest.xo's sha512_le)
	for _, k := range []int{47, 48, 49, 63, 64, 65, 79, 80, 81, 95, 96, 97, 111, 112, 113, 127, 128, 129, 175, 176, 177, 191, 192, 193, 207, 208, 209, 223, 224, 225, 1000} {
		seed, msg := rb(32), rb(k)
		priv := ed25519.NewKeyFromSeed(seed)
		sig := ed25519.Sign(priv, msg)
		cs = append(cs, kase{"edsign " + hx(seed) + " " + hx(msg), hx(sig)}, kase{"edverify " + hx([]byte(priv.Public().(ed25519.PublicKey))) + " " + hx(msg) + " " + hx(sig), "1"})
	}
	verify := func(pub, msg, sig []byte) kase {
		ok := len(pub) == ed25519.PublicKeySize && len(sig) == ed25519.SignatureSize && ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
		return kase{"edverify " + hx(pub) + " " + hx(msg) + " " + hx(sig), bit(ok)}
	}
	for i := 0; i < n/2; i++ {
		seed := rb(32)
		msg := rb(r.Intn(300))
		priv := ed25519.NewKeyFromSeed(seed)
		pub := []byte(priv.Public().(ed25519.PublicKey))
		sig := ed25519.Sign(priv, msg)
		cs = append(cs, kase{"edpub " + hx(seed), hx(pub)}, kase{"edsign " + hx(seed) + " " + hx(msg), hx(sig)}, verify(pub, msg, sig))
		bad := append([]byte(nil), sig...)
		bad[r.Intn(64)] ^= byte(1 << r.Intn(8))
		cs = append(cs, verify(pub, msg, bad))
		if len(msg) > 0 {
			m2 := append([]byte(nil), msg...)
			m2[r.Intn(len(m2))] ^= 1
			cs = append(cs, verify(pub, m2, sig))
		}
		p2 := append([]byte(nil), pub...)
		p2[r.Intn(32)] ^= byte(1 << r.Intn(8))
		cs = append(cs, verify(p2, msg, sig), verify(rb(32), msg, sig), verify(pub[:31], msg, sig), verify(pub, msg, sig[:63]))
		// S + L: the same point equation, refused as non canonical
		s := new(big.Int).SetBytes(reverse(sig[32:]))
		s.Add(s, edL)
		if s.BitLen() <= 256 {
			high := append(append([]byte(nil), sig[:32]...), reverse(leftPad(s.Bytes(), 32))...)
			cs = append(cs, verify(pub, msg, high))
		}
	}
	// non canonical encodings of A: y = p + j (j < 19), both sign bits,
	// and x = 0 with the sign bit (y = 1 and y = p - 1)
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 255), big.NewInt(19))
	sig := ed25519.Sign(ed25519.NewKeyFromSeed(rb(32)), []byte("m"))
	for j := int64(0); j < 19; j++ {
		y := reverse(leftPad(new(big.Int).Add(p, big.NewInt(j)).Bytes(), 32))
		for _, sign := range []byte{0, 0x80} {
			a := append([]byte(nil), y...)
			a[31] |= sign
			cs = append(cs, verify(a, []byte("m"), sig))
		}
	}
	for _, y := range []*big.Int{big.NewInt(1), new(big.Int).Sub(p, big.NewInt(1))} {
		a := reverse(leftPad(y.Bytes(), 32))
		a[31] |= 0x80
		cs = append(cs, verify(a, []byte("m"), sig))
	}
	return cs
}

// edL is the order of the Ed25519 base point.
var edL, _ = new(big.Int).SetString("7237005577332262213973186563042994240857116359379907606001950938285454250989", 10)

func reverse(b []byte) []byte {
	out := make([]byte, len(b))
	for i := range b {
		out[i] = b[len(b)-1-i]
	}
	return out
}

func leftPad(b []byte, n int) []byte {
	if len(b) >= n {
		return b
	}
	return append(make([]byte, n-len(b)), b...)
}
