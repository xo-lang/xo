package rt

import (
	"bytes"
	"crypto/tls"
	"testing"
)

// TestTicketsAsGo checks the session tickets of tlsticket.xo's Go copy
// against crypto/tls: a ticket Go's Config.EncryptTicket seals under a
// key set with SetSessionTicketKeys opens with t13sTicketOpen and the
// key t13sKeyFrom makes of the same seed (Go's ticketKeyFromBytes), and
// its state parses; one sealed here, Go's DecryptTicket opens.
func TestTicketsAsGo(t *testing.T) {
	var seed [32]byte
	for i := range seed {
		seed[i] = byte(i + 1)
	}
	cfg := &tls.Config{}
	cfg.SetSessionTicketKeys([][32]byte{seed})
	state := t13sStateBytes(0x0304, 0x1301, 1800000000, bytes.Repeat([]byte{7}, 32), false, 0)
	ss, err := tls.ParseSessionState(state)
	if err != nil {
		t.Fatal(err)
	}
	ticket, err := cfg.EncryptTicket(tls.ConnectionState{}, ss)
	if err != nil {
		t.Fatal(err)
	}
	keys := []t13sTicketKey{t13sKeyFrom(seed[:], 0)}
	plain, ok := t13sTicketOpen(ticket, keys)
	if !ok {
		t.Fatal("Go's ticket does not open with t13sKeyFrom's key")
	}
	if !bytes.Equal(plain, state) {
		t.Fatalf("state %x, want %x", plain, state)
	}
	if st, ok := t13sParseState(plain); !ok || st.suite != 0x1301 || st.created != 1800000000 {
		t.Fatalf("parse: %+v %v", st, ok)
	}
	mine := t13sTicketSeal(keys[0], state, bytes.Repeat([]byte{9}, 16))
	back, err := cfg.DecryptTicket(mine, tls.ConnectionState{})
	if err != nil || back == nil {
		t.Fatalf("Go does not open t13sTicketSeal's ticket: %v", err)
	}
	if b, _ := back.Bytes(); !bytes.Equal(b, state) {
		t.Fatalf("Go's state %x, want %x", b, state)
	}
}

// TestHelloExtensionsAsGo checks that t13sParseHello refuses the
// ClientHello extensions crypto/tls's clientHelloMsg.unmarshal refuses
// (a decode_error) and keeps the ones it keeps, as tls13srv.xo's
// extension does. Task #209: a second ClientHello whose ec_point_formats
// was 69 00 (a length of 105 over one byte) drew illegal_parameter on
// the Go backend; Go 1.26's server sends decode_error, as native Xo did.
func TestHelloExtensionsAsGo(t *testing.T) {
	cases := []struct {
		name string
		ext  int
		data []byte
		ok   bool
	}{
		{"points", 11, []byte{1, 0}, true},
		{"points length past the end", 11, []byte{0x69, 0}, false},
		{"points empty vector", 11, []byte{0}, false},
		{"points no vector", 11, nil, false},
		{"points trailing byte", 11, []byte{1, 0, 0}, false},
		{"status_request", 5, []byte{1, 0, 0, 0, 0}, true},
		{"status_request vectors", 5, []byte{1, 0, 1, 7, 0, 2, 8, 9}, true},
		{"status_request short", 5, []byte{1, 0, 0, 0}, false},
		{"status_request trailing byte", 5, []byte{1, 0, 0, 0, 0, 0}, false},
		{"sct", 18, nil, true},
		{"sct with data", 18, []byte{0}, false},
		{"extended master secret", 23, nil, true},
		{"extended master secret with data", 23, []byte{0}, false},
	}
	for _, c := range cases {
		body := t13U16(nil, 0x0303)
		body = append(body, make([]byte, 32)...)
		body = append(body, 0)                      // no session ID
		body = append(body, 0, 2, 0x13, 0x01, 1, 0) // one suite, null compression
		exts := t13Ext(nil, 43, []byte{2, 3, 4})
		exts = t13Ext(exts, c.ext, c.data)
		m := t13Message(1, append(body, t13Vec16(exts)...))
		if got := t13sParseHello(m).ok; got != c.ok {
			t.Errorf("%s: ok %v, want %v (crypto/tls)", c.name, got, c.ok)
		}
	}
}
