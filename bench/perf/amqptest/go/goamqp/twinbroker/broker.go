// Package twinbroker is the Go twin of Xo's std/amqptest broker (ADR 0655
// rule 7): the same queue model (sequence numbers, enqueue time, peek-lock
// with 16 byte lock tokens in .NET GUID order, delivery counts, complete,
// release) and the same connection handling (SASL ANONYMOUS, open, begin,
// attach, flow with link credit and session windows, transfer, disposition,
// detach, end, close), ported from std/amqptest.xo for the parts the
// bench/perf/amqptest workloads reach. It lives inside this copy of
// go-amqp v1.7.0 (MIT, see ../LICENSE) because it encodes frames with
// go-amqp's internal frame codec, as std/amqptest encodes them with
// std/amqp's. Not part of go-amqp.
package twinbroker

import (
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"time"

	amqp "github.com/Azure/go-amqp"
	"github.com/Azure/go-amqp/internal/buffer"
	"github.com/Azure/go-amqp/internal/encoding"
	"github.com/Azure/go-amqp/internal/frames"
)

const lockMs = 60000

type fakeMsg struct {
	seq           int
	m             *amqp.Message
	deliveryCount int
	lockToken     []byte
	lockedUntil   int64
	lockConn      int
	enqueuedMs    int64
}

type fakeQueue struct {
	name    string
	msgs    []fakeMsg
	nextSeq int
}

// Broker is an in-process broker with named queues.
type Broker struct {
	mu       sync.Mutex
	queues   []*fakeQueue
	nextLock int
	nextConn int
	wakes    []chan struct{}
}

// New returns a broker with no queues.
func New() *Broker { return &Broker{nextLock: 1} }

// Queue adds a queue.
func (b *Broker) Queue(name string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.queues = append(b.queues, &fakeQueue{name: name, nextSeq: 1})
}

func nowMs() int64 { return time.Now().UnixMilli() }

func (b *Broker) findQueue(name string) *fakeQueue {
	for _, q := range b.queues {
		if q.name == name {
			return q
		}
	}
	return nil
}

func (b *Broker) notifyAll() {
	for _, w := range b.wakes {
		select {
		case w <- struct{}{}:
		default:
		}
	}
}

func tokenBytes(n int) []byte {
	t := make([]byte, 16)
	copy(t, []byte{10, 11, 12, 13, 26, 27, 42, 43})
	binary.BigEndian.PutUint64(t[8:], uint64(n))
	return t
}

func guidOrder(t []byte) []byte {
	if len(t) != 16 {
		return t
	}
	o := make([]byte, 16)
	for i, k := range []int{3, 2, 1, 0, 5, 4, 7, 6, 8, 9, 10, 11, 12, 13, 14, 15} {
		o[i] = t[k]
	}
	return o
}

func annSet(m *amqp.Message, key string, v any) {
	if m.Annotations == nil {
		m.Annotations = amqp.Annotations{}
	}
	m.Annotations[encoding.Symbol(key)] = v
}

// enqueue stores a message: sequence number and enqueue time. b.mu held.
func (b *Broker) enqueue(q *fakeQueue, m *amqp.Message, now int64) int {
	seq := q.nextSeq
	q.nextSeq++
	annSet(m, "x-opt-sequence-number", int64(seq))
	annSet(m, "x-opt-enqueued-time", time.UnixMilli(now).UTC())
	q.msgs = append(q.msgs, fakeMsg{seq: seq, m: m, enqueuedMs: now})
	b.notifyAll()
	return seq
}

func expireLocks(q *fakeQueue, now int64) {
	for i := range q.msgs {
		m := &q.msgs[i]
		if m.lockedUntil > 0 && m.lockedUntil <= now {
			m.lockedUntil = 0
			m.lockToken = nil
		}
	}
}

func available(m *fakeMsg, now int64) bool {
	return !(m.lockedUntil > 0 && m.lockedUntil > now)
}

// take locks the next available message for conn, or removes it when
// settle (receive and delete). b.mu held.
func (b *Broker) take(q *fakeQueue, conn int, now int64, settle bool) (fakeMsg, bool) {
	expireLocks(q, now)
	for i := range q.msgs {
		m := &q.msgs[i]
		if available(m, now) {
			m.deliveryCount++
			if settle {
				got := *m
				complete(q, got.seq)
				return got, true
			}
			m.lockToken = tokenBytes(b.nextLock)
			b.nextLock++
			m.lockedUntil = now + lockMs
			m.lockConn = conn
			return *m, true
		}
	}
	return fakeMsg{}, false
}

