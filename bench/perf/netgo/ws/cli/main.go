package main

import (
	"flag"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

func main() {
	url := flag.String("url", "ws://127.0.0.1:9001/", "")
	conns := flag.Int("c", 8, "")
	size := flag.Int("size", 64, "")
	d := flag.Duration("d", 5*time.Second, "")
	flag.Parse()
	var n atomic.Int64
	var wg sync.WaitGroup
	end := time.Now().Add(*d)
	msg := make([]byte, *size)
	for i := range msg {
		msg[i] = 'a' + byte(i%26)
	}
	for i := 0; i < *conns; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, _, err := websocket.DefaultDialer.Dial(*url, nil)
			if err != nil {
				fmt.Println("dial:", err)
				return
			}
			defer c.Close()
			for time.Now().Before(end) {
				if c.WriteMessage(websocket.TextMessage, msg) != nil {
					return
				}
				if _, _, err := c.ReadMessage(); err != nil {
					return
				}
				n.Add(1)
			}
		}()
	}
	wg.Wait()
	fmt.Printf("msgs/s %.0f (size %d, conns %d)\n", float64(n.Load())/d.Seconds(), *size, *conns)
}
