package stdtest

// TestAmqptestFuzz (task #115): std/amqptest's broker driven by Go's
// go-amqp v1.7.0 client (vendored in scripts/amqptestdiff/go), against a
// Go model of the Service Bus queue semantics the broker documents. Each
// seeded script has its own queue (maximum delivery count 3, locks of 60
// s) and runs sends, peek-lock and receive-and-delete receivers with
// credit given one at a time, settles (accept, release, modify with and
// without delivery-failed and undeliverable-here, reject with a
// condition, and azservicebus's com.microsoft:dead-letter with the reason
// in the error's info), settles of deliveries whose lock ran out or that
// were settled, receivers closed with locks held, the broker's clock
// jumping past the locks, and the broker's view of the queue (ready,
// locked, deferred, and the dead letter queue with each message's
// DeadLetterReason and DeadLetterErrorDescription). The model:
// a receive takes the first message neither locked nor deferred and
// counts a delivery (the header's delivery-count is the deliveries
// before); accept removes, release gives it back uncounted, modify gives
// it back counted (or defers it when undeliverable-here), reject dead
// letters with the condition and description as the reason (or the
// info's for com.microsoft:dead-letter); a lock that runs out gives the
// message back counted; a message given back counted at the maximum is
// dead lettered as MaxDeliveryCountExceeded; settling a lock that is not
// held is com.microsoft:message-lock-lost; closing a receiver gives back
// what it held, uncounted.
//
//   XO_AMQPTFUZZ_SEED (default 2), XO_AMQPTFUZZ_N (400 scripts),
//   XO_AMQPTFUZZ_BACKENDS (arm64,llvm)

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/xo-lang/xo/internal/testtmp"
	"github.com/xo-lang/xo/internal/xotool"
)

type aqfMsg struct {
	body     string
	dc       int // deliveries so far
	token    int // the delivery holding its lock, 0 for none
	deferred bool
}

type aqfDead struct{ body, reason, desc string }

type aqfDelivery struct {
	body    string
	token   int // 0 for a receive and delete
	settled bool
}

// aqfModel: one script's queue as Service Bus keeps it.
type aqfModel struct {
	msgs      []*aqfMsg
	dead      []aqfDead
	links     map[string]string // label: lock or delete
	got       map[string][]*aqfDelivery
	nextToken int
	out       strings.Builder
}

const aqfMax = 3

func (m *aqfModel) find(body string) int {
	for i, x := range m.msgs {
		if x.body == body {
			return i
		}
	}
	return -1
}

func (m *aqfModel) deadLetter(i int, reason, desc string) {
	m.dead = append(m.dead, aqfDead{m.msgs[i].body, reason, desc})
	m.msgs = append(m.msgs[:i], m.msgs[i+1:]...)
}

// giveBack: the message's lock ends; counted, at the maximum it is dead lettered.
func (m *aqfModel) giveBack(i int, counted bool) {
	x := m.msgs[i]
	x.token = 0
	if !counted && x.dc > 0 {
		x.dc--
	}
	if counted && x.dc >= aqfMax {
		m.deadLetter(i, "MaxDeliveryCountExceeded", fmt.Sprintf("Message could not be consumed after %d delivery attempts.", aqfMax))
	}
}

func (m *aqfModel) available() int {
	for i, x := range m.msgs {
		if x.token == 0 && !x.deferred {
			return i
		}
	}
	return -1
}

