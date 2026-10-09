package rt

import (
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// std/time zones (std.md 13.1, decision 0094). Zone data is embedded per
// use: the generated package main registers the TZif data of every
// constant zone name (TZEmbed), the whole zoneinfo.zip of the Go
// toolchain when a name is known only at run time (TZFull), or asks for
// the host database (TZUseSystem, `--tzdata=system`). `utc` and `fixed`
// need no data.

var (
	tzMu       sync.Mutex
	tzEmbedded = map[string]string{} // IANA name -> TZif data
	tzZip      string                // zoneinfo.zip (stored, uncompressed)
	tzSystem   bool
	tzCache    = map[string]*time.Location{}
)

// TZEmbed registers the TZif data of one zone (generated init code).
func TZEmbed(name, data string) {
	tzMu.Lock()
	tzEmbedded[name] = data
	tzMu.Unlock()
}

// TZFull registers the whole zone database: Go's lib/time/zoneinfo.zip.
func TZFull(zip string) {
	tzMu.Lock()
	tzZip = zip
	tzMu.Unlock()
}

// TZUseSystem makes zone lookups read the host database
// (`--tzdata=system`).
func TZUseSystem() {
	tzMu.Lock()
	tzSystem = true
	tzMu.Unlock()
}

// Time_Zone is a zone handle (std.md 13.1). The zero value is UTC.
type Time_Zone struct{ z *zoneData }

type zoneData struct {
	name  string
	loc   *time.Location
	fixed bool
	off   int64 // seconds, fixed zones only
}

var utcZone = &zoneData{name: "UTC", loc: time.UTC}

func (z Time_Zone) data() *zoneData {
	if z.z == nil {
		return utcZone
	}
	return z.z
}

func (z Time_Zone) XoEq(x any) bool {
	o, ok := x.(Time_Zone)
	if !ok {
		return false
	}
	a, b := z.data(), o.data()
	return a.name == b.name && a.fixed == b.fixed && a.off == b.off
}

func (z Time_Zone) XoHash() uint64 {
	return MixHash(Hash(z.data().name), uint64(z.data().off))
}

func (z Time_Zone) XoDebug(b *strings.Builder) {
	b.WriteString("Zone(")
	b.WriteString(z.data().name)
	b.WriteString(")")
}

func (z Time_Zone) M_debug(*Task) string { return Debug(z) }

// M_display is the zone's name.
func (z Time_Zone) M_display(*Task) string { return z.data().name }

// Time_utc is `time.utc()`.
func Time_utc(*Task) Time_Zone { return Time_Zone{utcZone} }

// Time_zone is `time.zone(name)`: an IANA zone from the embedded data.
func Time_zone(_ *Task, name string) Result[Time_Zone, Time_ZoneErr] {
	if loc, ok := loadZone(name); ok {
		return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: loc}})
	}
	return Err[Time_Zone](Time_ZoneErr{Tag: 1, P: Time_ZoneErr_Unknown{F_name: name}})
}

// loadZone finds the rules of an IANA zone: the embedded zone, the
// embedded database, or (with --tzdata=system) the host database.
func loadZone(name string) (*time.Location, bool) {
	if name == "UTC" {
		return time.UTC, true
	}
	if !validZoneName(name) {
		return nil, false
	}
	tzMu.Lock()
	defer tzMu.Unlock()
	if loc, ok := tzCache[name]; ok {
		return loc, true
	}
	var loc *time.Location
	var err error
	switch {
	case tzEmbedded[name] != "":
		loc, err = time.LoadLocationFromTZData(name, []byte(tzEmbedded[name]))
	case tzZip != "":
		data, ok := zipEntry(tzZip, name)
		if !ok {
			return nil, false
		}
		loc, err = time.LoadLocationFromTZData(name, []byte(data))
	case tzSystem:
		loc, err = time.LoadLocation(name)
	default:
		return nil, false
	}
	if err != nil || loc == nil {
		return nil, false
	}
	tzCache[name] = loc
	return loc, true
}

// validZoneName rejects names that are not IANA zone names: "Local",
// paths, and empty names (time.LoadLocation accepts "Local").
func validZoneName(name string) bool {
	if name == "" || name == "Local" || len(name) > 255 || strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.ContainsAny(name, "\\\x00") {
		return false
	}
	return true
}

