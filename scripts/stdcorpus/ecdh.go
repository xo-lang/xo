package main

import (
	"crypto/ecdh"
	"encoding/hex"
	"fmt"
	"math/rand"
	"strings"
)

// ecdhCurves are crypto/ecdh's curves in std/crypto's EcdhCurve order.
var ecdhCurves = []ecdh.Curve{ecdh.X25519(), ecdh.P256(), ecdh.P384(), ecdh.P521()}

var ecdhNames = []string{"X25519", "P256", "P384", "P521"}

func ecdhBytes(r *rand.Rand, n int) []byte {
	b := make([]byte, n)
	r.Read(b)
	return b
}

// ecdhPrivSize is the private key's length on curve i.
func ecdhPrivSize(i int) int { return []int{32, 32, 48, 66}[i] }

// ecdhPrivInput is a private key candidate: mostly valid, else a wrong
// length, zero, all 0xff (at or above the order), or P-521 with its high
// bits set.
func ecdhPrivInput(r *rand.Rand, i int) []byte {
	n := ecdhPrivSize(i)
	switch r.Intn(10) {
	case 0:
		return ecdhBytes(r, r.Intn(n+3))
	case 1:
		return make([]byte, n)
	case 2:
		return []byte(strings.Repeat("\xff", n))
	}
	b := ecdhBytes(r, n)
	if i == 3 && r.Intn(3) > 0 {
		b[0] &= 1
	}
	return b
}

// ecdhPubInput is a public key candidate on curve i: a valid key, or one
// changed: a byte flipped, cut, a compressed or identity form, all 0xff
// coordinates, or random bytes; X25519's low order points too.
func ecdhPubInput(r *rand.Rand, i int, valid []byte) []byte {
	b := append([]byte(nil), valid...)
	switch r.Intn(9) {
	case 0:
		b[r.Intn(len(b))] ^= byte(1 + r.Intn(255))
	case 1:
		b = b[:r.Intn(len(b))]
	case 2:
		if i > 0 {
			b = append([]byte{byte(2 + r.Intn(2))}, b[1:1+(len(b)-1)/2]...)
		}
	case 3:
		b = []byte{0}
	case 4:
		if i > 0 {
			b = append([]byte{4}, []byte(strings.Repeat("\xff", len(b)-1))...)
		}
	case 5:
		b = ecdhBytes(r, len(b))
		if i > 0 {
			b[0] = 4
		}
	case 6:
		if i == 0 {
			// low order points of Curve25519 (and their non-canonical forms)
			lows := []string{
				"0000000000000000000000000000000000000000000000000000000000000000",
				"0100000000000000000000000000000000000000000000000000000000000000",
				"e0eb7a7c3b41b8ae1656e3faf19fc46ada098deb9c32b1fd866205165f49b800",
				"5f9c95bca3508c24b1d0b1559c83ef5b04445cc4581c8e86d8224eddd09f1157",
				"ecffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
				"edffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
				"eeffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff7f",
			}
			b, _ = hex.DecodeString(lows[r.Intn(len(lows))])
		}
	}
	return b
}

