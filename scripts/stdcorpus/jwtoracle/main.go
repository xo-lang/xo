// Command jwtoracle prints the std/jwt corpus: github.com/golang-jwt/jwt/v5
// v5.3.1 (std/jwt's reference) signs tokens with fixed keys, verifies
// them under many options, and the Xo test checks that std/jwt answers the
// same category for each (ok, signature, expired, notyet, issuer, audience,
// subject, alg, malformed, claim). scripts/stdcorpus runs it; it is its own
// module so the repository's go.mod has no third-party requirement.
package main

import (
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var keyPEMs = map[string][2]string{
	"rsa":     {"-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDB4VbTYTHERXZ/\n9ObD76o0l4GPFoyb+5KOqnGnhlq3t7e8s09gdi6sGOGrVHbQK7FBrHt9Z0l2XY5A\npoVMq+q2LW81n7fdRaHtpkTuetISI4xkC1/Px3Akgqr4btBs1ZvA+uUiJfkB6p2i\nrnz8TzZleAuLaKezvGQOkSIiDkTnfQJR72VCPDAC+iL7QblYkDRXRp6PQKr/KD5/\nW9swaUFopFPDQFkVeAxSjVqlZtdQJtdpmrmn3EZbuEUb6i8YoUdOLhDuJ9NnCOOE\nPvbyIwl4Fv4bM8a3Wrbw1Sgj9bWQ+qG3aO/CLB+63b4SmJ/D10R6dPapchK56wfL\n1TUf90yLAgMBAAECggEAFTl8flaBEroB/38JdkC58tj1K3tt+DOBt1GMcKkCLGOV\n3zgIBAjcEiYFKnLsMNzfok9mE3PntGgIhyZTqHGXl8sAhi/OepBBallN6YeTkL3I\nhDlEi2QLcEvHuLu04H0/+E/fHq5mhKGxERql/x8S1ufd56yU01XX5EpaIL1MKMUz\nTEVCuuYaQ1ckXwSEL1bVehGXiayqkiPwa0Sqf7Cxt3doTUnrZPKJnZjOXS/ZtuM3\nZj0JreMA0iEIxto2R4iATsuj4MsXGuA31COnfDHY1nFQFmZTaV3BHWgE7ry+xd//\nJIvd/MkLQRG5+qeFn7504bdm+kKa0dR4ymulCOIycQKBgQDSTlaUlNbMG/wB04Jt\nv7uNTfKz+mYiqGKBHzDYhGiUaJJXxC043AZIZstPPsNQ0fmc0d15kasbLEf3pHFA\nqX429xxwD87x/ARBgr9PKBRbyH7aZkSdTOtXWRecdpbCh6QCTw/U6CGk0f0rHRgT\nEPZl56MJ5BHYLe4r+cIKqNyCOwKBgQDsAV1eeLxJrZoIvj1jVKRPXpHI9abEZgat\nweXU3IXu6rpgKplIjLp7m1Dc48kViIJR7inDE+bjrItxK6YQd0fAImd5EVZ5w2QF\na7lwGF4Irq3VQKHPYiai78fhm6YCcmBYarh2sQqRYp/qq192Z+hk0hLuMUZthVKZ\nGWfE8vvp8QKBgGb0CLiWII0tSfJO6/P0P8mOFjl7ZJKk4gwaC9yC/XthsfqbRBQo\nyj6ZbAAwVodzM4JMzuJbqGsWA2k9LGAeWXZpBJfrdUDLQf4cFrGWiljgTxF1rTNg\nW3mYWNG00FsM8gJbWAG/XfEFB64agRWcYTND8XG/H+k3mvgWPaLgRYrPAoGBANhr\nP/Xb/b4Kxk/yNFUcTzjBXirMqcs8TC+WuW4Dc4ZuFc8OAVZ4RqIHAZYuBD4M2oTL\nQ9yyKW7q0UxMXJKI73EPm5d4ALbUKKqX6IsUrVaE6L+/7yI/vXJ5M/B9TTS9MgCi\njNm67Bf6Ou91i6sHCiM6SUXIRe807VOb6n6+26fhAoGBAKoVJLW4AE6RUm/AnCdA\n1nf6UF5mwhQ3ZfxLkw8nHUP3X5QOJ1GcCAR8oIshARBb6Grf96vO4QFQxi9YNOjq\n10309Htd0Ejcc3usUj+MFsswG6AtkXKTE0x6gjoIQRoSX+FJqvKzeh2p35KsZeXC\nZComAH7nKEXIAUHFDCTh1yku\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAweFW02ExxEV2f/Tmw++q\nNJeBjxaMm/uSjqpxp4Zat7e3vLNPYHYurBjhq1R20CuxQax7fWdJdl2OQKaFTKvq\nti1vNZ+33UWh7aZE7nrSEiOMZAtfz8dwJIKq+G7QbNWbwPrlIiX5Aeqdoq58/E82\nZXgLi2ins7xkDpEiIg5E530CUe9lQjwwAvoi+0G5WJA0V0aej0Cq/yg+f1vbMGlB\naKRTw0BZFXgMUo1apWbXUCbXaZq5p9xGW7hFG+ovGKFHTi4Q7ifTZwjjhD728iMJ\neBb+GzPGt1q28NUoI/W1kPqht2jvwiwfut2+Epifw9dEenT2qXISuesHy9U1H/dM\niwIDAQAB\n-----END PUBLIC KEY-----\n"},
	"rsa2":    {"-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQDaX5Xlv2rbmwqM\nVaX8kS8HHFi0h0aC5H1NeHA51bBz0Ip+G7MebEHAtfik1SY0A5XH+xXd/clHNywu\noHXuVNQ8ZvPJHAcvmLN5MpwPE2Yh7ykieNErqt1D8Zf49M6EfofUbDz2z7NZ2aY+\nkunPexN6hO2yU2D71m9bTRrLu/2Q8i2WedD3n0mfev6xLd/VjYUAtniPSxMSiULV\niVEmmVyyCBc5+a0EqMJhbblf3FtdG1LZe4R7jy38w8D3kSxBdRekFMll4ccyb5Y1\nDfeLmemr8u4Y+2pnivH22DjzmY5q4wKgYnwkK4xyHNx/4FY8/8wIDVS+dKWa+6m8\nAfgu/BS/AgMBAAECggEAFJXiKZBL4HvTGLuSNCbzifYQHqSJ7xje/wiVe747Lg/A\nnna6AeouoJ4G3HMDcO/NlNaEjzK3YLFAxvx1YYw5naAO5yxvOPtqH8i3kWxAun4z\nIH7HKRioVxlVPDDHrxHGeC3j6VhXDedIlql+xQJTKb+SMLdPcy4ZvT+dgQstshYV\nvVrZ4v1E5AIix5fnIqEoRy2+2jzO55PYoU2XjYTIq4+ifJcc5/8Chh3+W1OA6TRF\nA2YJ2Nsa1yetg9FQd0dwgj2uLUj4caoK/gBDzT+gooQCedTWcB3eZqaTeqfRGgTe\noPqwLg9TPECICiPg2XDk43E52DvQChEg96RMxdsCkQKBgQD17HHd4YUQl6FJoUvq\n8/pefkYgf46pz5zJIxifugtfdJvIsl6635ie1Nj3mtvxaZN+dkYLT0SXiFp9haW7\nICMjj8hAbWXgAg3eXMce79MNM8tFxAbvl0NjdmgePTkw8DtI/r5eyJRMB4e2MHn2\n5U2GWGqy/OKocOybUzsicG1phQKBgQDjUijMo5rENQ625YFgmKIHv8PhYrUnXCte\nYjVRvZeYSMOnnFcs22QRxGrNe6nMyp/asyOvwBm0R2OEcI8wy3dBf12BzaxQKjqS\n9rj5CF3H6RYhQK3b/HBkdnPUDVdXhNmWv1apYpKjzPbuAqlLUKmjvCUF+Vvxe+QA\nXxxYLsdWcwKBgErChV8RVtVTcymiCiUz5DHjq0fn3yPKRHx02eB8u0ZaeZ1KdDfV\nD12BkS+8eQ5rrPXt2J9Jiq2SOwgbn+VCAMynwhWr7CNsHHEzvSiveXZy/2GHFSCy\n2D6dWUuQCxnQfgVTw+WmjkaKp3wAQqOG08l6iX0+GV1hbtxNMhXnHavNAoGAQGiC\nCOF+M4O5BJdonf8BQeLS1GSMYXebDiYTZ3KppktPrIj+pkYPR314qXoVdWIgAu5o\n+R4b6lmj192wpKmxw4wb6XBmF1yvtf5UELOPJtpjG87gtAkFq2FoljelfLW2lSDn\npBy64dBsUqnMX/ud5Tek4dlydKyewIAyXsX6UwMCgYEAxQ0Z02/0vmOfN2PFkdr+\nUdVE8gJmsWypC8LFqJmlywK2yr6Yw0MntMfFakrv7mEXk6wzdLeYBHcDr4JvO/r7\nOjo/SkUjoxntGBvG2su8oeiYZqGKhV/AlYdFxPu4PoSiPGORLT0jNnGXoX6Jj1HD\nYS0GygF82QrQefewur05mKo=\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA2l+V5b9q25sKjFWl/JEv\nBxxYtIdGguR9TXhwOdWwc9CKfhuzHmxBwLX4pNUmNAOVx/sV3f3JRzcsLqB17lTU\nPGbzyRwHL5izeTKcDxNmIe8pInjRK6rdQ/GX+PTOhH6H1Gw89s+zWdmmPpLpz3sT\neoTtslNg+9ZvW00ay7v9kPItlnnQ959Jn3r+sS3f1Y2FALZ4j0sTEolC1YlRJplc\nsggXOfmtBKjCYW25X9xbXRtS2XuEe48t/MPA95EsQXUXpBTJZeHHMm+WNQ33i5np\nq/LuGPtqZ4rx9tg485mOauMCoGJ8JCuMchzcf+BWPP/MCA1UvnSlmvupvAH4LvwU\nvwIDAQAB\n-----END PUBLIC KEY-----\n"},
	"p256":    {"-----BEGIN PRIVATE KEY-----\nMIGHAgEAMBMGByqGSM49AgEGCCqGSM49AwEHBG0wawIBAQQgdZudbn82KK6ToN5K\nhguL9VxnTS6RI6YWkiFZsZDOmwShRANCAAQTdSfQcmyCEGbTxpGvMCpqt7+og3xZ\nj8bNz2+LdeS5RASHtjcVTY6annrHSO15nje7cjPweljkgbI1nk1+Clc9\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEE3Un0HJsghBm08aRrzAqare/qIN8\nWY/Gzc9vi3XkuUQEh7Y3FU2Omp56x0jteZ43u3Iz8HpY5IGyNZ5NfgpXPQ==\n-----END PUBLIC KEY-----\n"},
	"p384":    {"-----BEGIN PRIVATE KEY-----\nMIG2AgEAMBAGByqGSM49AgEGBSuBBAAiBIGeMIGbAgEBBDBPQhxZjLS6WFt9TZlD\nxuyqHzRFm6sviHubQrLKIsieoJQLiUHmAMTacr2zf1SnVAKhZANiAAQMPN4G63Ek\nIrDNpcLT1Acg84StnU33I5tOYSp+NmFj6lu52RE5uLfwO3ry3bWnNmHwd2BoqwxB\ni3FVFn8GEuKtGMVy31oOaycAni1P6z4RWzQxQfVEWGaLTumcjO15Qcg=\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEDDzeButxJCKwzaXC09QHIPOErZ1N9yOb\nTmEqfjZhY+pbudkRObi38Dt68t21pzZh8HdgaKsMQYtxVRZ/BhLirRjFct9aDmsn\nAJ4tT+s+EVs0MUH1RFhmi07pnIzteUHI\n-----END PUBLIC KEY-----\n"},
	"p521":    {"-----BEGIN PRIVATE KEY-----\nMIHuAgEAMBAGByqGSM49AgEGBSuBBAAjBIHWMIHTAgEBBEIB3c2vMofya6ZZrTXp\n+5HAiuxs7TInCz80d0s5t4qL0hsW5dxdAG/Kv4F0ZrEdXozDev0oKKcD9uqEZfJd\ncJw1M9OhgYkDgYYABAFACIHpuD0DSNkETYADd4AYFil0XLpNJlVISKNua8lT6q5R\n2/vjdeHXikg7YgsIOFti13HTnHov/8CRO2an0/Bx9AGZL4jE5spdSnpb/QdVoL/K\nzgJp07IOT0jVMqSHutT0WWfYOa2ZxqZnF1lbqKa0LSnWr1x8qqiTx2LrI0bAxyEm\nHA==\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMIGbMBAGByqGSM49AgEGBSuBBAAjA4GGAAQBQAiB6bg9A0jZBE2AA3eAGBYpdFy6\nTSZVSEijbmvJU+quUdv743Xh14pIO2ILCDhbYtdx05x6L//AkTtmp9PwcfQBmS+I\nxObKXUp6W/0HVaC/ys4CadOyDk9I1TKkh7rU9Fln2DmtmcamZxdZW6imtC0p1q9c\nfKqok8di6yNGwMchJhw=\n-----END PUBLIC KEY-----\n"},
	"ed25519": {"-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIC7Z8Ua7ufdSALkYsD/akRduAGTeVMHobs5KcOahwkRv\n-----END PRIVATE KEY-----\n", "-----BEGIN PUBLIC KEY-----\nMCowBQYDK2VwAyEAmtD1+qnxS5ac/r6NLnoHFt+7PAraxfLpVr3bdVEfSwc=\n-----END PUBLIC KEY-----\n"},
}

const hmacSecret = "secret-for-jwt-tests-0123456789abcdef"

func xq(s string) string {
	return strings.ReplaceAll(strconv.Quote(s), "$", `\$`)
}

type alg struct {
	name   string
	method jwt.SigningMethod
	key    string // key name; "hmac" for the shared secret
	other  string // another key of the same family, for a wrong key
	det    bool   // the signature is deterministic
}

var algs = []alg{
	{"HS256", jwt.SigningMethodHS256, "hmac", "hmac2", true},
	{"HS384", jwt.SigningMethodHS384, "hmac", "hmac2", true},
	{"HS512", jwt.SigningMethodHS512, "hmac", "hmac2", true},
	{"RS256", jwt.SigningMethodRS256, "rsa", "rsa2", true},
	{"RS384", jwt.SigningMethodRS384, "rsa", "rsa2", true},
	{"RS512", jwt.SigningMethodRS512, "rsa", "rsa2", true},
	{"PS256", jwt.SigningMethodPS256, "rsa", "rsa2", false},
	{"PS384", jwt.SigningMethodPS384, "rsa", "rsa2", false},
	{"PS512", jwt.SigningMethodPS512, "rsa", "rsa2", false},
	{"ES256", jwt.SigningMethodES256, "p256", "p256", false},
	{"ES384", jwt.SigningMethodES384, "p384", "p384", false},
	{"ES512", jwt.SigningMethodES512, "p521", "p521", false},
	{"EdDSA", jwt.SigningMethodEdDSA, "ed25519", "ed25519", true},
}

func privKey(name string) any {
	if name == "hmac" || name == "hmac2" {
		if name == "hmac2" {
			return []byte("a-different-secret")
		}
		return []byte(hmacSecret)
	}
	b, _ := pem.Decode([]byte(keyPEMs[name][0]))
	k, err := x509.ParsePKCS8PrivateKey(b.Bytes)
	if err != nil {
		panic(err)
	}
	return k
}

func pubKey(name string) any {
	switch name {
	case "hmac":
		return []byte(hmacSecret)
	case "hmac2":
		return []byte("a-different-secret")
	}
	switch k := privKey(name).(type) {
	case *rsa.PrivateKey:
		return &k.PublicKey
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case ed25519.PrivateKey:
		return k.Public()
	}
	panic("key")
}

func sign(a alg, claims jwt.MapClaims, kid string) string {
	t := jwt.NewWithClaims(a.method, claims)
	if kid != "" {
		t.Header["kid"] = kid
	}
	s, err := t.SignedString(privKey(a.key))
	if err != nil {
		panic(err)
	}
	return s
}

type opts struct {
	algs     []string
	now      int64
	leeway   int64
	iss, aud string
	sub      string
	noExp    bool // exp not required
}

func category(token string, keyName string, o opts) string {
	// the header's alg, to tell a disallowed algorithm (golang-jwt calls it
	// an invalid signature) from the rest
	if parts := strings.Split(token, "."); len(parts) == 3 {
		if hb, err := base64.RawURLEncoding.DecodeString(parts[0]); err == nil {
			if i := strings.Index(string(hb), `"alg":"`); i >= 0 {
				rest := string(hb)[i+7:]
				name := rest[:strings.Index(rest, `"`)]
				allowed := false
				for _, a := range o.algs {
					if a == name {
						allowed = true
					}
				}
				if !allowed {
					return "alg"
				}
			}
		}
	}
	popts := []jwt.ParserOption{jwt.WithValidMethods(o.algs), jwt.WithLeeway(time.Duration(o.leeway) * time.Second),
		jwt.WithTimeFunc(func() time.Time { return time.Unix(o.now, 0) })}
	if !o.noExp {
		popts = append(popts, jwt.WithExpirationRequired())
	}
	if o.iss != "" {
		popts = append(popts, jwt.WithIssuer(o.iss))
	}
	if o.aud != "" {
		popts = append(popts, jwt.WithAudience(o.aud))
	}
	if o.sub != "" {
		popts = append(popts, jwt.WithSubject(o.sub))
	}
	_, err := jwt.NewParser(popts...).Parse(token, func(*jwt.Token) (any, error) { return pubKey(keyName), nil })
	switch {
	case err == nil:
		return "ok"
	case errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, jwt.ErrTokenNotValidYet):
		return "notyet"
	case errors.Is(err, jwt.ErrTokenInvalidIssuer):
		return "issuer"
	case errors.Is(err, jwt.ErrTokenInvalidAudience):
		return "audience"
	case errors.Is(err, jwt.ErrTokenInvalidSubject):
		return "subject"
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

func b64(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }

// flip changes the first character of a segment to another valid one.
func flip(seg string) string {
	c := byte('A')
	if seg[0] == 'A' {
		c = 'B'
	}
	return string(c) + seg[1:]
}

func main() {
	var o strings.Builder
	o.WriteString("// Generated by scripts/stdcorpus from github.com/golang-jwt/jwt/v5 v5.3.1 (" + os.Args[1] + "): tokens it signed with fixed keys and the category of the error it gives for each under many options.\n")
	o.WriteString("// Do not edit: regenerate with go run ./scripts/stdcorpus.\n")
	o.WriteString("use std/jwt\nuse std/crypto\n\n")
	o.WriteString("const HMAC_SECRET: Str = " + xq(hmacSecret) + "\n\n")
	o.WriteString("/// (name, private key PEM, public key PEM)\nconst KEYS: List[(Str, Str, Str)] = [\n")
	for _, n := range []string{"rsa", "rsa2", "p256", "p384", "p521", "ed25519"} {
		o.WriteString("  (" + xq(n) + ", " + xq(keyPEMs[n][0]) + ", " + xq(keyPEMs[n][1]) + "),\n")
	}
	o.WriteString("]\n\n")

	base := func() jwt.MapClaims {
		return jwt.MapClaims{"exp": 2000, "iss": "me", "sub": "ann", "aud": []string{"x", "y"}}
	}
	type vcase struct {
		token, key string
		o          opts
		note       string
	}
	var cases []vcase
	for _, a := range algs {
		tok := sign(a, base(), "")
		mk := func(oo opts) opts {
			if oo.algs == nil {
				oo.algs = []string{a.name}
			}
			if oo.now == 0 {
				oo.now = 1000
			}
			return oo
		}
		cases = append(cases,
			vcase{tok, a.key, mk(opts{}), a.name + " ok"},
			vcase{tok, a.key, mk(opts{now: 2000}), a.name + " expired at exp"},
			vcase{tok, a.key, mk(opts{now: 1999}), a.name + " valid before exp"},
			vcase{tok, a.key, mk(opts{now: 2004, leeway: 5}), a.name + " leeway keeps it"},
			vcase{tok, a.key, mk(opts{now: 2005, leeway: 5}), a.name + " leeway ends"},
			vcase{tok, a.key, mk(opts{iss: "me", aud: "y", sub: "ann"}), a.name + " claims match"},
			vcase{tok, a.key, mk(opts{iss: "you"}), a.name + " issuer"},
			vcase{tok, a.key, mk(opts{aud: "z"}), a.name + " audience"},
			vcase{tok, a.key, mk(opts{sub: "bob"}), a.name + " subject"},
			vcase{tok, a.other, mk(opts{}), a.name + " wrong key"},
			vcase{tok, a.key, mk(opts{algs: []string{"HS256", "RS256", "ES256", "EdDSA", "PS256"}}), a.name + " in a list"},
		)
		parts := strings.Split(tok, ".")
		cases = append(cases,
			vcase{parts[0] + "." + flip(parts[1]) + "." + parts[2], a.key, mk(opts{}), a.name + " payload changed"},
			vcase{parts[0] + "." + parts[1] + "." + flip(parts[2]), a.key, mk(opts{}), a.name + " signature changed"},
			vcase{parts[0] + "." + parts[1] + ".", a.key, mk(opts{}), a.name + " no signature"},
		)
		other := "HS256"
		if a.name == "HS256" {
			other = "HS384"
		}
		cases = append(cases, vcase{tok, a.key, mk(opts{algs: []string{other}}), a.name + " not allowed"})
		nb := base()
		nb["nbf"] = 1500
		ntok := sign(a, nb, "")
		cases = append(cases,
			vcase{ntok, a.key, mk(opts{}), a.name + " nbf in the future"},
			vcase{ntok, a.key, mk(opts{leeway: 600}), a.name + " nbf with leeway"},
			vcase{ntok, a.key, mk(opts{now: 1500}), a.name + " nbf now"},
		)
		ne := base()
		delete(ne, "exp")
		etok := sign(a, ne, "")
		cases = append(cases,
			vcase{etok, a.key, mk(opts{}), a.name + " no exp"},
			vcase{etok, a.key, mk(opts{noExp: true}), a.name + " no exp, not required"},
		)
		se := base()
		se["exp"] = "soon"
		cases = append(cases, vcase{sign(a, se, ""), a.key, mk(opts{}), a.name + " exp is a string"})
		sa := base()
		sa["aud"] = "y"
		cases = append(cases, vcase{sign(a, sa, ""), a.key, mk(opts{aud: "y"}), a.name + " aud is a string"})
		fe := base()
		fe["exp"] = 1500.5
		cases = append(cases,
			vcase{sign(a, fe, ""), a.key, mk(opts{now: 1500}), a.name + " fractional exp"},
			vcase{sign(a, fe, ""), a.key, mk(opts{now: 1501}), a.name + " fractional exp passed"},
		)
	}
	hdr := func(s string) string { return b64(s) }
	pl := b64(`{"exp":2000,"sub":"ann"}`)
	hs := alg{"HS256", jwt.SigningMethodHS256, "hmac", "hmac2", true}
	good := sign(hs, base(), "")
	gp := strings.Split(good, ".")
	hmacOpts := opts{algs: []string{"HS256"}, now: 1000}
	cases = append(cases,
		vcase{hdr(`{"alg":"none","typ":"JWT"}`) + "." + pl + ".", "hmac", hmacOpts, "none"},
		vcase{hdr(`{"alg":"NONE","typ":"JWT"}`) + "." + pl + ".", "hmac", hmacOpts, "NONE"},
		vcase{hdr(`{"alg":"none"}`) + "." + pl + "." + gp[2], "hmac", hmacOpts, "none with a signature"},
		vcase{"a.b", "hmac", hmacOpts, "two segments"},
		vcase{good + ".x", "hmac", hmacOpts, "four segments"},
		vcase{"", "hmac", hmacOpts, "empty"},
		vcase{"!!.@@.##", "hmac", hmacOpts, "not base64url"},
		vcase{gp[0] + "=." + gp[1] + "." + gp[2], "hmac", hmacOpts, "padded header"},
		vcase{hdr(`not json`) + "." + pl + "." + gp[2], "hmac", hmacOpts, "header not JSON"},
		vcase{hdr(`["HS256"]`) + "." + pl + "." + gp[2], "hmac", hmacOpts, "header an array"},
		vcase{gp[0] + "." + b64(`[1,2]`) + "." + gp[2], "hmac", hmacOpts, "payload an array"},
		vcase{gp[0] + "." + b64(`not json`) + "." + gp[2], "hmac", hmacOpts, "payload not JSON"},
		vcase{hdr(`{"typ":"JWT"}`) + "." + pl + "." + gp[2], "hmac", hmacOpts, "no alg"},
		vcase{hdr(`{"alg":5}`) + "." + pl + "." + gp[2], "hmac", hmacOpts, "alg a number"},
	)

	o.WriteString("/// (token, key name, allowed algs, now, leeway, iss, aud, sub, exp required, the category golang-jwt gives, note)\n")
	o.WriteString("const VERIFY: List[(Str, Str, Str, Int, Int, Str, Str, Str, Bool, Str, Str)] = [\n")
	for _, c := range cases {
		want := category(c.token, c.key, c.o)
		if strings.HasPrefix(want, "other") {
			fmt.Fprintf(os.Stderr, "jwtoracle: %s: %s\n", c.note, want)
			os.Exit(1)
		}
		o.WriteString(fmt.Sprintf("  (%s, %s, %s, %d, %d, %s, %s, %s, %v, %s, %s),\n", xq(c.token), xq(c.key), xq(strings.Join(c.o.algs, ",")), c.o.now, c.o.leeway, xq(c.o.iss), xq(c.o.aud), xq(c.o.sub), !c.o.noExp, xq(want), xq(c.note)))
	}
	o.WriteString("]\n\n")

	// tokens Xo must reproduce byte for byte: deterministic signatures
	o.WriteString("/// (alg, key name, kid, the token golang-jwt signs for {exp: 2000, iss: me, sub: ann})\nconst SIGNED: List[(Str, Str, Str, Str)] = [\n")
	for _, a := range algs {
		if !a.det {
			continue
		}
		for _, kid := range []string{"", "k1"} {
			c := jwt.MapClaims{"exp": 2000, "iss": "me", "sub": "ann"}
			o.WriteString(fmt.Sprintf("  (%s, %s, %s, %s),\n", xq(a.name), xq(a.key), xq(kid), xq(sign(a, c, kid))))
		}
	}
	o.WriteString("]\n\n")
	o.WriteString(jwtTests)
	fmt.Print(o.String())
}

const jwtTests = `
struct SignClaims {
  exp: Int
  iss: Str
  sub: Str
} derive Json

struct Loose {
  exp: Float?
  iss: Str?
  sub: Str?
} derive Json

fn alg_of(s: Str) -> jwt.Alg {
  match s {
    "HS256" => jwt.Alg.HS256
    "HS384" => jwt.Alg.HS384
    "HS512" => jwt.Alg.HS512
    "RS256" => jwt.Alg.RS256
    "RS384" => jwt.Alg.RS384
    "RS512" => jwt.Alg.RS512
    "PS256" => jwt.Alg.PS256
    "PS384" => jwt.Alg.PS384
    "PS512" => jwt.Alg.PS512
    "ES256" => jwt.Alg.ES256
    "ES384" => jwt.Alg.ES384
    "ES512" => jwt.Alg.ES512
    "EdDSA" => jwt.Alg.EdDSA
    _ => fault("unknown alg ${s}")
  }
}

fn pem_der(text: Str) -> Bytes {
  let blocks = crypto.pem_decode(text)
  blocks[0].bytes
}

fn public_key(name: Str) -> jwt.Key {
  if name == "hmac" {
    return jwt.Key.Hmac(secret: Secret.new(HMAC_SECRET.bytes()))
  }
  if name == "hmac2" {
    return jwt.Key.Hmac(secret: Secret.new(b"a-different-secret"))
  }
  for (n, _priv, pubpem) in KEYS {
    if n == name {
      return jwt.Key.Public(key: crypto.parse_public_key(pem_der(pubpem)).ok() ?? fault("public key"))
    }
  }
  fault("no key ${name}")
}

fn private_key(name: Str) -> jwt.Key {
  if name == "hmac" {
    return jwt.Key.Hmac(secret: Secret.new(HMAC_SECRET.bytes()))
  }
  for (n, priv, _pub) in KEYS {
    if n == name {
      return jwt.Key.Private(key: crypto.parse_private_key(pem_der(priv)).ok() ?? fault("private key"))
    }
  }
  fault("no key ${name}")
}

fn category_of(e: jwt.JwtErr) -> Str {
  match e {
    Malformed(_) => "malformed"
    Alg(_) => "alg"
    Signature => "signature"
    Expired(_) => "expired"
    NotYetValid(_) => "notyet"
    Issuer => "issuer"
    Audience => "audience"
    Subject => "subject"
    Claim(_) => "claim"
    Key(_) => "key"
  }
}

test "std/jwt answers golang-jwt's category for every token and option" {
  var bad: List[(Str, Str, Str)] = []
  for (token, key, algs, now, leeway, iss, aud, sub, need_exp, want, note) in VERIFY {
    var o = jwt.Options{algs: algs.split(",").map(fn(a) { alg_of(a) }), now: Time.UNIX_EPOCH + Duration.from_secs_int(now), leeway: Duration.from_secs_int(leeway), require_exp: need_exp}
    if iss != "" {
      o.issuer = Some(iss)
    }
    if aud != "" {
      o.audience = Some(aud)
    }
    if sub != "" {
      o.subject = Some(sub)
    }
    let got = match jwt.verify_with[Loose](token, public_key(key), o) {
      Ok(_) => "ok"
      Err(e) => category_of(e)
    }
    if got != want {
      bad.push((note, got, want))
    }
  }
  expect(bad[0..(if bad.len() > 40 { 40 } else { bad.len() })].map(fn(b) { "${b.0}: ${b.1} want ${b.2}" })) == []
}

test "std/jwt signs the tokens golang-jwt signs, byte for byte" {
  var bad: List[(Str, Str, Str)] = []
  for (alg, key, kid, want) in SIGNED {
    let kid_opt = if kid == "" { None } else { Some(kid) }
    let got = match jwt.sign(os.rand, alg_of(alg), private_key(key), SignClaims{exp: 2000, iss: "me", sub: "ann"}, kid_opt) {
      Ok(t) => t
      Err(e) => "!error ${e.display()}"
    }
    if got != want {
      bad.push((alg, got, want))
    }
  }
  expect(bad) == []
}

test "std/jwt verifies what it signs under every algorithm" {
  for a in ["HS256", "HS384", "HS512", "RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"] {
    let key = if a.starts_with("HS") { "hmac" } else if a.starts_with("ES") { "p" + (if a == "ES256" { "256" } else if a == "ES384" { "384" } else { "521" }) } else if a == "EdDSA" { "ed25519" } else { "rsa" }
    let tok = jwt.sign(os.rand, alg_of(a), private_key(key), SignClaims{exp: 2000, iss: "me", sub: "ann"}, None).ok() ?? fault("sign ${a}")
    let o = jwt.Options{algs: [alg_of(a)], now: Time.UNIX_EPOCH + Duration.from_secs_int(1000)}
    let pk = if a.starts_with("HS") { public_key("hmac") } else { public_key(key) }
    let got = jwt.verify_with[SignClaims](tok, pk, o).ok() ?? fault("verify ${a}")
    expect(got.sub) == "ann"
  }
}

test "std/jwt refuses a key that does not fit the algorithm and picks keys by kid" {
  let rsa = public_key("rsa")
  let o = jwt.Options{algs: [jwt.Alg.HS256, jwt.Alg.RS256], now: Time.UNIX_EPOCH + Duration.from_secs_int(1000)}
  let tok = jwt.sign(os.rand, jwt.Alg.RS256, private_key("rsa"), SignClaims{exp: 2000, iss: "me", sub: "ann"}, Some("r1")).ok() ?? fault("sign")
  // a token claiming HS256 and signed with an RSA public key's bytes as the secret
  let forged = jwt.sign(os.rand, jwt.Alg.HS256, jwt.Key.Hmac(secret: Secret.new(KEYS[0].2.bytes())), SignClaims{exp: 2000, iss: "me", sub: "ann"}, None).ok() ?? fault("forge")
  match jwt.verify_with[SignClaims](forged, rsa, o) {
    Err(jwt.JwtErr.Alg(_)) => ()
    _ => fault("an HS256 token against an RSA key must be refused")
  }
  match jwt.verify[SignClaims](tok, fn(h) { if h.kid == Some("r1") { Ok(rsa) } else { Err(jwt.JwtErr.Key(msg: "unknown kid")) } }, o) {
    Ok(c) => expect(c.iss) == "me"
    Err(e) => fault("kid lookup: ${e.display()}")
  }
  let bad = jwt.sign(os.rand, jwt.Alg.RS256, private_key("rsa"), SignClaims{exp: 2000, iss: "me", sub: "ann"}, Some("zz")).ok() ?? fault("sign")
  match jwt.verify[SignClaims](bad, fn(h) { if h.kid == Some("r1") { Ok(rsa) } else { Err(jwt.JwtErr.Key(msg: "unknown kid")) } }, o) {
    Err(jwt.JwtErr.Key(_)) => ()
    _ => fault("an unknown kid must be a Key error")
  }
}

test "std/jwt reads a key set and peeks at headers" {
  let es = "{\"keys\":[{\"kty\":\"EC\",\"crv\":\"P-256\",\"kid\":\"e1\",\"x\":\"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU\",\"y\":\"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0\"}]}"
  let set = jwt.jwks(es).ok() ?? fault("jwks")
  expect(set.len()) == 1
  expect(set.contains("e1")) == true
  expect(jwt.jwks("{}").is_err()) == true
  expect(jwt.jwks("{\"keys\":[{\"kty\":\"EC\",\"crv\":\"P-256\"}]}").is_err()) == true
  let tok = jwt.sign(os.rand, jwt.Alg.HS256, private_key("hmac"), SignClaims{exp: 2000, iss: "me", sub: "ann"}, Some("k9")).ok() ?? fault("sign")
  let h = jwt.peek_header(tok).ok() ?? fault("peek")
  expect(h.kid) == Some("k9")
  expect(h.typ) == Some("JWT")
  expect(jwt.peek_header("x.y").is_err()) == true
}
`