func findSeq(q *fakeQueue, seq int) int {
	for i := range q.msgs {
		if q.msgs[i].seq == seq {
			return i
		}
	}
	return -1
}

// complete removes the message in place, as std/amqptest's remove_at does.
func complete(q *fakeQueue, seq int) {
	if i := findSeq(q, seq); i >= 0 {
		q.msgs = append(q.msgs[:i], q.msgs[i+1:]...)
	}
}

func unlock(q *fakeQueue, seq int) {
	if i := findSeq(q, seq); i >= 0 {
		q.msgs[i].lockedUntil = 0
		q.msgs[i].lockToken = nil
	}
}

// withDeliveryAnnotations is the message as delivered: a copy with the
// lock annotations and the header's delivery count.
func withDeliveryAnnotations(m fakeMsg) *amqp.Message {
	msg := *m.m
	ann := amqp.Annotations{}
	for k, v := range m.m.Annotations {
		ann[k] = v
	}
	msg.Annotations = ann
	if m.lockedUntil > 0 {
		annSet(&msg, "x-opt-locked-until", time.UnixMilli(m.lockedUntil).UTC())
		annSet(&msg, "x-opt-lock-token", amqp.UUID(m.lockToken))
	}
	h := amqp.MessageHeader{}
	if m.m.Header != nil {
		h = *m.m.Header
	}
	if m.deliveryCount > 0 {
		h.DeliveryCount = uint32(m.deliveryCount - 1)
	} else {
		h.DeliveryCount = 0
	}
	msg.Header = &h
	if m.lockedUntil > 0 {
		msg.DeliveryAnnotations = amqp.Annotations{encoding.Symbol("x-opt-lock-token"): amqp.UUID(m.lockToken)}
	}
	return &msg
}

type unsettled struct{ id, seq int }

type svLink struct {
	handle, remote, channel int
	name                    string
	sends                   bool
	queue                   string
	credit                  int
	deliveryCount           int
	presettled              bool
	detached                bool
	unsettled               []unsettled
	partialActive           bool
	partialID               int
	partialSettled          bool
	partial                 []byte
}

type svSession struct {
	channel      int
	nextIn       int
	nextOut      int
	remoteWindow int
	inUsed       int
	ended        bool
}

type svConn struct {
	id       int
	nc       net.Conn
	maxFrame int
	sess     []*svSession
	links    []*svLink
	wbuf     buffer.Buffer
}

func (c *svConn) out(ch int, body frames.FrameBody) error {
	c.wbuf.Reset()
	if err := frames.Write(&c.wbuf, frames.Frame{Type: frames.TypeAMQP, Channel: uint16(ch), Body: body}); err != nil {
		return err
	}
	_, err := c.nc.Write(c.wbuf.Bytes())
	return err
}

func (c *svConn) outSASL(body frames.FrameBody) error {
	c.wbuf.Reset()
	if err := frames.Write(&c.wbuf, frames.Frame{Type: frames.TypeSASL, Body: body}); err != nil {
		return err
	}
	_, err := c.nc.Write(c.wbuf.Bytes())
	return err
}

func (c *svConn) session(ch int) *svSession {
	for _, z := range c.sess {
		if z.channel == ch {
			return z
		}
	}
	return nil
}

func (c *svConn) linkRemote(ch, remote int) *svLink {
	for _, l := range c.links {
		if l.channel == ch && l.remote == remote && !l.detached {
			return l
		}
	}
	return nil
}

func u32(x int) *uint32 { v := uint32(x & 0xffffffff); return &v }

// Serve accepts connections on l until it is closed.
func (b *Broker) Serve(l net.Listener) {
	for {
		nc, err := l.Accept()
		if err != nil {
			return
		}
		b.mu.Lock()
		b.nextConn++
		id := b.nextConn
		wake := make(chan struct{}, 1)
		b.wakes = append(b.wakes, wake)
		b.mu.Unlock()
		go func() {
			b.runConn(nc, id, wake)
			b.mu.Lock()
			for _, q := range b.queues {
				for i := range q.msgs {
					if q.msgs[i].lockConn == id && q.msgs[i].lockedUntil > 0 {
						q.msgs[i].lockedUntil = 0
						q.msgs[i].lockToken = nil
					}
				}
			}
			for i, w := range b.wakes {
				if w == wake {
					b.wakes = append(b.wakes[:i:i], b.wakes[i+1:]...)
					break
				}
			}
			b.mu.Unlock()
			nc.Close()
		}()
	}
}

