// The reference side of the WebSocket differential test (internal/stdtest
// TestWsFuzz): for each case name on standard input, gorilla/websocket
// v1.5.3's client dials BASE/NAME and reads messages until an error,
// printing each message's kind and data and then the error, as
// scripts/wsdiff/xo prints std/websocket's.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/gorilla/websocket"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: gorilla ws://HOST:PORT < cases")
		os.Exit(2)
	}
	base := os.Args[1]
	sc := bufio.NewScanner(os.Stdin)
	for sc.Scan() {
		name := strings.TrimSpace(sc.Text())
		if name == "" {
			continue
		}
		fmt.Println("== " + name)
		d := websocket.Dialer{}
		c, _, err := d.Dial(base+"/"+name, nil)
		if err != nil {
			fmt.Println("error dial")
			continue
		}
		// std/websocket's default max_message
		c.SetReadLimit(1048576)
		for {
			kind, data, err := c.ReadMessage()
			if err != nil {
				fmt.Println("error " + err.Error())
				break
			}
			k := "binary"
			if kind == websocket.TextMessage {
				k = "text"
			}
			fmt.Printf("msg %s %x\n", k, data)
		}
		c.Close()
	}
}
