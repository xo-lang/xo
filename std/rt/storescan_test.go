package rt

import (
	"fmt"
	"strings"
	"testing"
)

// The store of bench/perf/http (task 01) scans every user under the
// write lock (`st.users.values().any(...)`) on create and update. These
// benchmarks time that scan on a map in the state the load leaves it in:
// 64 preloaded users plus 32 in flight, after create, update, and delete
// churn (so the log holds overwritten entries and tombstones).

type scanUser struct {
	id          int64
	name, email string
	nick        Option[string]
}

func churnedUsers(b *testing.B) Map[int64, scanUser] {
	var m Map[int64, scanUser]
	id := int64(1)
	put := func() int64 {
		u := scanUser{id: id, name: "Worker", email: fmt.Sprintf("w%d@x.io", id)}
		Map_put(nil, &m, id, u)
		id++
		return u.id
	}
	for range 64 {
		put()
	}
	var live []int64
	for i := range 2000 {
		k := put()
		u := Map_get(nil, m, k).V
		u.nick = Some("nick")
		Map_put(nil, &m, k, u)
		live = append(live, k)
		if len(live) > 32 || i%3 == 0 {
			Map_remove(nil, &m, live[0])
			live = live[1:]
		}
	}
	if m.n < 64 {
		b.Fatalf("map has %d users", m.n)
	}
	return m
}

func sameEmail(a, b string) bool { return Str_eq_lower(a, b) }

// TestStrEqLower: Str_eq_lower(a, b) is ToLower(a) == ToLower(b) for every
// pair of a set of tricky strings (case, length, Unicode that lowers to
// ASCII or changes length, invalid UTF-8) and their concatenations.
func TestStrEqLower(t *testing.T) {
	base := []string{"", "a", "A", "k", "K", "K", "ab", "aB", "Ab@X.io", "ab@x.io", "İ", "i̇", "i",
		"ß", "SS", "ss", "\xff", "\xfe", "Σ", "σ", "ς", "é", "É", "é", "w1@x.io", "W1@X.IO", "w1@x.io "}
	var all []string
	for _, a := range base {
		for _, b := range base {
			all = append(all, a+b)
		}
	}
	for _, a := range all {
		for _, b := range all {
			want := strings.ToLower(a) == strings.ToLower(b)
			if got := Str_eq_lower(a, b); got != want {
				t.Fatalf("Str_eq_lower(%q, %q) = %v, want %v", a, b, got, want)
			}
		}
	}
}

func BenchmarkStoreScanValues(b *testing.B) {
	m := churnedUsers(b)
	b.ReportAllocs()
	for b.Loop() {
		found := false
		for _, u := range List_Items(Map_values(nil, m)) {
			if u.id != 0 && sameEmail(u.email, "nobody@x.io") {
				found = true
				break
			}
		}
		if found {
			b.Fatal("found")
		}
	}
}

func BenchmarkStoreScanAll(b *testing.B) {
	m := churnedUsers(b)
	b.ReportAllocs()
	for b.Loop() {
		found := false
		for _, u := range Map_All(m) {
			if u.id != 0 && sameEmail(u.email, "nobody@x.io") {
				found = true
				break
			}
		}
		if found {
			b.Fatal("found")
		}
	}
}

// BenchmarkMutexWithUncontended is the fixed cost of one Mutex.with or
// Mutex.with_read around an empty closure, most of it inside the lock.
func BenchmarkMutexWithUncontended(b *testing.B) {
	tk := NewRootTask()
	m := Mutex_new(tk, Map[int64, scanUser]{})
	b.ReportAllocs()
	for b.Loop() {
		Mutex_with(tk, m, func(*Task, *Map[int64, scanUser]) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) })
	}
}

func BenchmarkMutexWithReadUncontended(b *testing.B) {
	tk := NewRootTask()
	m := Mutex_new(tk, Map[int64, scanUser]{})
	b.ReportAllocs()
	for b.Loop() {
		Mutex_with_read(tk, m, func(*Task, Map[int64, scanUser]) Result[Unit, Never] { return Ok[Unit, Never](Unit{}) })
	}
}

// BenchmarkStoreScanChurn interleaves the scan with the churn of one load
// cycle (create, update, delete), so it sees the log at every garbage
// level between two flattens.
func BenchmarkStoreScanChurn(b *testing.B) {
	m := churnedUsers(b)
	id := int64(1 << 20)
	var live []int64
	b.ReportAllocs()
	for b.Loop() {
		found := false
		for u := range Map_values_seq(m) {
			if u.id != 0 && sameEmail(u.email, "nobody@x.io") {
				found = true
				break
			}
		}
		if found {
			b.Fatal("found")
		}
		Map_put(nil, &m, id, scanUser{id: id, name: "Worker", email: "w@x.io"})
		u := Map_get(nil, m, id).V
		u.nick = Some("nick")
		Map_put(nil, &m, id, u)
		live = append(live, id)
		id++
		if len(live) > 32 {
			Map_remove(nil, &m, live[0])
			live = live[1:]
		}
	}
}

func BenchmarkStoreScanEachOnly(b *testing.B) {
	m := churnedUsers(b)
	for b.Loop() {
		n := 0
		Map_each(&m, func(_ *mentry[int64, scanUser], _ scanUser) bool { n++; return true })
	}
}
