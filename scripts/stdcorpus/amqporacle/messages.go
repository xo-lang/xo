package main

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	amqp "github.com/Azure/go-amqp"
)

func sp(s string) *string  { return &s }
func tp(ms int64) *time.Time { t := time.UnixMilli(ms).UTC(); return &t }
func up(n uint32) *uint32  { return &n }

func showStr(p *string) string {
	if p == nil {
		return "-"
	}
	return hex.EncodeToString([]byte(*p))
}

func showTime(t *time.Time) string {
	if t == nil {
		return "-"
	}
	return fmt.Sprint(t.UnixMilli())
}

func showAnn(a amqp.Annotations) string {
	if a == nil {
		return "-"
	}
	return show(map[any]any(a))
}

// showMsg is the canonical text of a decoded message, matched by std/amqp's test.
func showMsg(m *amqp.Message) string {
	var parts []string
	if h := m.Header; h == nil {
		parts = append(parts, "h=-")
	} else {
		parts = append(parts, fmt.Sprintf("h=%v,%d,%d,%v,%d", h.Durable, h.Priority, h.TTL.Milliseconds(), h.FirstAcquirer, h.DeliveryCount))
	}
	parts = append(parts, "da="+showAnn(m.DeliveryAnnotations), "ann="+showAnn(m.Annotations))
	if p := m.Properties; p == nil {
		parts = append(parts, "p=-")
	} else {
		gs := "-"
		if p.GroupSequence != nil {
			gs = fmt.Sprint(*p.GroupSequence)
		}
		parts = append(parts, "p="+strings.Join([]string{
			show(p.MessageID), hex.EncodeToString(p.UserID), showStr(p.To), showStr(p.Subject), showStr(p.ReplyTo),
			show(p.CorrelationID), showStr(p.ContentType), showStr(p.ContentEncoding), showTime(p.AbsoluteExpiryTime),
			showTime(p.CreationTime), showStr(p.GroupID), gs, showStr(p.ReplyToGroupID)}, ";"))
	}
	if m.ApplicationProperties == nil {
		parts = append(parts, "ap=-")
	} else {
		parts = append(parts, "ap="+show(m.ApplicationProperties))
	}
	switch {
	case len(m.Data) > 0:
		var ds []string
		for _, d := range m.Data {
			ds = append(ds, hex.EncodeToString(d))
		}
		parts = append(parts, "body=data["+strings.Join(ds, ",")+"]")
	case len(m.Sequence) > 0:
		var ss []string
		for _, s := range m.Sequence {
			ss = append(ss, show(s))
		}
		parts = append(parts, "body=seq["+strings.Join(ss, ",")+"]")
	case m.Value != nil:
		parts = append(parts, "body=value "+show(m.Value))
	default:
		parts = append(parts, "body=empty")
	}
	parts = append(parts, "f="+showAnn(m.Footer))
	return strings.Join(parts, "|")
}

type msgCase struct {
	name string
	m    *amqp.Message
}

func messageCases() []msgCase {
	return []msgCase{
		{"empty body", &amqp.Message{}},
		{"data", &amqp.Message{Data: [][]byte{[]byte("hello")}}},
		{"two data sections", &amqp.Message{Data: [][]byte{[]byte("a"), {}, []byte("ccc")}}},
		{"value string", &amqp.Message{Value: "text"}},
		{"value map", &amqp.Message{Value: map[string]any{"k": int64(1), "z": []any{true, "x"}}}},
		{"sequence", &amqp.Message{Sequence: [][]any{{int32(1), "a"}, {}, {uint8(7)}}}},
		{"header default", &amqp.Message{Header: &amqp.MessageHeader{Priority: 4}, Data: [][]byte{[]byte("x")}}},
		{"header full", &amqp.Message{Header: &amqp.MessageHeader{Durable: true, Priority: 9, TTL: 90 * time.Second, FirstAcquirer: true, DeliveryCount: 3}, Data: [][]byte{[]byte("x")}}},
		{"header priority 0", &amqp.Message{Header: &amqp.MessageHeader{Priority: 0}, Data: [][]byte{[]byte("x")}}},
		{"delivery annotations", &amqp.Message{DeliveryAnnotations: amqp.Annotations{"a": "b", amqp.Symbol("s"): int64(2)}, Data: [][]byte{[]byte("x")}}},
		{"message annotations", &amqp.Message{Annotations: amqp.Annotations{"x-opt-key": "v", int64(5): true}, Data: [][]byte{[]byte("x")}}},
		{"empty annotations", &amqp.Message{Annotations: amqp.Annotations{}, Data: [][]byte{[]byte("x")}}},
		{"properties ids", &amqp.Message{Properties: &amqp.MessageProperties{MessageID: "id-1", CorrelationID: uint64(77)}, Data: [][]byte{[]byte("x")}}},
		{"properties uuid and binary ids", &amqp.Message{Properties: &amqp.MessageProperties{MessageID: uuid(3), CorrelationID: []byte{1, 2, 3}}, Data: [][]byte{[]byte("x")}}},
		{"properties all", &amqp.Message{Properties: &amqp.MessageProperties{MessageID: "m", UserID: []byte("u"), To: sp("to"), Subject: sp("subj"), ReplyTo: sp("rt"), CorrelationID: "c", ContentType: sp("text/plain"), ContentEncoding: sp("gzip"), AbsoluteExpiryTime: tp(1700000000123), CreationTime: tp(1600000000000), GroupID: sp("g"), GroupSequence: up(4), ReplyToGroupID: sp("rg")}, Data: [][]byte{[]byte("x")}}},
		{"properties empty", &amqp.Message{Properties: &amqp.MessageProperties{}, Data: [][]byte{[]byte("x")}}},
		{"properties only subject", &amqp.Message{Properties: &amqp.MessageProperties{Subject: sp("s")}}},
		{"application properties", &amqp.Message{ApplicationProperties: map[string]any{"a": int32(1), "b": "two", "c": true}, Data: [][]byte{[]byte("x")}}},
		{"empty application properties", &amqp.Message{ApplicationProperties: map[string]any{}, Data: [][]byte{[]byte("x")}}},
		{"footer", &amqp.Message{Footer: amqp.Annotations{"sig": "abc"}, Data: [][]byte{[]byte("x")}}},
		{"everything", &amqp.Message{Header: &amqp.MessageHeader{Durable: true, Priority: 4}, DeliveryAnnotations: amqp.Annotations{"d": int8(1)}, Annotations: amqp.Annotations{"m": "v"}, Properties: &amqp.MessageProperties{MessageID: "x", To: sp("t")}, ApplicationProperties: map[string]any{"k": "v"}, Value: "body", Footer: amqp.Annotations{"f": uint8(1)}}},
		{"large data", &amqp.Message{Data: [][]byte{make([]byte, 300)}}},
	}
}

func messageRows() []row {
	var rows []row
	for _, c := range messageCases() {
		b, err := c.m.MarshalBinary()
		if err != nil {
			panic(fmt.Sprintf("%s: %v", c.name, err))
		}
		var back amqp.Message
		if err := back.UnmarshalBinary(b); err != nil {
			panic(fmt.Sprintf("%s: unmarshal: %v", c.name, err))
		}
		rows = append(rows, row{c.name, b, showMsg(&back)})
	}
	return rows
}
