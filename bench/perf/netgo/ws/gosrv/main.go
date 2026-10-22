package main

import (
	"net/http"
	"os"

	"github.com/gorilla/websocket"
)

var up = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }, ReadBufferSize: 4096, WriteBufferSize: 4096}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		for {
			t, m, err := c.ReadMessage()
			if err != nil {
				return
			}
			if c.WriteMessage(t, m) != nil {
				return
			}
		}
	})
	http.ListenAndServe("127.0.0.1:"+os.Getenv("PORT"), nil)
}
