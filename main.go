package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
)

type ChatServer struct {
	mu    sync.Mutex
	Rooms map[string]map[*websocket.Conn]struct{}
}

func (cs *ChatServer) GetRooms() []string {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	rooms := make([]string, 0, len(cs.Rooms))
	for room := range cs.Rooms {
		rooms = append(rooms, room)
	}
	return rooms
}

func (cs *ChatServer) Join(room string, conn *websocket.Conn) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, ok := cs.Rooms[room]; !ok {
		cs.Rooms[room] = make(map[*websocket.Conn]struct{})
	}
	cs.Rooms[room][conn] = struct{}{}
}

func (cs *ChatServer) Leave(room string, conn *websocket.Conn) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, ok := cs.Rooms[room]; ok {
		delete(cs.Rooms[room], conn)
		if len(cs.Rooms[room]) == 0 {
			delete(cs.Rooms, room)
		}
	}
}

func (cs *ChatServer) Broadcast(room string, msg []byte) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if clients, ok := cs.Rooms[room]; ok {
		for client := range clients {
			err := client.WriteMessage(websocket.TextMessage, msg)
			if err != nil {
				log.Println(err)
				continue
			}
		}
	}
}

var chatServer *ChatServer

func main() {
	// Create Fiber application
	app := fiber.New()

	// Set WebSocket configuration
	config := websocket.Config{
		HandshakeTimeout:  0,             // No timeout for handshake
		Origins:           []string{"*"}, // Allow all origins
		EnableCompression: false,         // Disable compression
		ReadBufferSize:    1024,
		WriteBufferSize:   1024,
	}

	// Initialize ChatServer globally
	chatServer = &ChatServer{
		Rooms: make(map[string]map[*websocket.Conn]struct{}),
	}

	// Use WebSocket middleware in Fiber application
	app.Get("/ws/:room/:username/:role", websocket.New(handleWebSocket, config))

	// Add endpoint to fetch all room names
	app.Get("/rooms", websocket.New(Roomhandle, config))

	// Start Fiber server on port 3000
	app.Listen(":3000")
}

func Roomhandle(c *websocket.Conn) {
	var prevRooms []string // Store previous room names

	for {
		// Create a new message when there is a change in chatServer
		rooms := chatServer.GetRooms()

		// Check for changes
		if len(rooms) != len(prevRooms) {
			message := fiber.Map{
				"rooms": rooms,
			}
			// Send the updated room list to the client
			err := c.WriteJSON(message)
			if err != nil {
				log.Println(err)
				break
			}

			prevRooms = rooms
		}

		// Wait for 1 second before checking again
		time.Sleep(1 * time.Second)
	}
}

func handleWebSocket(conn *websocket.Conn) {
	// Get room name and username from the URL parameters
	room := conn.Params("room")
	username := conn.Params("username")
	role := conn.Params("role")
	if role == "" {
		role = "client"
	}

	chatServer.Join(room, conn)
	defer chatServer.Leave(room, conn)
	defer conn.Close()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			log.Println(fmt.Sprintf("Error: %s", err))
			break
		}
		fmt.Println("Received:", string(msg))

		// Create a JSON message including the username
		message := map[string]string{
			"username": username,
			"message":  string(msg),
			"role":     role,
		}
		messageBytes, _ := json.Marshal(message)

		chatServer.Broadcast(room, messageBytes)
	}
}
