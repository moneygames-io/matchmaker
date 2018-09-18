package main

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/go-redis/redis"
)

type Matchmaker struct {
	GameServerRedis *redis.Client
	PlayerRedis     *redis.Client
	TargetPlayers   int
	TargetTime      int
	Dispatch        bool
	Mutex           *sync.Mutex
	Players         map[string]*websocket.Conn
}

func NewMatchmaker(targetPlayers int, targetTime int) *Matchmaker {
	gameServerRedis := connectToRedis("redis-gameservers:6379")
	playerRedis := connectToRedis("redis-players:6379")
	players := make(map[string]*websocket.Conn)
	mutex := &sync.Mutex{}
	return &Matchmaker{
		gameServerRedis,
		playerRedis,
		targetPlayers,
		targetTime,
		false,
		mutex,
		players,
	}
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
			gameServerRedis.HSet(key, "players", strconv.Itoa(len(m.Players)))
			gameServerRedis.HSet(key, "pot", strconv.Itoa(len(m.Players)))

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

func (m *Matchmaker) ValidateToken(token string) bool {
	status, _ := m.PlayerRedis.HGet(token, "status").Result()
	fmt.Println(status)
	return status == "paid"
}

func (m *Matchmaker) PlayerJoined(conn *websocket.Conn) {
	message := &RegisterMessage{}
	error := conn.ReadJSON(message)
	if error != nil || !m.ValidateToken(message.Token) {
		fmt.Println("Closing connection, token invalid", error, message)
		conn.Close()
	}
	m.Mutex.Lock()
	m.Players[message.Token] = conn   // Add player
	m.RelayPlayers()                  // Update other players
	go m.MonitorPlayer(message.Token) // Check for disconnect
	if len(m.Players) >= m.TargetPlayers && !m.Dispatch {
		m.Dispatch = true
		go m.DispatchPlayers() // Start countdown to start game
	}
	m.Mutex.Unlock()
}

func (m *Matchmaker) DispatchPlayers() {
	time.Sleep(time.Duration(m.TargetTime) * time.Second)
	m.Mutex.Lock()
	if len(m.Players) >= m.TargetPlayers {
		selectedPort := m.getIdleGameserver()
		for _, c := range m.Players {
			if err := c.WriteJSON(map[string]string{
				"Port": selectedPort,
			}); err != nil {
				fmt.Println(err)
			}
			c.Close()
		}
		m.Players = make(map[string]*websocket.Conn)
	}
	m.Dispatch = false
	m.Mutex.Unlock()
}

func (m *Matchmaker) MonitorPlayer(token string) {
	msg := &Msg{}
	if err := m.Players[token].ReadJSON(msg); err != nil {
		m.Mutex.Lock()
		// If the token is still in the players map then it was  disconnect
		if _, exists := m.Players[token]; exists {
			delete(m.Players, token)
			m.RelayPlayers()
		}
		m.Mutex.Unlock()
	}
}

func (m *Matchmaker) RelayPlayers() {
	for _, c := range m.Players {
		if err := c.WriteJSON(map[string]map[string]int{
			"Status": {
				"CurrentClients": len(m.Players),
				"TargetClients":  m.TargetPlayers,
			},
		}); err != nil {
			fmt.Println(err)
		}
	}
}
