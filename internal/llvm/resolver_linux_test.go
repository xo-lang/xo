package llvm_test

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"testing"
	"unsafe"

	"github.com/xo-lang/xo/internal/build"
)

// The resolver of decision 0206 stage 3 (std/native/llrt/dns.xo) against
// glibc's getaddrinfo: testdata/resolver built twice (the Xo path, and
// XO_LLRT_C=net, the C library's), each run in a user, network, and
// mount namespace (unshare) whose /etc/resolv.conf, /etc/hosts, and
// /etc/nsswitch.conf are this test's, with a DNS server of this test
// (TestResolverDNSServer, started in the namespace and stopped by its
// PID) on 127.0.0.1:53. The outputs must be the same, and so must the
// questions the server saw.

// resolverConfig is one configuration of the namespace and the names
// the program dials in it.
type resolverConfig struct {
	name     string
	resolv   string
	hosts    string
	nsswitch string
	names    []string
}

var resolverConfigs = []resolverConfig{
	{
		name: "files_dns",
		resolv: "# a comment\n; another\nnameserver 127.0.0.1\nsearch corp.test other.test\n" +
			"options ndots:2 timeout:1 attempts:1\n",
		hosts:    "127.0.0.1 localhost h4.test\n::1 localhost h6.test # a comment\n127.0.0.1\tHostCase.Test\n",
		nsswitch: "passwd: files\nhosts:      files dns\n",
		names: []string{"localhost", "h4.test", "h6.test", "hostcase.test", "a.test", "six.test", "both.test",
			"cname.test", "big.test", "host", "nodata.test", "nx.test", "srvfail.test", "a.test.", "127.0.0.1", "::1"},
	},
	{
		name:     "dns_files",
		resolv:   "nameserver 127.0.0.1\ndomain corp.test\noptions ndots:1 timeout:1 attempts:2 single-request\n",
		hosts:    "127.0.0.1 localhost h4.test\n::1 localhost\n",
		nsswitch: "hosts: dns files\n",
		names:    []string{"h4.test", "a.test", "host", "both.test", "nx.test", "nodata.test", "big.test"},
	},
}

