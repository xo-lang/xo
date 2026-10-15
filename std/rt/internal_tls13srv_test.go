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
