package rt

import (
	"crypto/mlkem"
	"crypto/mlkem/mlkemtest"
	"strings"
)

// ML-KEM (decision 0604): Go's crypto/mlkem, with the seed and the
// encapsulation message read from r as the native code reads them
// (64 bytes, then 32), so the same r gives the same keys and ciphertexts
// on every backend; mlkemtest encapsulates with a given message.

// Crypto_MlkemDecapsulationKey is `crypto.MlkemDecapsulationKey`. Debug
// never shows key material.
type Crypto_MlkemDecapsulationKey struct {
	k768  *mlkem.DecapsulationKey768
	k1024 *mlkem.DecapsulationKey1024
}

// Crypto_MlkemEncapsulationKey is `crypto.MlkemEncapsulationKey`.
type Crypto_MlkemEncapsulationKey struct {
	k768  *mlkem.EncapsulationKey768
	k1024 *mlkem.EncapsulationKey1024
}

func mlkemIs1024(p Crypto_MlkemParams) bool { return p.Tag == 2 }

func (v Crypto_MlkemDecapsulationKey) XoEq(x any) bool {
	o, ok := x.(Crypto_MlkemDecapsulationKey)
	return ok && v.M_bytes(nil) == o.M_bytes(nil) && (v.k1024 == nil) == (o.k1024 == nil)
}

func (v Crypto_MlkemDecapsulationKey) XoHash() uint64 { return Hash(string(v.M_bytes(nil))) }

func (Crypto_MlkemDecapsulationKey) XoDebug(b *strings.Builder) {
	b.WriteString("<mlkemdecapsulationkey>")
}

func (v Crypto_MlkemDecapsulationKey) M_debug(*Task) string { return Debug(v) }

func (v Crypto_MlkemEncapsulationKey) XoEq(x any) bool {
	o, ok := x.(Crypto_MlkemEncapsulationKey)
	return ok && v.M_bytes(nil) == o.M_bytes(nil) && (v.k1024 == nil) == (o.k1024 == nil)
}

func (v Crypto_MlkemEncapsulationKey) XoHash() uint64 { return Hash(string(v.M_bytes(nil))) }

func (Crypto_MlkemEncapsulationKey) XoDebug(b *strings.Builder) {
	b.WriteString("<mlkemencapsulationkey>")
}

func (v Crypto_MlkemEncapsulationKey) M_debug(*Task) string { return Debug(v) }

// M_encapsulation_key_size is MlkemParams.encapsulation_key_size.
func (p Crypto_MlkemParams) M_encapsulation_key_size(*Task) int64 {
	if mlkemIs1024(p) {
		return mlkem.EncapsulationKeySize1024
	}
	return mlkem.EncapsulationKeySize768
}

// M_ciphertext_size is MlkemParams.ciphertext_size.
func (p Crypto_MlkemParams) M_ciphertext_size(*Task) int64 {
	if mlkemIs1024(p) {
		return mlkem.CiphertextSize1024
	}
	return mlkem.CiphertextSize768
}

func mlkemDK(p Crypto_MlkemParams, seed []byte) (Crypto_MlkemDecapsulationKey, error) {
	if mlkemIs1024(p) {
		k, err := mlkem.NewDecapsulationKey1024(seed)
		return Crypto_MlkemDecapsulationKey{k1024: k}, err
	}
	k, err := mlkem.NewDecapsulationKey768(seed)
	return Crypto_MlkemDecapsulationKey{k768: k}, err
}

// Crypto_mlkem_generate_key is GenerateKey768/1024 reading the seed from r.
func Crypto_mlkem_generate_key(tk *Task, r Rand, p Crypto_MlkemParams) Crypto_MlkemDecapsulationKey {
	k, err := mlkemDK(p, []byte(r.M_bytes(tk, 64)))
	if err != nil {
		panic("crypto.mlkem_generate_key: " + err.Error())
	}
	return k
}

