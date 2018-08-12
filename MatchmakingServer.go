package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"net/http"
)

var matchmaker *Matchmaker

func main() {
	fmt.Println("Matchmaker started")
	matchmaker = NewMatchmaker(4)

	http.HandleFunc("/ws", wsHandler)

	panic(http.ListenAndServe(":8000", nil))
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r, w.Header(), 1024, 1024)
	if err != nil {
		http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
	}

	matchmaker.PlayerJoined(conn)
}