func ecdhCorpus() string {
	r := rand.New(rand.NewSource(599))
	var o strings.Builder
	o.WriteString(header("crypto/ecdh's NewPrivateKey, PrivateKey.PublicKey, NewPublicKey and ECDH on seeded random keys, invalid scalars and points, and X25519's low order points"))
	o.WriteString("use std/crypto\n\nconst CURVES: List[crypto.EcdhCurve] = [crypto.EcdhCurve.X25519, crypto.EcdhCurve.P256, crypto.EcdhCurve.P384, crypto.EcdhCurve.P521]\n\n/// (curve, private key hex, Go's NewPrivateKey: ok PUBLIC_HEX or err TEXT)\nconst PRIVS: List[(Int, Str, Str)] = [\n")
	for k := 0; k < 600; k++ {
		i := r.Intn(4)
		b := ecdhPrivInput(r, i)
		got := ""
		if pk, err := ecdhCurves[i].NewPrivateKey(b); err != nil {
			got = "err " + err.Error()
		} else {
			got = "ok " + hex.EncodeToString(pk.PublicKey().Bytes())
		}
		fmt.Fprintf(&o, "  (%d, %s, %s),\n", i, xq(hex.EncodeToString(b)), xq(got))
	}
	o.WriteString("]\n\n/// (curve, private key hex, peer public key hex, Go's NewPublicKey then ECDH: ok SECRET_HEX or err TEXT)\nconst AGREE: List[(Int, Str, Str, Str)] = [\n")
	for k := 0; k < 800; k++ {
		i := r.Intn(4)
		var priv, peer *ecdh.PrivateKey
		for priv == nil || peer == nil {
			priv, _ = ecdhCurves[i].NewPrivateKey(ecdhBytes(r, ecdhPrivSize(i)))
			peer, _ = ecdhCurves[i].NewPrivateKey(ecdhBytes(r, ecdhPrivSize(i)))
		}
		pb := ecdhPubInput(r, i, peer.PublicKey().Bytes())
		got := ""
		if pub, err := ecdhCurves[i].NewPublicKey(pb); err != nil {
			got = "err " + err.Error()
		} else if s, err := priv.ECDH(pub); err != nil {
			got = "err " + err.Error()
		} else {
			got = "ok " + hex.EncodeToString(s)
		}
		fmt.Fprintf(&o, "  (%d, %s, %s, %s),\n", i, xq(hex.EncodeToString(priv.Bytes())), xq(hex.EncodeToString(pb)), xq(got))
	}
	o.WriteString("]\n")
	o.WriteString(ecdhTests)
	return o.String()
}

const ecdhTests = `
fn first_bad[T](bad: List[T]) -> List[T] { bad[0..(if bad.len() < 20 { bad.len() } else { 20 })] }

fn hx(s: Str) -> Bytes { Bytes.from_hex(s) ?? fault("hex") }

test "private keys are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (i, key, want) in PRIVS {
    let got = match crypto.ecdh_private_key(CURVES[i], hx(key)) {
      Ok(k) => {
        if k.bytes() != hx(key) || k.curve() != CURVES[i] || k.public_key().curve() != CURVES[i] {
          "round trip"
        } else {
          "ok ${k.public_key().bytes().to_hex()}"
        }
      }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((key, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "public keys and shared secrets are Go's" {
  var bad: List[(Str, Str, Str)] = []
  for (i, priv, peer, want) in AGREE {
    let k = crypto.ecdh_private_key(CURVES[i], hx(priv)).ok() ?? fault("private key")
    let got = match crypto.ecdh_public_key(CURVES[i], hx(peer)) {
      Ok(p) => {
        if p.bytes() != hx(peer) {
          "round trip"
        } else {
          match k.ecdh(p) {
            Ok(s) => "ok ${s.to_hex()}"
            Err(e) => "err ${e.msg}"
          }
        }
      }
      Err(e) => "err ${e.msg}"
    }
    if got != want {
      bad.push((peer, got, want))
    }
  }
  expect(first_bad(bad)) == []
}

test "curves must match" {
  let a = crypto.ecdh_private_key(crypto.EcdhCurve.P256, hx(PRIVS.find(fn(p) { p.0 == 1 && p.2.starts_with("ok ") }).map(fn(p) { p.1 }) ?? fault("p256"))).ok() ?? fault("a")
  let b = crypto.ecdh_private_key(crypto.EcdhCurve.X25519, hx(PRIVS.find(fn(p) { p.0 == 0 && p.2.starts_with("ok ") }).map(fn(p) { p.1 }) ?? fault("x"))).ok() ?? fault("b")
  expect(a.ecdh(b.public_key()).map_err(fn(e) { e.msg })) == Err("crypto/ecdh: private key and public key curves do not match")
}
`