type reader struct {
	nc  net.Conn
	buf []byte
	tmp []byte
}

func (r *reader) readN(n int) error {
	for len(r.buf) < n {
		k, err := r.nc.Read(r.tmp)
		if k > 0 {
			r.buf = append(r.buf, r.tmp[:k]...)
		}
		if err != nil {
			if err == io.EOF && len(r.buf) >= n {
				break
			}
			return err
		}
	}
	return nil
}

// frame reads one frame; sasl selects the SASL body types the client sends.
func (r *reader) frame() (uint16, frames.FrameBody, error) {
	if err := r.readN(8); err != nil {
		return 0, nil, err
	}
	size := int(binary.BigEndian.Uint32(r.buf))
	if size < 8 {
		return 0, nil, errors.New("bad frame header")
	}
	if err := r.readN(size); err != nil {
		return 0, nil, err
	}
	raw := r.buf[:size]
	r.buf = r.buf[size:]
	hb := buffer.New(raw)
	h, err := frames.ParseHeader(hb)
	if err != nil {
		return 0, nil, err
	}
	body := raw[int(h.DataOffset)*4:]
	if len(body) == 0 {
		return h.Channel, nil, nil // empty frame (keepalive)
	}
	bb := buffer.New(body)
	if len(body) > 2 && encoding.AMQPType(body[2]) == encoding.TypeCodeSASLInit {
		si := new(frames.SASLInit)
		return h.Channel, si, si.Unmarshal(bb)
	}
	fb, err := frames.ParseBody(bb)
	return h.Channel, fb, err
}

func (b *Broker) handshake(c *svConn, r *reader) error {
	if err := r.readN(8); err != nil {
		return err
	}
	if r.buf[4] == 3 {
		if _, err := c.nc.Write(r.buf[:8]); err != nil {
			return err
		}
		r.buf = r.buf[8:]
		if err := c.outSASL(&frames.SASLMechanisms{Mechanisms: encoding.MultiSymbol{"ANONYMOUS", "PLAIN", "MSSBCBS"}}); err != nil {
			return err
		}
		_, body, err := r.frame()
		if err != nil {
			return err
		}
		if _, ok := body.(*frames.SASLInit); !ok {
			return errors.New("expected sasl-init")
		}
		if err := c.outSASL(&frames.SASLOutcome{Code: encoding.CodeSASLOK}); err != nil {
			return err
		}
		if err := r.readN(8); err != nil {
			return err
		}
	}
	if _, err := c.nc.Write(r.buf[:8]); err != nil {
		return err
	}
	r.buf = r.buf[8:]
	_, body, err := r.frame()
	if err != nil {
		return err
	}
	o, ok := body.(*frames.PerformOpen)
	if !ok {
		return errors.New("expected open")
	}
	if o.MaxFrameSize > 0 && int(o.MaxFrameSize) < c.maxFrame {
		c.maxFrame = int(o.MaxFrameSize)
	}
	return c.out(0, &frames.PerformOpen{ContainerID: "go-amqptest", MaxFrameSize: 65536, ChannelMax: 255})
}

type inFrame struct {
	ch   uint16
	body frames.FrameBody
}

func (b *Broker) runConn(nc net.Conn, id int, wake chan struct{}) {
	c := &svConn{id: id, nc: nc, maxFrame: 65536}
	r := &reader{nc: nc, tmp: make([]byte, 65536)}
	if err := b.handshake(c, r); err != nil {
		return
	}
	in := make(chan inFrame, 64)
	go func() {
		defer close(in)
		for {
			ch, body, err := r.frame()
			if err != nil {
				return
			}
			if body != nil {
				in <- inFrame{ch, body}
			}
		}
	}()
	for {
		select {
		case f, ok := <-in:
			if !ok {
				return
			}
			stop, err := b.handleFrame(c, int(f.ch), f.body)
			if err != nil || stop {
				return
			}
		case <-wake:
			if err := b.deliverAll(c); err != nil {
				return
			}
		}
	}
}

