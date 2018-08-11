package main

import (
	"fmt"
	"strconv"
	"time"

	"github.com/gorilla/websocket"

	"github.com/go-redis/redis"
)

type Matchmaker struct {
	StatusChannels     []chan string
	GameserverChannels []chan string
	RedisClient        *redis.Client
	CurrentClients     int
	TargetClients      int
}

func NewMatchmaker(target int) *Matchmaker {
	var client *redis.Client
	for {
		fmt.Println("Attempting to connect to redis")
		client = redis.NewClient(&redis.Options{
			Addr:     "redis-gameservers:6379",
			Password: "",
			DB:       0,
		})
		_, err := client.Ping().Result()
		if err != nil {
			fmt.Println("Matchmaker could not connect to redis")
			fmt.Println(err)
		} else {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	fmt.Println("Connected to redis")
	return &Matchmaker{nil, nil, client, 0, target}
}

func (m *Matchmaker) getIdleGameserver() string {
	c := m.RedisClient

	keys, _ := c.Keys("*").Result()

	for _, key := range keys {
		status, _ := c.HGet(key, "status").Result()
		if status == "idle" {
			c.HSet(key, "players", strconv.Itoa(m.CurrentClients), 0)

			for {
				currentStatus := c.HGet(key, "status")
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

func (m *Matchmaker) PlayerJoined(conn *websocket.Conn) {
	m.CurrentClients++

	status := make(chan string)
	m.StatusChannels = append(m.StatusChannels, status)

	gameserver := make(chan string)
	m.GameserverChannels = append(m.GameserverChannels, gameserver)

	go m.syncMatchmaker(conn, status, gameserver)

	for _, statusChannel := range m.StatusChannels {
		statusChannel <- "nop"
	}

	if m.CurrentClients == m.TargetClients {

		selectedPort := m.getIdleGameserver()

		for _, gameChannel := range m.GameserverChannels {
			gameChannel <- selectedPort
		}

		m.CurrentClients = 0
		//TODO  cleanup here
	}
}

func (m *Matchmaker) syncMatchmaker(conn *websocket.Conn, status chan string, gameserver chan string) {
	for {
		select {
		case <-status:
			if err := conn.WriteJSON(map[string]map[string]int{
				"Status": {
					"CurrentClients": m.CurrentClients,
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
