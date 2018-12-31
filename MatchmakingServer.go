package main

import (
	"fmt"
	"net/http"
	"os"
	"strconv"

	"github.com/gorilla/websocket"
)

var matchmaker *Matchmaker

func main() {
	fmt.Println("Matchmaker started")

	time := getTime()
	minPlayers := minPlayers()

	matchmaker = NewMatchmaker(minPlayers, time)

	http.HandleFunc("/ws", wsHandler)
	panic(http.ListenAndServe(":8000", nil))
}

func minPlayers() int {
	players, present := os.LookupEnv("MM_MIN_PLAYERS")
	if present {
		val, err := strconv.Atoi(players)
		if err == nil {
			return val
		}
	}

	return 2
}

func getTime() int {
	timeString, present := os.LookupEnv("MM_TIMER_SECONDS")
	if present {
		val, err := strconv.Atoi(timeString)
		if err == nil {
			return val
		}
	}

	return 15
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Upgrade(w, r, w.Header(), 1024, 1024)
	if err != nil {
		http.Error(w, "Could not open websocket connection", http.StatusBadRequest)
	}

	matchmaker.PlayerJoined(conn)
}
