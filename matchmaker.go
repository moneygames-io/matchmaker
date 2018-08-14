package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/go-redis/redis"
)

type Matchmaker struct {
	StatusChannels     map[string](chan string)
	GameserverChannels map[string](chan string)
	GameServerRedis    *redis.Client
	PlayerRedis        *redis.Client
	TargetClients      int
}

func NewMatchmaker(target int) *Matchmaker {
	gameServerRedis := connectToRedis("redis-gameservers:6379")
	playerRedis := connectToRedis("redis-players:6379")
	statusChannels := make(map[string](chan string))
	gameserverChannels := make(map[string](chan string))
	return &Matchmaker{statusChannels, gameserverChannels, gameServerRedis, playerRedis, target}
}

func connectToRedis(addr string) *redis.Client {
	var client *redis.Client
	for {
		client = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: "",
			DB:       0,
		})
		_, err := client.Ping().Result()
		if err != nil {
			fmt.Println("Could not connect to redis")
			fmt.Println(err)
		} else {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Connected to redis")
	return client
}

func (m *Matchmaker) getIdleGameserver() string {
	gameServerRedis := m.GameServerRedis

	keys, _ := gameServerRedis.Keys("*").Result()

	for _, key := range keys {
		status, _ := gameServerRedis.HGet(key, "status").Result()
		if status == "idle" {
			gameServerRedis.HSet(key, "players", strconv.Itoa(len(m.StatusChannels)))

			for {
				currentStatus, _ := gameServerRedis.HGet(key, "status").Result()
				if currentStatus != "ready" {
					// TODO This feels bad
					time.Sleep(1000 * time.Millisecond)
				} else {
					return key
				}
			}
		}
	}

	return ""
}

// TODO make blocking?
func (m *Matchmaker) PlayerJoined(conn *websocket.Conn) {

	message := &RegisterMessage{}
	error := conn.ReadJSON(message)
	if error != nil || !validateToken(message.Token, m.PlayerRedis) {
		fmt.Println("Closing connection, token invalid", error, message)
		conn.Close()
	}

	status := make(chan string)
	m.StatusChannels[message.Token] = status

	gameserver := make(chan string)
	m.GameserverChannels[message.Token] = gameserver

	go monitorDisconect(conn, status)
	go m.syncMatchmaker(conn, status, gameserver, message.Token)

	m.updateStatus()

	if len(m.StatusChannels) == m.TargetClients {

		selectedPort := m.getIdleGameserver()

		for _, gameChannel := range m.GameserverChannels {
			gameChannel <- selectedPort
		}
	}
}

func (m *Matchmaker)updateStatus(){
	for _, statusChannel := range m.StatusChannels {
		statusChannel <- "nop"
	}
}

func monitorDisconect(conn *websocket.Conn, status chan string){
	msg := &Msg{}
	if err := conn.ReadJSON(msg); err != nil {
		status <- "dc"
	}
}

func validateToken(token string, playerRedis *redis.Client) bool {
	status, _ := playerRedis.HGet(token, "status").Result()
	fmt.Println(status)
	return status == "paid"
}

func (m *Matchmaker) syncMatchmaker(conn *websocket.Conn, status chan string, gameserver chan string, token string) {
	for {
		select {
		case msg:= <-status:
			if msg == "dc"{
				fmt.Println("DISCONNECTING")
				delete(m.StatusChannels, token)
				delete(m.GameserverChannels, token)
				m.updateStatus()
				return
			}
			if err := conn.WriteJSON(map[string]map[string]int{
				"Status": {
					"CurrentClients": len(m.StatusChannels),
					"TargetClients":  m.TargetClients,
				},
			}); err != nil {
				fmt.Println(err)
			}
		case gs := <-gameserver:
			if err := conn.WriteJSON(map[string]string{
				"Port": gs,
			}); err != nil {
				fmt.Println(err)
			}
			conn.Close()
		}
	}
}
