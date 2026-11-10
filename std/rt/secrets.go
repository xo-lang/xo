package rt

import (
	"strconv"
	"sync"
	"syscall"
)

// The user's secret stores (decision 0584): the Go backend's twins of
// std/native/llrt/userfs.xo's RealSecrets, RealKeyring and NoSecrets.

func (v KeyErr) M_display(*Task) string {
	switch v.Tag {
	case 1:
		return "key not found"
	case 2:
		return "key expired"
	case 3:
		return "key revoked"
	case 4:
		return "permission denied"
	case 5:
		return v.P.(KeyErr_Io).F_msg
	}
	return ""
}

// keyErrOf maps errno e as userfs.xo's key_err does.
func keyErrOf(e syscall.Errno) KeyErr {
	switch int(e) {
	case 2, 126:
		return KeyErr{Tag: 1}
	case 127:
		return KeyErr{Tag: 2}
	case 128:
		return KeyErr{Tag: 3}
	case 1, 13:
		return KeyErr{Tag: 4}
	case 38:
		return KeyErr{Tag: 5, P: KeyErr_Io{F_msg: "the kernel keyring is not available"}}
	}
	return KeyErr{Tag: 5, P: KeyErr_Io{F_msg: "errno " + strconv.Itoa(int(e))}}
}

// realSecrets is os.secrets; fake (a test's, decision 0584 stage C) keeps
// its keys in memory on every OS.
type realSecrets struct {
	ring bool
	fake *fakeKeyring
}

func (s realSecrets) M_keyring(*Task) Option[Keyring] {
	if s.ring && s.fake != nil {
		return Some[Keyring](s.fake)
	}
	if s.ring && keyringAvailable {
		return Some[Keyring](realKeyring{})
	}
	return None[Keyring]()
}

func (s realSecrets) M_only(_ *Task, stores List[string]) Secrets {
	ring := false
	for _, x := range stores.s {
		if x == "keyring" {
			ring = true
		}
	}
	return realSecrets{ring: s.ring && ring, fake: s.fake}
}

type noSecrets struct{}

func (noSecrets) M_keyring(*Task) Option[Keyring]      { return None[Keyring]() }
func (s noSecrets) M_only(*Task, List[string]) Secrets { return s }

// NoAccess_secrets is the Secrets an Os literal without `secrets` holds
// (decisions 0584, 0585).
func NoAccess_secrets() Secrets { return noSecrets{} }

type realKeyring struct{}

func keyID(r int64, e syscall.Errno) Result[int64, KeyErr] {
	if e != 0 {
		return Err[int64](keyErrOf(e))
	}
	return Ok[int64, KeyErr](r)
}

func (realKeyring) M_user_persistent(*Task) Result[int64, KeyErr] {
	return keyID(keyUserPersistent())
}

func (realKeyring) M_add(_ *Task, ring int64, kind, desc string, payload Bytes) Result[int64, KeyErr] {
	return keyID(addKey(kind, desc, []byte(payload), ring))
}

func (realKeyring) M_search(_ *Task, ring int64, kind, desc string) Result[int64, KeyErr] {
	return keyID(keyctlSearchKey(ring, kind, desc))
}

func (realKeyring) M_read(_ *Task, id int64) Result[Bytes, KeyErr] {
	b, e := keyctlReadKey(id)
	if e != 0 {
		return Err[Bytes](keyErrOf(e))
	}
	return Ok[Bytes, KeyErr](Bytes(b))
}

func (realKeyring) M_unlink(_ *Task, id, ring int64) Result[Unit, KeyErr] {
	if _, e := keyctl(keyctlUnlink, uintptr(id), uintptr(ring), 0, 0); e != 0 {
		return Err[Unit](keyErrOf(e))
	}
	return Ok[Unit, KeyErr](Unit{})
}

// fakeKeyring is a test's keyring (testworld.xo key_* on the native
// backends): ring 7, ids from 1000, ENOKEY when absent.
type fakeKeyring struct {
	mu   sync.Mutex
	next int64
	keys []fakeKey
}

type fakeKey struct {
	id, ring   int64
	kind, desc string
	data       Bytes
}

const fakeKeyRing = 7

func (k *fakeKeyring) find(ring int64, kind, desc string) int {
	for i, x := range k.keys {
		if x.ring == ring && x.kind == kind && x.desc == desc {
			return i
		}
	}
	return -1
}

func (k *fakeKeyring) M_user_persistent(*Task) Result[int64, KeyErr] {
	return Ok[int64, KeyErr](fakeKeyRing)
}

func (k *fakeKeyring) M_add(_ *Task, ring int64, kind, desc string, payload Bytes) Result[int64, KeyErr] {
	k.mu.Lock()
	defer k.mu.Unlock()
	if i := k.find(ring, kind, desc); i >= 0 {
		k.keys[i].data = payload
		return Ok[int64, KeyErr](k.keys[i].id)
	}
	id := 1000 + k.next
	k.next++
	k.keys = append(k.keys, fakeKey{id: id, ring: ring, kind: kind, desc: desc, data: payload})
	return Ok[int64, KeyErr](id)
}

func (k *fakeKeyring) M_search(_ *Task, ring int64, kind, desc string) Result[int64, KeyErr] {
	k.mu.Lock()
	defer k.mu.Unlock()
	if i := k.find(ring, kind, desc); i >= 0 {
		return Ok[int64, KeyErr](k.keys[i].id)
	}
	return Err[int64](KeyErr{Tag: 1})
}

func (k *fakeKeyring) M_read(_ *Task, id int64) Result[Bytes, KeyErr] {
	k.mu.Lock()
	defer k.mu.Unlock()
	for _, x := range k.keys {
		if x.id == id {
			return Ok[Bytes, KeyErr](x.data)
		}
	}
	return Err[Bytes](KeyErr{Tag: 1})
}

func (k *fakeKeyring) M_unlink(_ *Task, id, ring int64) Result[Unit, KeyErr] {
	k.mu.Lock()
	defer k.mu.Unlock()
	for i, x := range k.keys {
		if x.id == id && x.ring == ring {
			k.keys = append(k.keys[:i], k.keys[i+1:]...)
			return Ok[Unit, KeyErr](Unit{})
		}
	}
	return Err[Unit](KeyErr{Tag: 1})
}