func (b *Broker) handleFrame(c *svConn, ch int, body frames.FrameBody) (bool, error) {
	switch x := body.(type) {
	case *frames.PerformBegin:
		c.sess = append(c.sess, &svSession{channel: ch, nextIn: int(x.NextOutgoingID), remoteWindow: int(x.IncomingWindow)})
		rc := uint16(ch)
		return false, c.out(ch, &frames.PerformBegin{RemoteChannel: &rc, NextOutgoingID: 0, IncomingWindow: 2048, OutgoingWindow: 2048, HandleMax: 4294967295})
	case *frames.PerformAttach:
		return false, b.onAttach(c, ch, x)
	case *frames.PerformFlow:
		return false, b.onFlow(c, ch, x)
	case *frames.PerformTransfer:
		return false, b.onTransfer(c, ch, x)
	case *frames.PerformDisposition:
		return false, b.onDisposition(c, ch, x)
	case *frames.PerformDetach:
		if l := c.linkRemote(ch, int(x.Handle)); l != nil {
			b.dropLink(c, l)
			return false, c.out(ch, &frames.PerformDetach{Handle: uint32(l.handle), Closed: true})
		}
		return false, nil
	case *frames.PerformEnd:
		for _, l := range c.links {
			if l.channel == ch {
				b.dropLink(c, l)
			}
		}
		if z := c.session(ch); z != nil {
			z.ended = true
		}
		return false, c.out(ch, &frames.PerformEnd{})
	case *frames.PerformClose:
		return true, c.out(0, &frames.PerformClose{})
	}
	return false, nil
}

func (b *Broker) dropLink(c *svConn, l *svLink) {
	if l.detached {
		return
	}
	b.mu.Lock()
	if q := b.findQueue(l.queue); q != nil {
		for _, u := range l.unsettled {
			unlock(q, u.seq)
		}
	}
	b.notifyAll()
	b.mu.Unlock()
	l.unsettled = nil
	l.detached = true
}

func (b *Broker) onAttach(c *svConn, ch int, a *frames.PerformAttach) error {
	z := c.session(ch)
	if z == nil {
		return nil
	}
	clientSends := a.Role == encoding.RoleSender
	addr := ""
	if clientSends {
		if a.Target != nil {
			addr = a.Target.Address
		}
	} else if a.Source != nil {
		addr = a.Source.Address
	}
	b.mu.Lock()
	found := b.findQueue(addr) != nil
	b.mu.Unlock()
	h := len(c.links)
	l := &svLink{handle: h, remote: int(a.Handle), channel: ch, name: a.Name, sends: !clientSends, queue: addr}
	l.presettled = a.SenderSettleMode != nil && *a.SenderSettleMode == encoding.SenderSettleModeSettled
	role := encoding.RoleSender
	if clientSends {
		role = encoding.RoleReceiver
	}
	if !found {
		l.detached = true
		c.links = append(c.links, l)
		if err := c.out(ch, &frames.PerformAttach{Name: a.Name, Handle: uint32(h), Role: role}); err != nil {
			return err
		}
		return c.out(ch, &frames.PerformDetach{Handle: uint32(h), Closed: true, Error: &encoding.Error{Condition: "amqp:not-found", Description: "The messaging entity '" + addr + "' could not be found."}})
	}
	if clientSends {
		l.credit = 100000
	}
	c.links = append(c.links, l)
	if clientSends {
		if err := c.out(ch, &frames.PerformAttach{Name: a.Name, Handle: uint32(h), Role: encoding.RoleReceiver, SenderSettleMode: a.SenderSettleMode,
			ReceiverSettleMode: a.ReceiverSettleMode, Source: a.Source, Target: a.Target}); err != nil {
			return err
		}
		return c.out(ch, &frames.PerformFlow{NextIncomingID: u32(z.nextIn), IncomingWindow: 2048, NextOutgoingID: uint32(z.nextOut), OutgoingWindow: 2048,
			Handle: u32(h), DeliveryCount: u32(0), LinkCredit: u32(100000)})
	}
	return c.out(ch, &frames.PerformAttach{Name: a.Name, Handle: uint32(h), Role: encoding.RoleSender, SenderSettleMode: a.SenderSettleMode,
		ReceiverSettleMode: a.ReceiverSettleMode, Source: a.Source, Target: a.Target, InitialDeliveryCount: 0})
}

