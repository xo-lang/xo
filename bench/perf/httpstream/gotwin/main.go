// The Go twin of bench/perf/httpstream's client: MODE=stream reads
// resp.Body in 64 KiB pieces and drops them; MODE=send io.ReadAll's it,
// as Xo's Client.send holds the body.
package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = "127.0.0.1:18097"
	}
	size := os.Getenv("SIZE")
	if size == "" {
		size = "104857600"
	}
	c := &http.Client{Timeout: 300 * time.Second}
	resp, err := c.Get("http://" + addr + "/big?n=" + size)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	total := 0
	if os.Getenv("MODE") == "send" {
		b, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(err)
		}
		total = len(b)
	} else {
		buf := make([]byte, 65536)
		for {
			n, err := resp.Body.Read(buf)
			total += n
			if err == io.EOF {
				break
			}
			if err != nil {
				panic(err)
			}
		}
	}
	fmt.Println(total)
}