// zipEntry returns the content of a stored (uncompressed) entry of a zip
// archive, the format of Go's lib/time/zoneinfo.zip. A dependency free
// reader, like the one in Go's time package, so the runtime does not link
// archive/zip.
func zipEntry(z, name string) (string, bool) {
	const eocdLen = 22
	if len(z) < eocdLen {
		return "", false
	}
	end := z[len(z)-eocdLen:]
	if binary.LittleEndian.Uint32([]byte(end[0:4])) != 0x06054b50 {
		return "", false
	}
	n := int(binary.LittleEndian.Uint16([]byte(end[10:12])))
	off := int(binary.LittleEndian.Uint32([]byte(end[16:20])))
	for i := 0; i < n; i++ {
		if off+46 > len(z) {
			return "", false
		}
		h := []byte(z[off : off+46])
		if binary.LittleEndian.Uint32(h[0:4]) != 0x02014b50 {
			return "", false
		}
		method := binary.LittleEndian.Uint16(h[10:12])
		size := int(binary.LittleEndian.Uint32(h[24:28]))
		nameLen := int(binary.LittleEndian.Uint16(h[28:30]))
		extraLen := int(binary.LittleEndian.Uint16(h[30:32]))
		commentLen := int(binary.LittleEndian.Uint16(h[32:34]))
		local := int(binary.LittleEndian.Uint32(h[42:46]))
		if off+46+nameLen > len(z) {
			return "", false
		}
		entry := z[off+46 : off+46+nameLen]
		off += 46 + nameLen + extraLen + commentLen
		if entry != name {
			continue
		}
		if method != 0 || local+30 > len(z) {
			return "", false
		}
		lh := []byte(z[local : local+30])
		if binary.LittleEndian.Uint32(lh[0:4]) != 0x04034b50 {
			return "", false
		}
		start := local + 30 + int(binary.LittleEndian.Uint16(lh[26:28])) + int(binary.LittleEndian.Uint16(lh[28:30]))
		if start+size > len(z) {
			return "", false
		}
		return z[start : start+size], true
	}
	return "", false
}

// Time_fixed is `time.fixed(name, offset)`: one offset forever.
func Time_fixed(_ *Task, name string, offset Duration) Time_Zone {
	if offset > Duration(24*time.Hour) || offset < -Duration(24*time.Hour) {
		FaultHere("time.fixed: offset %s is past 24h", offset)
	}
	secs := int64(offset) / 1e9
	return Time_Zone{&zoneData{name: name, loc: time.FixedZone(name, int(secs)), fixed: true, off: secs}}
}

// Time_local is `time.local(env)`: the zone named by TZ, then the system
// setting. A fake Env without TZ gives UTC.
func Time_local(tk *Task, env Env) Result[Time_Zone, Time_ZoneErr] {
	if tz := env.M_get(tk, "TZ"); tz.Some {
		name := strings.TrimPrefix(tz.V, ":")
		if name == "" || name == "UTC" {
			return Ok[Time_Zone, Time_ZoneErr](Time_Zone{utcZone})
		}
		if loc, ok := loadZone(name); ok {
			return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: loc}})
		}
		// the machine's own zone is host dependent anyway: the host
		// database is the fallback
		if loc, err := time.LoadLocation(name); err == nil && validZoneName(name) {
			return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: loc}})
		}
		return Err[Time_Zone](Time_ZoneErr{Tag: 1, P: Time_ZoneErr_Unknown{F_name: name}})
	}
	if _, real := env.(realEnv); !real {
		return Ok[Time_Zone, Time_ZoneErr](Time_Zone{utcZone})
	}
	name := "Local"
	if p, err := os.Readlink("/etc/localtime"); err == nil {
		if i := strings.Index(p, "zoneinfo/"); i >= 0 {
			name = p[i+len("zoneinfo/"):]
		}
	}
	if name != "Local" {
		if loc, ok := loadZone(name); ok {
			return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: loc}})
		}
		if loc, err := time.LoadLocation(name); err == nil {
			return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: loc}})
		}
	}
	if time.Local == nil {
		return Err[Time_Zone](Time_ZoneErr{Tag: 2, P: Time_ZoneErr_NoLocal{F_msg: "the system zone is not set"}})
	}
	return Ok[Time_Zone, Time_ZoneErr](Time_Zone{&zoneData{name: name, loc: time.Local}})
}

// M_name is `z.name()`.
func (z Time_Zone) M_name(*Task) string { return z.data().name }

// M_offset_at is `z.offset_at(t)`.
func (z Time_Zone) M_offset_at(_ *Task, t Time) Duration {
	return Duration(z.offsetSecs(int64(t)) * 1e9)
}

// M_abbreviation_at is `z.abbreviation_at(t)`.
func (z Time_Zone) M_abbreviation_at(_ *Task, t Time) string {
	abbr, _ := goTime(t, z).Zone()
	return abbr
}

// goTime is t in zone z.
func goTime(t Time, z Time_Zone) time.Time {
	return time.Unix(0, int64(t)).In(z.data().loc)
}

// offsetSecs is the offset of z at the instant ns (nanoseconds since the
// epoch), in seconds.
func (z Time_Zone) offsetSecs(ns int64) int64 {
	_, off := time.Unix(0, ns).In(z.data().loc).Zone()
	return int64(off)
}

// M_display of a ZoneErr (std.md 13.1): `unknown time zone "Mars/Olympus"`.
func (v Time_ZoneErr) M_display(*Task) string {
	switch p := v.P.(type) {
	case Time_ZoneErr_Unknown:
		return "unknown time zone " + strconv.Quote(p.F_name)
	case Time_ZoneErr_NoLocal:
		return "cannot determine the local time zone: " + p.F_msg
	}
	return Debug(v)
}
