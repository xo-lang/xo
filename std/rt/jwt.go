package rt

// The display of std/jwt's error (decision 0486): the runtime calls it
// through an interface, so it stays a Go method; the texts are
// std/native/jwt.xo's jwterr_display.

func (e Jwt_JwtErr) M_display(*Task) string {
	switch e.Tag {
	case 0:
		return "jwt: malformed token: " + e.P.(Jwt_JwtErr_Malformed).F_msg
	case 1:
		return "jwt: signing method: " + e.P.(Jwt_JwtErr_Alg).F_msg
	case 2:
		return "jwt: signature is invalid"
	case 3:
		return "jwt: token is expired"
	case 4:
		return "jwt: token is not valid yet"
	case 5:
		return "jwt: token has invalid issuer"
	case 6:
		return "jwt: token has invalid audience"
	case 7:
		return "jwt: token has invalid subject"
	case 8:
		return "jwt: token has invalid claims: " + e.P.(Jwt_JwtErr_Claim).F_msg
	}
	return "jwt: key: " + e.P.(Jwt_JwtErr_Key).F_msg
}
