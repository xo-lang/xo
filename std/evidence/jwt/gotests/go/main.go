// Command go is the Go side of std/jwt's gotests evidence (decision 0720,
// task #403): main CASES prints, for each case line `KIND<TAB>B64...`, the
// base64 of golang-jwt v5's result as text; std/evidence/jwt/gotests/main.xo
// prints std/jwt's the same way. See scripts/stdproven/gotests.go.
//
// Kind verify over golang-jwt's signing tables (hmacTestData, rsaTestData,
// rsaPSSTestData, ecdsaTestData, ed25519TestData): the token parsed with
// only its table's alg allowed and the test key of that alg (golang-jwt's
// test/ files, MIT), the clock before the tables' exp; "ok" and the claims
// as sorted JSON, or the error's category (scripts/stdcorpus/jwtoracle's).
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const hmacKeyB64 = "AyM1SysPpbyDfgZld3umj1qzKObwVMkoqQ+EstJQLr/T+1qS0gZH75aKtMN3Yj0iPS4hcgUuTwjAzZr1Z9CAow=="

var pems = map[string]string{
	"rsa": "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA4f5wg5l2hKsTeNem/V41\nfGnJm6gOdrj8ym3rFkEU/wT8RDtnSgFEZOQpHEgQ7JL38xUfU0Y3g6aYw9QT0hJ7\nmCpz9Er5qLaMXJwZxzHzAahlfA0icqabvJOMvQtzD6uQv6wPEyZtDTWiQi9AXwBp\nHssPnpYGIn20ZZuNlX2BrClciHhCPUIIZOQn/MmqTD31jSyjoQoV7MhhMTATKJx2\nXrHhR+1DcKJzQBSTAGnpYVaqpsARap+nwRipr3nUTuxyGohBTSmjJ2usSeQXHI3b\nODIRe1AuTyHceAbewn8b462yEWKARdpd9AjQW5SIVPfdsz5B6GlYQ5LdYKtznTuy\n7wIDAQAB\n-----END PUBLIC KEY-----\n",
	"ec256": "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEYD54V/vp+54P9DXarYqx4MPcm+HK\nRIQzNasYSoRQHQ/6S6Ps8tpMcT+KvIIC8W/e9k0W7Cm72M1P9jU7SLf/vg==\n-----END PUBLIC KEY-----\n",
	"ec384": "-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAE2Gu6yWlO33izKqwMemnWRTUDqWlB/1Mp\nW2S64jizjeWBVcLVVKTtRXxF2VCEKabUT7XOYsSD2OufMoQUm+oq3yCVEj/WmE35\nSRipP5g5CuLfJlgc4Yg+Qeo4PXBBoRoA\n-----END PUBLIC KEY-----\n",
	"ec512": "-----BEGIN PUBLIC KEY-----\nMIGbMBAGByqGSM49AgEGBSuBBAAjA4GGAAQACVyLsNdjFM6R4IImvTzgRWF0sWjh\nihmzIyMgyPuqu8IuyzMNx4G2jpoCKhRu9qPCQUMGDeCG1x3/n/OgkWNQANsB82x7\n7eiIZAl0zcQRH32tcjFILvJ/XCihdoi4MkCnCqlt9/HxjsP590ZtmHfxAeertq5w\n9vakvpzjPXhkvoMt/Tk=\n-----END PUBLIC KEY-----\n",
	"ed25519": "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAWH7z6hpYqvPns2i4n9yymwvB3APhi4LyQ7iHOT6crtE=\n-----END PUBLIC KEY-----\n",
}

// keyName is the test key golang-jwt's tables verify alg with.
func keyName(alg string) string {
	switch {
	case strings.HasPrefix(alg, "HS"):
		return "hmac"
	case strings.HasPrefix(alg, "RS"), strings.HasPrefix(alg, "PS"):
		return "rsa"
	case alg == "ES256":
		return "ec256"
	case alg == "ES384":
		return "ec384"
	case alg == "ES512":
		return "ec512"
	}
	return "ed25519"
}

func key(alg string) any {
	name := keyName(alg)
	if name == "hmac" {
		b, _ := base64.StdEncoding.DecodeString(hmacKeyB64)
		return b
	}
	var k any
	var err error
	switch name {
	case "rsa":
		k, err = jwt.ParseRSAPublicKeyFromPEM([]byte(pems[name]))
	case "ed25519":
		k, err = jwt.ParseEdPublicKeyFromPEM([]byte(pems[name]))
	default:
		k, err = jwt.ParseECPublicKeyFromPEM([]byte(pems[name]))
	}
	if err != nil {
		panic(err)
	}
	return k
}

func verify(token, alg string) string {
	if parts := strings.Split(token, "."); len(parts) == 3 {
		if hb, err := base64.RawURLEncoding.DecodeString(parts[0]); err == nil {
			var h map[string]any
			if json.Unmarshal(hb, &h) == nil {
				if name, ok := h["alg"].(string); !ok || name != alg {
					return "alg"
				}
			}
		}
	}
	p := jwt.NewParser(jwt.WithValidMethods([]string{alg}), jwt.WithTimeFunc(func() time.Time { return time.Unix(1300000000, 0) }))
	t, err := p.Parse(token, func(*jwt.Token) (any, error) { return key(alg), nil })
	switch {
	case err == nil:
		b, _ := json.Marshal(t.Claims)
		return "ok " + string(b)
	case errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return "notyet"
	case errors.Is(err, jwt.ErrTokenSignatureInvalid):
		return "signature"
	case errors.Is(err, jwt.ErrTokenUnverifiable):
		return "alg"
	case errors.Is(err, jwt.ErrTokenMalformed):
		return "malformed"
	case errors.Is(err, jwt.ErrTokenRequiredClaimMissing), errors.Is(err, jwt.ErrTokenInvalidClaims):
		return "claim"
	}
	return "other: " + err.Error()
}

func run(kind string, in []string) string {
	switch kind {
	case "verify":
		return verify(in[0], in[1])
	}
	return "unknown kind " + kind
}

func main() {
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(nil, 1<<24)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()
	for sc.Scan() {
		f := strings.Split(sc.Text(), "\t")
		in := make([]string, 0, len(f)-1)
		for _, s := range f[1:] {
			b, err := base64.StdEncoding.DecodeString(s)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
			in = append(in, string(b))
		}
		fmt.Fprintln(w, base64.StdEncoding.EncodeToString([]byte(run(f[0], in))))
	}
}