func (m *aqfModel) step(line string) {
	w := strings.Split(line, " ")
	fmt.Fprintf(&m.out, "== %s\n", line)
	switch w[0] {
	case "send":
		m.msgs = append(m.msgs, &aqfMsg{body: w[1]})
		m.out.WriteString("sent ok\n")
	case "open":
		m.links[w[1]] = w[2]
		m.out.WriteString("open ok\n")
	case "recv":
		i := m.available()
		if i < 0 {
			m.out.WriteString("recv none\n")
			return
		}
		x := m.msgs[i]
		x.dc++
		fmt.Fprintf(&m.out, "recv %s dc %d\n", x.body, x.dc-1)
		d := &aqfDelivery{body: x.body}
		if m.links[w[1]] == "delete" {
			m.msgs = append(m.msgs[:i], m.msgs[i+1:]...)
		} else {
			m.nextToken++
			x.token = m.nextToken
			d.token = x.token
		}
		m.got[w[1]] = append(m.got[w[1]], d)
	case "settle":
		var k int
		fmt.Sscan(w[2], &k)
		d := m.got[w[1]][k]
		d.settled = true
		i := m.find(d.body)
		if i < 0 || m.msgs[i].token != d.token {
			m.out.WriteString("settled error com.microsoft:message-lock-lost\n")
			return
		}
		switch w[3] {
		case "accept":
			m.msgs = append(m.msgs[:i], m.msgs[i+1:]...)
		case "release":
			m.giveBack(i, false)
		case "modify":
			if w[5] == "1" {
				m.msgs[i].token = 0
				m.msgs[i].deferred = true
			} else {
				m.giveBack(i, true)
			}
		case "deadletter":
			m.deadLetter(i, w[4], "dd")
		default:
			m.deadLetter(i, w[4], "d")
		}
		m.out.WriteString("settled ok\n")
	case "close":
		for _, d := range m.got[w[1]] {
			if i := m.find(d.body); d.token != 0 && !d.settled && i >= 0 && m.msgs[i].token == d.token {
				m.giveBack(i, false)
			}
		}
		delete(m.links, w[1])
		m.out.WriteString("closed ok\n")
	case "advance":
		// every lock runs out, in queue order
		var held []string
		for _, x := range m.msgs {
			if x.token != 0 {
				held = append(held, x.body)
			}
		}
		for _, b := range held {
			m.giveBack(m.find(b), true)
		}
	case "state":
		var ready, deferred, dead []string
		locked := 0
		for _, x := range m.msgs {
			switch {
			case x.deferred:
				deferred = append(deferred, x.body)
			case x.token != 0:
				locked++
			default:
				ready = append(ready, x.body)
			}
		}
		for _, d := range m.dead {
			dead = append(dead, d.body+":"+d.reason+":"+d.desc)
		}
		fmt.Fprintf(&m.out, "ready %s\nlocked %d\ndeferred %s\ndead %s\n", strings.Join(ready, ","), locked, strings.Join(deferred, ","), strings.Join(dead, ","))
	}
}

// aqfGen: one script on queue q, its steps chosen by the model so that a
// receive is asked for only when a message is there to take (a credit
// left open would take a later message at a time the script does not
// control).
func aqfGen(r *rand.Rand, q string) []string {
	m := &aqfModel{links: map[string]string{}, got: map[string][]*aqfDelivery{}}
	lines := []string{"use " + q}
	emit := func(l string) {
		lines = append(lines, l)
		m.step(l)
	}
	sent, nlinks := 0, 0
	for i, n := 0, 10+r.Intn(50); i < n; i++ {
		var open []string
		for l := range m.links {
			open = append(open, l)
		}
		sortStrings(open)
		switch k := r.Intn(20); {
		case k < 4:
			emit(fmt.Sprintf("send m%d", sent))
			sent++
		case k < 5 || len(open) == 0:
			if len(open) < 3 {
				mode := "lock"
				if r.Intn(4) == 0 {
					mode = "delete"
				}
				emit(fmt.Sprintf("open L%d %s", nlinks, mode))
				nlinks++
			}
		case k < 10:
			if m.available() >= 0 {
				emit("recv " + open[r.Intn(len(open))])
			}
		case k < 16:
			// a delivery of a lock link, settled or not, its lock held or not
			l := open[r.Intn(len(open))]
			if m.links[l] != "lock" || len(m.got[l]) == 0 {
				continue
			}
			var fresh []int
			for j, d := range m.got[l] {
				if !d.settled {
					fresh = append(fresh, j)
				}
			}
			if len(fresh) == 0 {
				continue
			}
			j := fresh[r.Intn(len(fresh))]
			var op string
			switch r.Intn(7) {
			case 0, 1:
				op = "accept"
			case 2:
				op = "release"
			case 3:
				op = fmt.Sprintf("modify %d 0", r.Intn(2))
			case 4:
				op = fmt.Sprintf("modify %d 1", r.Intn(2))
			case 5:
				op = "reject " + []string{"my:cond", "amqp:internal-error"}[r.Intn(2)]
			default:
				op = "deadletter " + []string{"why", "Expired"}[r.Intn(2)]
			}
			emit(fmt.Sprintf("settle %s %d %s", l, j, op))
		case k < 17:
			emit("close " + open[r.Intn(len(open))])
		case k < 18:
			emit("advance 61000")
		default:
			emit("state " + q)
		}
	}
	emit("state " + q)
	return lines
}

func sortStrings(xs []string) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

// aqfWant: the model's output of a script.
func aqfWant(script []string) string {
	m := &aqfModel{links: map[string]string{}, got: map[string][]*aqfDelivery{}}
	for _, l := range script[1:] {
		m.step(l)
	}
	return "== " + script[0] + "\n" + m.out.String()
}

// aqfScripts: the driver's output split by script (each starts with "== use ").
func aqfScripts(out string) []string {
	var cs []string
	for _, l := range strings.SplitAfter(out, "\n") {
		if strings.HasPrefix(l, "== use ") {
			cs = append(cs, "")
		}
		if len(cs) > 0 {
			cs[len(cs)-1] += l
		}
	}
	return cs
}

