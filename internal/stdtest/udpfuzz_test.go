package stdtest

// A differential of std/udp against Go's net package (net.ListenPacket,
// net.Dial "udp", ReadFrom and WriteTo) on seeded steps (std Proven
// evidence for udp, decision 0616's fuzz kind, task #27).
//
// Steps (scripts/udpdiff/xo prints std/udp's answers, udfGo Go's):
//
//   - listen ADDR and dial ADDR: whether the address is accepted. Hosts
//     are IP literals, bracketed or not, "localhost", empty, or malformed
//     around a literal (never a name that needs DNS); ports are 0 for a
//     listen, any number for a dial, or malformed (signs, leading zeros,
//     past 65535, hex, empty; no spaces, which split a step). Only ok or error is compared: the error
//     texts differ by design.
//   - send VIA LEN MAX SEED: a datagram of 0 to 9,000 bytes (under
//     macOS's 9,216 byte net.inet.udp.maxdgram) from a dialed or a bound
//     socket to a bound one, read with a buffer of MAX bytes (a longer
//     datagram is cut, as Go's ReadFrom cuts it), and a reply the other
//     way: the length, the first and last 8 bytes, and the sender.
//
// XO_UDPFUZZ_SEED (default 2), XO_UDPFUZZ_N (default 300) and
// XO_UDPFUZZ_BACKENDS (default arm64,llvm). The reference is Go's own
// net package: nothing to build.

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

var udfHosts = []string{
	"127.0.0.1", "0.0.0.0", "[::1]", "[::]", "localhost", "",
	"::1", "[127.0.0.1]", "[::1", "::1]", "127.0.0.1.", "[::ffff:127.0.0.1]",
	"127.0.0.01", "127.000.0.1", "[0:0:0:0:0:0:0:1]", "[::1%]",
}

var udfBadPorts = []string{"", "-1", "+0", "65536", "70000", "99999999999999999999", "0x10", "1e3", "00", "000053", "-0"}

func udfAddr(r *rand.Rand, listen bool) string {
	h := udfHosts[r.Intn(len(udfHosts))]
	var p string
	switch {
	case r.Intn(3) == 0:
		p = udfBadPorts[r.Intn(len(udfBadPorts))]
	case listen:
		p = "0"
	default:
		p = strconv.Itoa(1 + r.Intn(65535))
	}
	switch r.Intn(12) {
	case 0:
		return h + p // no colon
	case 1:
		return h + "::" + p
	}
	return h + ":" + p
}

func udfGen(r *rand.Rand) string {
	switch k := r.Intn(10); {
	case k < 3:
		return "listen " + udfAddr(r, true)
	case k < 5:
		return "dial " + udfAddr(r, false)
	}
	via := "to"
	if r.Intn(2) == 0 {
		via = "dial"
	}
	var n int
	switch r.Intn(4) {
	case 0:
		n = r.Intn(16)
	case 1:
		n = 9000 - r.Intn(64)
	default:
		n = r.Intn(9001)
	}
	max := 1 + r.Intn(n+16)
	if r.Intn(3) == 0 {
		max = 65535
	}
	return fmt.Sprintf("send %s %d %d %d", via, n, max, r.Intn(256))
}

func udfPayload(n, seed int) []byte {
	b := make([]byte, n)
	for k := range b {
		b[k] = byte((seed + 7*k) % 256)
	}
	return b
}

func udfShow(d []byte) string {
	head, tail := d, d
	if len(d) > 8 {
		head, tail = d[:8], d[len(d)-8:]
	}
	return fmt.Sprintf("%d %s %s", len(d), hex.EncodeToString(head), hex.EncodeToString(tail))
}

func udfFrom(from, want net.Addr) string {
	if from.String() == want.String() {
		return "peer"
	}
	return "other " + from.String()
}

func udfSend(w []string) (string, error) {
	n, _ := strconv.Atoi(w[2])
	max, _ := strconv.Atoi(w[3])
	seed, _ := strconv.Atoi(w[4])
	a, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		return "", err
	}
	defer a.Close()
	dial := w[1] == "dial"
	var b net.PacketConn
	var bc net.Conn
	if dial {
		bc, err = net.Dial("udp", a.LocalAddr().String())
	} else {
		b, err = net.ListenPacket("udp", "127.0.0.1:0")
	}
	if err != nil {
		return "", err
	}
	var blocal net.Addr
	if dial {
		defer bc.Close()
		blocal = bc.LocalAddr()
		_, err = bc.Write(udfPayload(n, seed))
	} else {
		defer b.Close()
		blocal = b.LocalAddr()
		_, err = b.WriteTo(udfPayload(n, seed), a.LocalAddr())
	}
	if err != nil {
		return "", err
	}
	buf := make([]byte, max)
	k, from, err := a.ReadFrom(buf)
	if err != nil {
		return "", err
	}
	first := udfShow(buf[:k]) + " " + udfFrom(from, blocal)
	if _, err := a.WriteTo(udfPayload(n, seed+1), from); err != nil {
		return "", err
	}
	buf2 := make([]byte, max)
	var second string
	if dial {
		k, err = bc.Read(buf2)
		if err != nil {
			return "", err
		}
		second = udfShow(buf2[:k]) + " peer"
	} else {
		var f2 net.Addr
		k, f2, err = b.ReadFrom(buf2)
		if err != nil {
			return "", err
		}
		second = udfShow(buf2[:k]) + " " + udfFrom(f2, a.LocalAddr())
	}
	return first + "\n" + second, nil
}

// udfGo answers the steps with Go's net package, as scripts/udpdiff/xo
// prints std/udp's.
func udfGo(lines []string) string {
	var out strings.Builder
	for _, line := range lines {
		w := strings.Split(line, " ")
		fmt.Fprintf(&out, "== %s\n", line)
		var s string
		var err error
		switch w[0] {
		case "listen":
			var c net.PacketConn
			if c, err = net.ListenPacket("udp", w[1]); err == nil {
				c.Close()
				s = "ok"
			}
		case "dial":
			var c net.Conn
			if c, err = net.Dial("udp", w[1]); err == nil {
				c.Close()
				s = "ok"
			}
		default:
			s, err = udfSend(w)
		}
		if err != nil {
			s = "error"
		}
		out.WriteString(s + "\n")
	}
	return out.String()
}

func TestUdpFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the udp fuzz builds the Xo side")
	}
	seed := int64(envInt(t, "XO_UDPFUZZ_SEED", 2))
	n := envInt(t, "XO_UDPFUZZ_N", 300)
	backends := envBackends("XO_UDPFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	lines := make([]string, n)
	for i := range lines {
		lines[i] = udfGen(r)
	}
	want := myfBlocksBy(udfGo(lines), "== ", lines)
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-udpfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	for _, b := range backends {
		bin := filepath.Join(work, "xo-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "udpdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		cmd := testtmp.Guard(exec.Command(bin))
		cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
		var out, errb bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errb
		if err := cmd.Run(); err != nil {
			t.Fatalf("%s: %v\n%s\nstderr:\n%s", b, err, myfTail(out.String(), 20), myfTail(errb.String(), 30))
		}
		got := myfBlocksBy(out.String(), "== ", lines)
		bad := 0
		for i := 0; i < n; i++ {
			if got[i] == want[i] {
				continue
			}
			if bad < 10 {
				t.Errorf("%s case %d (seed %d): %s\ngot\n%swant\n%s", b, i, seed, lines[i], got[i], want[i])
			}
			bad++
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d cases differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d cases equal Go's net package (seed %d)", b, n, seed)
	}
}
