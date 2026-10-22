package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
)

func main() {
	n, _ := strconv.Atoi(os.Getenv("N"))
	url := "http://" + os.Getenv("ADDR") + "/" + os.Getenv("W")
	total := 0
	for i := 0; i < n; i++ {
		r, err := http.Get(url)
		if err != nil {
			fmt.Println("error:", err)
			return
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		total += len(b) + len(r.Header)
	}
	fmt.Println(total)
}