func (b *Broker) onFlow(c *svConn, ch int, x *frames.PerformFlow) error {
	z := c.session(ch)
	if z == nil {
		return nil
	}
	nin := 0
	if x.NextIncomingID != nil {
		nin = int(*x.NextIncomingID)
	}
	z.remoteWindow = (nin + int(x.IncomingWindow) - z.nextOut) & 0xffffffff
	if x.Handle != nil {
		if l := c.linkRemote(ch, int(*x.Handle)); l != nil && l.sends {
			dc := l.deliveryCount
			if x.DeliveryCount != nil {
				dc = int(*x.DeliveryCount)
			}
			lc := 0
			if x.LinkCredit != nil {
				lc = int(*x.LinkCredit)
			}
			l.credit = (dc + lc - l.deliveryCount) & 0xffffffff
		}
	}
	if err := b.deliverAll(c); err != nil {
		return err
	}
	if x.Drain && x.Handle != nil {
		if l := c.linkRemote(ch, int(*x.Handle)); l != nil && l.sends {
			l.deliveryCount = (l.deliveryCount + l.credit) & 0xffffffff
			l.credit = 0
			return c.out(ch, &frames.PerformFlow{NextIncomingID: u32(z.nextIn), IncomingWindow: 2048, NextOutgoingID: uint32(z.nextOut), OutgoingWindow: 2048,
				Handle: u32(l.handle), DeliveryCount: u32(l.deliveryCount), LinkCredit: u32(0), Drain: true})
		}
	}
	return nil
}

func (b *Broker) onTransfer(c *svConn, ch int, t *frames.PerformTransfer) error {
	l := c.linkRemote(ch, int(t.Handle))
	z := c.session(ch)
	if l == nil || z == nil {
		return nil
	}
	z.inUsed++
	if t.DeliveryID != nil {
		z.nextIn = (int(*t.DeliveryID) + 1) & 0xffffffff
		if !l.partialActive {
			l.partialActive = true
			l.partialID = int(*t.DeliveryID)
			l.partialSettled = t.Settled
			l.partial = l.partial[:0]
		}
	}
	done := false
	if t.Aborted {
		l.partialActive = false
		l.partial = l.partial[:0]
	} else if l.partialActive {
		l.partial = append(l.partial, t.Payload...)
		if t.Settled {
			l.partialSettled = true
		}
		if !t.More {
			done = true
		}
	}
	id, settled := l.partialID, l.partialSettled
	var payload []byte
	if done {
		payload = append([]byte(nil), l.partial...)
		l.partialActive = false
		l.partial = l.partial[:0]
		l.deliveryCount++
	}
	flow := false
	if z.inUsed >= 1000 {
		z.inUsed = 0
		flow = true
	}
	if done {
		if err := b.processMessage(c, l, id, settled, payload); err != nil {
			return err
		}
	}
	if flow {
		return c.out(ch, &frames.PerformFlow{NextIncomingID: u32(z.nextIn), IncomingWindow: 2048, NextOutgoingID: uint32(z.nextOut), OutgoingWindow: 2048})
	}
	return nil
}

func settleReply(c *svConn, ch, id int, settled bool, state encoding.DeliveryState) error {
	if settled {
		return nil
	}
	return c.out(ch, &frames.PerformDisposition{Role: encoding.RoleReceiver, First: uint32(id), Settled: true, State: state})
}

func (b *Broker) processMessage(c *svConn, l *svLink, id int, settled bool, payload []byte) error {
	m := new(amqp.Message)
	if err := m.UnmarshalBinary(payload); err != nil {
		return settleReply(c, l.channel, id, settled, &encoding.StateRejected{Error: &encoding.Error{Condition: "amqp:decode-error", Description: "the message does not decode"}})
	}
	now := nowMs()
	var state encoding.DeliveryState
	b.mu.Lock()
	if q := b.findQueue(l.queue); q == nil {
		state = &encoding.StateRejected{Error: &encoding.Error{Condition: "amqp:not-found", Description: "no such entity " + l.queue}}
	} else {
		b.enqueue(q, m, now)
		state = &encoding.StateAccepted{}
	}
	b.mu.Unlock()
	return settleReply(c, l.channel, id, settled, state)
}