func TestAmqptestFuzz(t *testing.T) {
	if testing.Short() {
		t.Skip("the amqptest fuzz builds the broker and the go-amqp driver")
	}
	seed := int64(envInt(t, "XO_AMQPTFUZZ_SEED", 2))
	n := envInt(t, "XO_AMQPTFUZZ_N", 400)
	backends := envBackends("XO_AMQPTFUZZ_BACKENDS", []string{"arm64", "llvm"})
	r := rand.New(rand.NewSource(seed))
	scripts := make([][]string, n)
	var all []string
	steps := 0
	for i := range scripts {
		scripts[i] = aqfGen(r, fmt.Sprintf("q%d", i))
		all = append(all, scripts[i]...)
		steps += len(scripts[i])
	}
	root, err := xotool.Root()
	if err != nil {
		t.Fatal(err)
	}
	xo, err := xotool.Path()
	if err != nil {
		t.Fatal(err)
	}
	work, err := testtmp.Dir("xo-amqptfuzz-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(work)
	driver := filepath.Join(work, "driver")
	gb := exec.Command("go", "build", "-o", driver, ".")
	gb.Dir = filepath.Join(root, "scripts", "amqptestdiff", "go")
	gb.Env = append(os.Environ(), "GOFLAGS=-mod=vendor", "GOPROXY=off", "GOWORK=off")
	if out, err := gb.CombinedOutput(); err != nil {
		t.Fatalf("building the go-amqp driver: %v\n%s", err, out)
	}
	for _, b := range backends {
		bin := filepath.Join(work, "broker-"+b)
		xb := testtmp.Guard(exec.Command(xo, "build", "--backend="+b, "-o", bin, filepath.Join(root, "scripts", "amqptestdiff", "xo")))
		xb.Env = append(os.Environ(), "XO_STD="+filepath.Join(root, "std"))
		if out, err := xb.CombinedOutput(); err != nil {
			t.Fatalf("xo build --backend=%s: %v\n%s", b, err, out)
		}
		got := aqfRun(t, bin, driver, all)
		cs := aqfScripts(got)
		if len(cs) != n {
			t.Fatalf("%s: %d scripts ran of %d\n%s", b, len(cs), n, myfTail(got, 30))
		}
		bad := 0
		for i := range scripts {
			want := aqfWant(scripts[i])
			if cs[i] != want {
				if bad < 3 {
					t.Errorf("%s script %d (seed %d):\n%s", b, i, seed, aqfDiff(cs[i], want))
				}
				bad++
			}
		}
		if bad > 0 {
			t.Fatalf("%s: %d of %d scripts differ (seed %d)", b, bad, n, seed)
		}
		t.Logf("%s: %d scripts (%d steps) as the model (seed %d)", b, n, steps, seed)
	}
}

// aqfRun: the broker serves while the driver runs the scripts; the driver's output.
func aqfRun(t *testing.T, broker, driver string, lines []string) string {
	bk := testtmp.Guard(exec.Command(broker))
	stdin, err := bk.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := bk.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var berr bytes.Buffer
	bk.Stderr = &berr
	if err := bk.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		stdin.Close()
		done := make(chan error, 1)
		go func() { done <- bk.Wait() }()
		select {
		case <-done:
		case <-time.After(10 * time.Second):
			bk.Process.Kill()
			<-done
		}
	}()
	br := bufio.NewReader(stdout)
	var amqpAddr, ctlAddr string
	for amqpAddr == "" || ctlAddr == "" {
		l, err := br.ReadString('\n')
		if err != nil {
			t.Fatalf("broker: %v\n%s", err, berr.String())
		}
		f := strings.Fields(l)
		if len(f) == 2 && f[0] == "amqp" {
			amqpAddr = f[1]
		} else if len(f) == 2 && f[0] == "control" {
			ctlAddr = f[1]
		}
	}
	go io.Copy(io.Discard, br)
	dr := testtmp.Guard(exec.Command(driver, amqpAddr, ctlAddr))
	dr.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	var out, derr bytes.Buffer
	dr.Stdout = &out
	dr.Stderr = &derr
	if err := dr.Run(); err != nil {
		t.Fatalf("driver: %v\n%s\nstderr:\n%s\nbroker stderr:\n%s", err, myfTail(out.String(), 20), myfTail(derr.String(), 20), myfTail(berr.String(), 20))
	}
	return out.String()
}

// aqfDiff: the first line that differs, with the lines before it.
func aqfDiff(got, want string) string {
	g := strings.Split(got, "\n")
	w := strings.Split(want, "\n")
	for i := 0; i < len(g) || i < len(w); i++ {
		var a, b string
		if i < len(g) {
			a = g[i]
		}
		if i < len(w) {
			b = w[i]
		}
		if a != b {
			lo := i - 12
			if lo < 0 {
				lo = 0
			}
			return fmt.Sprintf("line %d:\n%s\ngot  %s\nwant %s", i, strings.Join(w[lo:i], "\n"), a, b)
		}
	}
	return ""
}
