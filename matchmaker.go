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
	GameServerRedis        *redis.Client
	CurrentClients     int
	TargetClients      int
}

func NewMatchmaker(target int) *Matchmaker {
	gameServerRedis := connectToRedis("redis-gameservers:6379")
	return &Matchmaker{nil, nil, gameServerRedis, 0, target}
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
			fmt.Println("gameserver could not connect to redis")
			fmt.Println(err)
		} else {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Println("gameserver connected to redis")

	return client
}

func (m *Matchmaker) getIdleGameserver() string {
	c := m.GameServerRedis

	keys, _ := c.Keys("*").Result()

	for _, key := range keys {
		status, _ := c.Get(key).Result()
		if status == "idle" {
			err := c.Set(key, strconv.Itoa(m.CurrentClients), 0).Err()

			if err == nil {
				// TODO before returning you need to check to make sure gameserver is on the same page
				return key
			} else {
				fmt.Println("Could not find idle gameserver")
				fmt.Println(err)
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