func (b *Broker) sendMessage(c *svConn, l *svLink, z *svSession, msg *amqp.Message, tag []byte, settled bool) (int, error) {
	payload, err := msg.MarshalBinary()
	if err != nil {
		return 0, err
	}
	id := z.nextOut
	z.nextOut++
	// the frame overhead of a first transfer, as std/amqptest measures it with a probe
	var probe buffer.Buffer
	if err := frames.Write(&probe, frames.Frame{Type: frames.TypeAMQP, Channel: uint16(l.channel), Body: &frames.PerformTransfer{Handle: uint32(l.handle),
		DeliveryID: u32(id), DeliveryTag: tag, MessageFormat: u32(0), More: true}}); err != nil {
		return 0, err
	}
	chunk := c.maxFrame - probe.Len()
	if chunk < 1 {
		chunk = 1
	}
	off, first := 0, true
	for first || off < len(payload) {
		end := off + chunk
		if end > len(payload) {
			end = len(payload)
		}
		t := &frames.PerformTransfer{Handle: uint32(l.handle), Settled: settled, More: end < len(payload), Payload: payload[off:end]}
		if first {
			t.DeliveryID = u32(id)
			t.DeliveryTag = tag
			t.MessageFormat = u32(0)
		}
		if err := c.out(l.channel, t); err != nil {
			return 0, err
		}
		z.remoteWindow--
		first = false
		off = end
	}
	l.deliveryCount++
	l.credit--
	return id, nil
}

func (b *Broker) deliverLink(c *svConn, l *svLink) error {
	for {
		if !l.sends || l.detached {
			return nil
		}
		z := c.session(l.channel)
		if z == nil || z.ended || l.credit <= 0 || z.remoteWindow <= 0 {
			return nil
		}
		now := nowMs()
		b.mu.Lock()
		var m fakeMsg
		ok := false
		if q := b.findQueue(l.queue); q != nil {
			m, ok = b.take(q, c.id, now, l.presettled)
		}
		b.mu.Unlock()
		if !ok {
			return nil
		}
		msg := withDeliveryAnnotations(m)
		tag := tokenBytes(m.seq)
		if len(m.lockToken) > 0 {
			tag = guidOrder(m.lockToken)
		}
		id, err := b.sendMessage(c, l, z, msg, tag, l.presettled)
		if err != nil {
			return err
		}
		if !l.presettled {
			l.unsettled = append(l.unsettled, unsettled{id, m.seq})
		}
	}
}

func (b *Broker) deliverAll(c *svConn) error {
	for _, l := range c.links {
		if err := b.deliverLink(c, l); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) onDisposition(c *svConn, ch int, d *frames.PerformDisposition) error {
	if d.Role == encoding.RoleSender {
		return nil
	}
	first := int(d.First)
	last := first
	if d.Last != nil {
		last = int(*d.Last)
	}
	state := d.State
	if state == nil {
		state = &encoding.StateAccepted{}
	}
	now := nowMs()
	answer := state
	for _, l := range c.links {
		if l.channel != ch || !l.sends || len(l.unsettled) == 0 {
			continue
		}
		var keep []unsettled
		for _, u := range l.unsettled {
			if u.id < first || u.id > last {
				keep = append(keep, u)
				continue
			}
			b.mu.Lock()
			q := b.findQueue(l.queue)
			if q == nil {
				answer = state
			} else {
				expireLocks(q, now)
				mi := findSeq(q, u.seq)
				if mi < 0 || q.msgs[mi].lockedUntil == 0 || q.msgs[mi].lockConn != c.id {
					answer = &encoding.StateRejected{Error: &encoding.Error{Condition: "com.microsoft:message-lock-lost", Description: "The lock supplied is invalid. Either the lock expired, or the message has already been removed from the queue."}}
				} else {
					switch state.(type) {
					case *encoding.StateAccepted:
						complete(q, u.seq)
					case *encoding.StateReleased:
						unlock(q, u.seq)
					}
					answer = state
				}
			}
			b.mu.Unlock()
		}
		l.unsettled = keep
	}
	if !d.Settled {
		if err := c.out(ch, &frames.PerformDisposition{Role: encoding.RoleSender, First: d.First, Last: d.Last, Settled: true, State: answer}); err != nil {
			return err
		}
	}
	return b.deliverAll(c)
}