// TestResolverDiff runs the resolver program of each configuration with
// the Xo resolver and with glibc's (Linux, user namespaces).
func TestResolverDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("builds programs")
	}
	unshare, err := exec.LookPath("unshare")
	if err != nil {
		t.Skip("no unshare")
	}
	if out, err := exec.Command(unshare, "-r", "-n", "-m", "true").CombinedOutput(); err != nil {
		t.Skipf("no user namespaces: %v %s", err, out)
	}
	if _, err := os.Stat("/etc/nsswitch.conf"); err != nil {
		t.Skip("no /etc/nsswitch.conf to mount over")
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join("testdata", "resolver")
	xoBin := resolverBuild(t, dir, "")
	cBin := resolverBuild(t, dir, "net")
	for _, cf := range resolverConfigs {
		t.Run(cf.name, func(t *testing.T) {
			d := t.TempDir()
			for name, text := range map[string]string{"resolv.conf": cf.resolv, "hosts": cf.hosts, "nsswitch.conf": cf.nsswitch, "run.sh": resolverRunSh} {
				if err := os.WriteFile(filepath.Join(d, name), []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			args := append([]string{"-r", "-n", "-m", "sh", filepath.Join(d, "run.sh"), d, self, xoBin, cBin}, cf.names...)
			if out, err := exec.Command(unshare, args...).CombinedOutput(); err != nil {
				t.Fatalf("namespace run: %v\n%s", err, out)
			}
			read := func(name string) string {
				b, err := os.ReadFile(filepath.Join(d, name))
				if err != nil {
					t.Fatal(err)
				}
				return string(b)
			}
			xo, c := read("out-xo.txt"), read("out-c.txt")
			t.Logf("Xo resolver:\n%s", xo)
			if xo != c {
				t.Errorf("the Xo resolver's output differs from glibc's:\n--- Xo\n%s--- glibc\n%s", xo, c)
			}
			qx, qc := resolverQuestions(read("queries-xo.log")), resolverQuestions(read("queries-c.log"))
			if qx != qc {
				t.Errorf("the server saw other questions:\n--- Xo\n%s--- glibc\n%s", qx, qc)
			}
			if cf.name == "files_dns" {
				for _, want := range []string{"big.test: 4\n", "host: 4\n", "cname.test: 4\n", "both.test: 6\n", "h6.test: 6\n"} {
					if !strings.Contains(xo, want) {
						t.Errorf("Xo output lacks %q", want)
					}
				}
				if !strings.Contains(qx, "tcp big.test. A\n") {
					t.Errorf("no TCP question after the truncated answer:\n%s", qx)
				}
			}
		})
	}
}

// resolverRunSh runs in the namespace: DIR TESTBIN XOBIN CBIN NAMES...
const resolverRunSh = `set -e
D=$1; T=$2; X=$3; C=$4; shift 4
mount --bind "$D/resolv.conf" /etc/resolv.conf
mount --bind "$D/hosts" /etc/hosts
mount --bind "$D/nsswitch.conf" /etc/nsswitch.conf
XO_RESOLVER_SERVER_DIR="$D" "$T" -test.run='^TestResolverDNSServer$' > "$D/server.out" 2>&1 &
PID=$!
i=0
while [ ! -e "$D/ready" ] && [ $i -lt 200 ]; do sleep 0.05; i=$((i+1)); done
set +e
"$X" "$@" > "$D/out-xo.txt" 2>&1
mv "$D/queries.log" "$D/queries-xo.log" 2>/dev/null || : > "$D/queries-xo.log"
"$C" "$@" > "$D/out-c.txt" 2>&1
mv "$D/queries.log" "$D/queries-c.log" 2>/dev/null || : > "$D/queries-c.log"
kill $PID
wait $PID
exit 0
`

// resolverBuild builds the program with XO_LLRT_C set to llrtC.
func resolverBuild(t *testing.T, dir, llrtC string) string {
	t.Helper()
	t.Setenv("XO_LLRT_C", llrtC)
	res, err := build.Build([]string{dir}, build.Options{Backend: build.BackendLLVM, Out: filepath.Join(t.TempDir(), "resolver")})
	if err != nil {
		t.Fatal(err)
	}
	if res.Failed {
		t.Fatalf("build failed: %v", res.Diagnostics)
	}
	return res.Binaries[build.Host()]
}

// resolverQuestions is the distinct lines of a server log, sorted.
func resolverQuestions(log string) string {
	seen := map[string]bool{}
	var out []string
	for _, l := range strings.Split(log, "\n") {
		if l != "" && !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	sort.Strings(out)
	return strings.Join(out, "\n") + "\n"
}

// resolverZone is the server's data: A and AAAA records, a CNAME, an
// answer only TCP carries, a name with no address, and a failing one;
// every other name does not exist.
var resolverZone = map[string]struct {
	a4, a6 []string
	cname  string
	rcode  int
	trunc  bool
}{
	"a.test.":         {a4: []string{"127.0.0.1"}},
	"six.test.":       {a6: []string{"::1"}},
	"both.test.":      {a4: []string{"127.0.0.1"}, a6: []string{"::1"}},
	"cname.test.":     {cname: "a.test."},
	"big.test.":       {a4: []string{"127.0.0.1"}, trunc: true},
	"host.corp.test.": {a4: []string{"127.0.0.1"}},
	"nodata.test.":    {},
	"srvfail.test.":   {rcode: 2},
}

// TestResolverDNSServer is the DNS server of TestResolverDiff, run in its
// namespace (XO_RESOLVER_SERVER_DIR names the directory: queries.log
// gets one line per question, ready is made once it listens); it serves
// until killed.
func TestResolverDNSServer(t *testing.T) {
	d := os.Getenv("XO_RESOLVER_SERVER_DIR")
	if d == "" {
		t.Skip("run by TestResolverDiff")
	}
	if err := loopbackUp(); err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	logq := func(proto, name string, qtype uint16) {
		mu.Lock()
		defer mu.Unlock()
		f, err := os.OpenFile(filepath.Join(d, "queries.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err == nil {
			fmt.Fprintf(f, "%s %s %s\n", proto, name, map[uint16]string{1: "A", 28: "AAAA"}[qtype])
			f.Close()
		}
	}
	uc, err := net.ListenPacket("udp", "127.0.0.1:53")
	if err != nil {
		t.Fatal(err)
	}
	tl, err := net.Listen("tcp", "127.0.0.1:53")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			c, err := tl.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				for {
					var n [2]byte
					if _, err := io.ReadFull(c, n[:]); err != nil {
						return
					}
					q := make([]byte, binary.BigEndian.Uint16(n[:]))
					if _, err := io.ReadFull(c, q); err != nil {
						return
					}
					r := dnsReply(q, false, logq)
					if r == nil {
						return
					}
					binary.BigEndian.PutUint16(n[:], uint16(len(r)))
					c.Write(append(n[:], r...))
				}
			}()
		}
	}()
	if err := os.WriteFile(filepath.Join(d, "ready"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 4096)
	for {
		n, from, err := uc.ReadFrom(buf)
		if err != nil {
			t.Fatal(err)
		}
		if r := dnsReply(buf[:n], true, logq); r != nil {
			uc.WriteTo(r, from)
		}
	}
}

// dnsReply answers query q from resolverZone (nil for one it cannot
// read), logging the question.
func dnsReply(q []byte, udp bool, logq func(string, string, uint16)) []byte {
	if len(q) < 12 || binary.BigEndian.Uint16(q[4:]) < 1 {
		return nil
	}
	var labels []string
	i := 12
	for {
		if i >= len(q) {
			return nil
		}
		l := int(q[i])
		if l == 0 {
			i++
			break
		}
		if l >= 64 || i+1+l > len(q) {
			return nil
		}
		labels = append(labels, strings.ToLower(string(q[i+1:i+1+l])))
		i += 1 + l
	}
	if i+4 > len(q) {
		return nil
	}
	qtype := binary.BigEndian.Uint16(q[i:])
	name := strings.Join(labels, ".") + "."
	question := q[12 : i+4]
	proto := "udp"
	if !udp {
		proto = "tcp"
	}
	logq(proto, name, qtype)
	rec, ok := resolverZone[name]
	rcode := 0
	if !ok {
		rcode = 3
	} else {
		rcode = rec.rcode
	}
	var answers [][]byte
	tc := false
	if ok && rcode == 0 {
		if rec.trunc && udp {
			tc = true
		} else {
			owner := name
			if rec.cname != "" {
				answers = append(answers, dnsRR(owner, 5, dnsName(rec.cname)))
				owner = rec.cname
				rec = resolverZone[owner]
			}
			ips := rec.a4
			if qtype == 28 {
				ips = rec.a6
			}
			if qtype == 1 || qtype == 28 {
				for _, s := range ips {
					ip := net.ParseIP(s)
					if qtype == 1 {
						ip = ip.To4()
					} else {
						ip = ip.To16()
					}
					answers = append(answers, dnsRR(owner, qtype, ip))
				}
			}
		}
	}
	var b bytes.Buffer
	b.Write(q[0:2])
	flags := uint16(0x8000 | 0x0400 | 0x0080 | uint16(rcode))
	flags |= binary.BigEndian.Uint16(q[2:]) & 0x0100 // RD
	if tc {
		flags |= 0x0200
	}
	binary.Write(&b, binary.BigEndian, flags)
	binary.Write(&b, binary.BigEndian, [4]uint16{1, uint16(len(answers)), 0, 0})
	b.Write(question)
	for _, a := range answers {
		b.Write(a)
	}
	return b.Bytes()
}

func dnsName(name string) []byte {
	var b []byte
	for _, l := range strings.Split(strings.TrimSuffix(name, "."), ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	return append(b, 0)
}

func dnsRR(owner string, ty uint16, data []byte) []byte {
	b := dnsName(owner)
	var h [10]byte
	binary.BigEndian.PutUint16(h[0:], ty)
	binary.BigEndian.PutUint16(h[2:], 1)
	binary.BigEndian.PutUint32(h[4:], 60)
	binary.BigEndian.PutUint16(h[8:], uint16(len(data)))
	return append(append(b, h[:]...), data...)
}

// loopbackUp brings up lo in a new network namespace.
func loopbackUp() error {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		return err
	}
	defer syscall.Close(fd)
	var ifr [40]byte
	copy(ifr[:], "lo")
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCGIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		return e
	}
	*(*uint16)(unsafe.Pointer(&ifr[16])) |= syscall.IFF_UP | syscall.IFF_RUNNING
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), syscall.SIOCSIFFLAGS, uintptr(unsafe.Pointer(&ifr[0]))); e != 0 {
		return e
	}
	return nil
}