// Crypto_mlkem_decapsulation_key is NewDecapsulationKey768/1024.
func Crypto_mlkem_decapsulation_key(_ *Task, p Crypto_MlkemParams, seed Bytes) Result[Crypto_MlkemDecapsulationKey, Crypto_KeyErr] {
	k, err := mlkemDK(p, []byte(seed))
	if err != nil {
		return Err[Crypto_MlkemDecapsulationKey](keyErr(err.Error()))
	}
	return Ok[Crypto_MlkemDecapsulationKey, Crypto_KeyErr](k)
}

// Crypto_mlkem_encapsulation_key is NewEncapsulationKey768/1024.
func Crypto_mlkem_encapsulation_key(_ *Task, p Crypto_MlkemParams, key Bytes) Result[Crypto_MlkemEncapsulationKey, Crypto_KeyErr] {
	var e Crypto_MlkemEncapsulationKey
	var err error
	if mlkemIs1024(p) {
		e.k1024, err = mlkem.NewEncapsulationKey1024([]byte(key))
	} else {
		e.k768, err = mlkem.NewEncapsulationKey768([]byte(key))
	}
	if err != nil {
		return Err[Crypto_MlkemEncapsulationKey](keyErr(err.Error()))
	}
	return Ok[Crypto_MlkemEncapsulationKey, Crypto_KeyErr](e)
}

func (v Crypto_MlkemDecapsulationKey) M_params(*Task) Crypto_MlkemParams {
	if v.k1024 != nil {
		return Crypto_MlkemParams{Tag: 2}
	}
	return Crypto_MlkemParams{Tag: 1}
}

func (v Crypto_MlkemDecapsulationKey) M_bytes(*Task) Bytes {
	if v.k1024 != nil {
		return Bytes(v.k1024.Bytes())
	}
	return Bytes(v.k768.Bytes())
}

func (v Crypto_MlkemDecapsulationKey) M_encapsulation_key(*Task) Crypto_MlkemEncapsulationKey {
	if v.k1024 != nil {
		return Crypto_MlkemEncapsulationKey{k1024: v.k1024.EncapsulationKey()}
	}
	return Crypto_MlkemEncapsulationKey{k768: v.k768.EncapsulationKey()}
}

func (v Crypto_MlkemDecapsulationKey) M_decapsulate(_ *Task, c Bytes) Result[Bytes, Crypto_KeyErr] {
	var s []byte
	var err error
	if v.k1024 != nil {
		s, err = v.k1024.Decapsulate([]byte(c))
	} else {
		s, err = v.k768.Decapsulate([]byte(c))
	}
	if err != nil {
		return Err[Bytes](keyErr(err.Error()))
	}
	return Ok[Bytes, Crypto_KeyErr](Bytes(s))
}

func (v Crypto_MlkemEncapsulationKey) M_params(*Task) Crypto_MlkemParams {
	if v.k1024 != nil {
		return Crypto_MlkemParams{Tag: 2}
	}
	return Crypto_MlkemParams{Tag: 1}
}

func (v Crypto_MlkemEncapsulationKey) M_bytes(*Task) Bytes {
	if v.k1024 != nil {
		return Bytes(v.k1024.Bytes())
	}
	return Bytes(v.k768.Bytes())
}

// M_encapsulate is mlkemtest.Encapsulate768/1024 with 32 bytes from r.
func (v Crypto_MlkemEncapsulationKey) M_encapsulate(tk *Task, r Rand) Tuple2[Bytes, Bytes] {
	m := []byte(r.M_bytes(tk, 32))
	var s, c []byte
	var err error
	if v.k1024 != nil {
		s, c, err = mlkemtest.Encapsulate1024(v.k1024, m)
	} else {
		s, c, err = mlkemtest.Encapsulate768(v.k768, m)
	}
	if err != nil {
		panic("crypto.MlkemEncapsulationKey.encapsulate: " + err.Error())
	}
	return Tuple2[Bytes, Bytes]{Bytes(s), Bytes(c)}
}
