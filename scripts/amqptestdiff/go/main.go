// The driver of the amqptest differential test (internal/stdtest
// TestAmqptestFuzz): go-amqp v1.7.0 (vendored) runs a script against the
// std/amqptest broker of scripts/amqptestdiff/xo and prints what it sees.
//
//	driver AMQP_ADDR CONTROL_ADDR < script
//
// A script's lines:
//
//	use Q                  closes the links, declares queue Q (control) and opens a sender on it
//	send BODY              a message with a Data body, on the sender
//	open L lock|delete     receiver L on Q, peek-lock or receive and delete, credit by hand
//	recv L                 one credit and one Receive (2 s); "recv L none" when nothing came
//	settle L K OP...       the K-th message L received: accept, release, modify DF UH,
//	                       reject COND, deadletter REASON (as azservicebus dead letters)
//	close L                close receiver L
//	advance MS | state Q   the broker's control commands
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	amqp "github.com/Azure/go-amqp"
)

func main() {
	ctx := context.Background()
	conn, err := amqp.Dial(ctx, "amqp://"+os.Args[1], &amqp.ConnOptions{SASLType: amqp.SASLTypeAnonymous(), ContainerID: "driver"})
	if err != nil {
		fmt.Println("dial error", err)
		os.Exit(1)
	}
	defer conn.Close()
	ctl, err := net.Dial("tcp", os.Args[2])
	if err != nil {
		fmt.Println("control error", err)
		os.Exit(1)
	}
	ctlr := bufio.NewReader(ctl)
	sess, err := conn.NewSession(ctx, nil)
	if err != nil {
		fmt.Println("session error", err)
		os.Exit(1)
	}
	var snd *amqp.Sender
	queue := ""
	rcvs := map[string]*amqp.Receiver{}
	got := map[string][]*amqp.Message{}
	in := bufio.NewScanner(os.Stdin)
	in.Buffer(make([]byte, 1<<20), 1<<20)
	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		w := strings.Split(line, " ")
		fmt.Println("== " + line)
		ctlCmd := func(cmd string) {
			fmt.Fprintln(ctl, cmd)
			for {
				l, err := ctlr.ReadString('\n')
				if err != nil {
					fmt.Println("control error", err)
					os.Exit(1)
				}
				if l == ".\n" {
					return
				}
				fmt.Print(l)
			}
		}
		switch w[0] {
		case "use":
			for k, r := range rcvs {
				r.Close(ctx)
				delete(rcvs, k)
			}
			got = map[string][]*amqp.Message{}
			if snd != nil {
				snd.Close(ctx)
			}
			queue = w[1]
			ctlCmd("queue " + queue)
			snd, err = sess.NewSender(ctx, queue, nil)
			if err != nil {
				fmt.Println("sender error", err)
				os.Exit(1)
			}
		case "send":
			err := snd.Send(ctx, amqp.NewMessage([]byte(w[1])), nil)
			fmt.Println("sent", errText(err))
		case "open":
			mode := amqp.ReceiverSettleModeSecond.Ptr()
			var smode *amqp.SenderSettleMode
			if w[2] == "delete" {
				mode = nil
				smode = amqp.SenderSettleModeSettled.Ptr()
			}
			r, err := sess.NewReceiver(ctx, queue, &amqp.ReceiverOptions{Credit: -1, SettlementMode: mode, RequestedSenderSettleMode: smode, Name: "r-" + w[1]})
			if err != nil {
				fmt.Println("open", errText(err))
				continue
			}
			rcvs[w[1]] = r
			fmt.Println("open ok")
		case "recv":
			r := rcvs[w[1]]
			if err := r.IssueCredit(1); err != nil {
				fmt.Println("credit", errText(err))
				continue
			}
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			m, err := r.Receive(c, nil)
			cancel()
			if err != nil {
				fmt.Println("recv none")
				continue
			}
			got[w[1]] = append(got[w[1]], m)
			dc := uint32(0)
			if m.Header != nil {
				dc = m.Header.DeliveryCount
			}
			fmt.Printf("recv %s dc %d\n", string(m.GetData()), dc)
		case "settle":
			r := rcvs[w[1]]
			k, _ := strconv.Atoi(w[2])
			m := got[w[1]][k]
			var err error
			switch w[3] {
			case "accept":
				err = r.AcceptMessage(ctx, m)
			case "release":
				err = r.ReleaseMessage(ctx, m)
			case "modify":
				err = r.ModifyMessage(ctx, m, &amqp.ModifyMessageOptions{DeliveryFailed: w[4] == "1", UndeliverableHere: w[5] == "1"})
			case "deadletter":
				err = r.RejectMessage(ctx, m, &amqp.Error{Condition: "com.microsoft:dead-letter", Info: map[string]any{"DeadLetterReason": w[4], "DeadLetterErrorDescription": "dd", "extra": "x"}})
			default:
				err = r.RejectMessage(ctx, m, &amqp.Error{Condition: amqp.ErrCond(w[4]), Description: "d"})
			}
			fmt.Println("settled", errText(err))
		case "close":
			fmt.Println("closed", errText(rcvs[w[1]].Close(ctx)))
			delete(rcvs, w[1])
		default:
			ctlCmd(line)
		}
	}
}

// errText: "ok", or the AMQP error's condition.
func errText(err error) string {
	if err == nil {
		return "ok"
	}
	var ae *amqp.Error
	if errors.As(err, &ae) {
		return "error " + string(ae.Condition)
	}
	return "error " + err.Error()
}
