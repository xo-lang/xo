// The Go twin of bench/perf/amqptest/main.xo: go-amqp v1.7.0 (a copy in
// goamqp/, MIT) against goamqp/twinbroker, a Go port of std/amqptest's
// broker, over 127.0.0.1:0. Same workloads, same output.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	amqp "github.com/Azure/go-amqp"
	"github.com/Azure/go-amqp/twinbroker"
)

func body(i int) []byte {
	head := fmt.Sprintf("message %d ", i)
	return []byte(head + strings.Repeat("x", 100-len(head)))
}

func sumOf(m *amqp.Message) int {
	b := m.Data[0]
	return len(b) + int(b[8]) + int(b[9])
}

func must(err error, what string) {
	if err != nil {
		fmt.Fprintln(os.Stderr, what+":", err)
		os.Exit(1)
	}
}

func main() {
	w := "none"
	if len(os.Args) > 1 {
		w = os.Args[1]
	}
	n := 1
	if w != "none" {
		n = 2000
		if v, err := strconv.Atoi(os.Getenv("N")); err == nil {
			n = v
		}
	}
	b := twinbroker.New()
	b.Queue("orders")
	l, err := net.Listen("tcp", "127.0.0.1:0")
	must(err, "listen")
	go b.Serve(l)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	c, err := amqp.Dial(ctx, "amqp://"+l.Addr().String(), &amqp.ConnOptions{ContainerID: "bench", SASLType: amqp.SASLTypeAnonymous()})
	must(err, "dial")
	sess, err := c.NewSession(ctx, nil)
	must(err, "begin")
	snd, err := sess.NewSender(ctx, "orders", nil)
	must(err, "sender")
	rcv, err := sess.NewReceiver(ctx, "orders", &amqp.ReceiverOptions{Credit: 100})
	must(err, "receiver")
	send := func(i int) {
		must(snd.Send(ctx, amqp.NewMessage(body(i)), nil), "send")
	}
	take := func() int {
		m, err := rcv.Receive(ctx, nil)
		must(err, "receive")
		must(rcv.AcceptMessage(ctx, m), "accept")
		return sumOf(m)
	}
	sum := 0
	if w == "pipe" {
		for i := 0; i < n; i++ {
			send(i)
			sum += take()
		}
	} else {
		for i := 0; i < n; i++ {
			send(i)
		}
		for i := 0; i < n; i++ {
			sum += take()
		}
	}
	must(rcv.Close(ctx), "close receiver")
	must(snd.Close(ctx), "close sender")
	must(sess.Close(ctx), "end")
	must(c.Close(), "close")
	l.Close()
	fmt.Printf("%s %d\n", w, sum)
}
