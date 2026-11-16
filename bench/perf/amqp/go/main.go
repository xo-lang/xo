// Go twin of bench/perf/amqp: go-amqp v1.7.0's Message MarshalBinary and
// UnmarshalBinary over the same messages.
package main

import (
	"fmt"
	"os"
	"time"

	amqp "github.com/Azure/go-amqp"
)

func msg(i int, body []byte) *amqp.Message {
	ct := "application/json"
	to := "queue/orders"
	subj := "order.created"
	created := time.UnixMilli(1763251200000 + int64(i))
	return &amqp.Message{
		Header:                &amqp.MessageHeader{Durable: true, Priority: 4},
		Properties:            &amqp.MessageProperties{MessageID: fmt.Sprintf("m-%d", i), To: &to, Subject: &subj, ContentType: &ct, CreationTime: &created},
		ApplicationProperties: map[string]any{"tenant": "acme", "attempt": int64(i % 5)},
		Data:                  [][]byte{body},
	}
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	body := make([]byte, 256)
	for k := range body {
		body[k] = byte(k)
	}
	sum := 0
	switch w {
	case "encode":
		for i := 0; i < 300000; i++ {
			b, _ := msg(i, body).MarshalBinary()
			sum += len(b)
		}
	case "decode":
		enc := make([][]byte, 100)
		for i := range enc {
			enc[i], _ = msg(i, body).MarshalBinary()
		}
		for i := 0; i < 300000; i++ {
			var m amqp.Message
			if err := m.UnmarshalBinary(enc[i%100]); err != nil {
				sum += 100000
				continue
			}
			sum += len(m.Data[0])
			if m.Properties != nil && m.Properties.Subject != nil {
				sum += len(*m.Properties.Subject)
			}
		}
	}
	fmt.Printf("%s %d\n", w, sum)
}
