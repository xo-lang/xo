package rt

import "strconv"

// std/bcrypt on the Go backend: compiled from std/native/bcrypt.xo
// (std/go_compiled.txt, decision 0312). The display method stays here, as
// the runtime calls it through interfaces.

// M_display is Go's error text (std/native/bcrypt.xo bcrypterr_display).
func (e Bcrypt_BcryptErr) M_display(*Task) string {
	switch e.Tag {
	case 1:
		return "crypto/bcrypt: hashedPassword is not the hash of the given password"
	case 2:
		return "crypto/bcrypt: hashedSecret too short to be a bcrypted password"
	case 3:
		return "crypto/bcrypt: bcrypt algorithm version '" + string(rune(e.P.(Bcrypt_BcryptErr_VersionTooNew).F_version)) + "' requested is newer than current version '2'"
	case 4:
		return "crypto/bcrypt: bcrypt hashes must start with '$', but hashedSecret started with '" + string(rune(e.P.(Bcrypt_BcryptErr_InvalidPrefix).F_byte)) + "'"
	case 5:
		return "crypto/bcrypt: cost " + strconv.FormatInt(e.P.(Bcrypt_BcryptErr_InvalidCost).F_cost, 10) + " is outside allowed inclusive range 4..31"
	case 6:
		return "bcrypt: password length exceeds 72 bytes"
	}
	return e.P.(Bcrypt_BcryptErr_Corrupt).F_msg
}
