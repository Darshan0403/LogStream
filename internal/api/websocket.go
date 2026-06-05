// internal/api/websocket.go
package api

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/logstream/internal/models"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// In production, you would restrict this to specific origins
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []models.LogEntry // Buffered channel for outbound messages
	serviceFilter string                 // Filter logs by service (optional)
	levelFilter   string                 // Filter logs by level (optional)
}

// Hub maintains the set of active clients and broadcasts messages to the clients.
type Hub struct {
	clients    map[*Client]bool
	mu         sync.RWMutex
	broadcast  chan []models.LogEntry
	register   chan *Client
	unregister chan *Client
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		broadcast:  make(chan []models.LogEntry),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		clients:    make(map[*Client]bool),
	}
}

// Run starts the hub's main loop to handle client registration and unregistration.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()
			log.Println("WebSocket client registered. Active clients:", len(h.clients))
		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				log.Println("WebSocket client unregistered. Active clients:", len(h.clients))
			}
			h.mu.Unlock()
		}
	}
}

// Broadcast sends a batch of logs to all connected, filtered clients.
func (h *Hub) Broadcast(batch []models.LogEntry) {
	if len(batch) == 0 {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for client := range h.clients {
		// Filter the batch for this specific client
		var filtered []models.LogEntry
		for _, logEntry := range batch {
			if client.serviceFilter != "" && client.serviceFilter != logEntry.Service {
				continue
			}
			if client.levelFilter != "" && client.levelFilter != logEntry.Level {
				continue
			}
			filtered = append(filtered, logEntry)
		}

		if len(filtered) > 0 {
			// Non-blocking send. If the client buffer is full, drop the oldest message.
			select {
			case client.send <- filtered:
			default:
				log.Println("Warning: Slow client detected, dropping oldest batch")
				select {
				case <-client.send: // drain one old message
				default:
				}
				client.send <- filtered
			}
		}
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case batch, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// The hub closed the channel.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			if err := c.conn.WriteJSON(batch); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// readPump pumps messages from the websocket connection to the hub.
// The application runs readPump in a per-connection goroutine. The application
// ensures that there is at most one reader on a connection by executing all
// reads from this goroutine.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("error: %v", err)
			}
			break
		}
	}
}

// ServeWS handles websocket requests from the peer.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request, apiKey string) {
	// Validate API Key passed as query parameter
	key := r.URL.Query().Get("key")
	if key != apiKey {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Upgrade error:", err)
		return
	}

	client := &Client{
		hub:           h,
		conn:          conn,
		send:          make(chan []models.LogEntry, 256),
		serviceFilter: r.URL.Query().Get("service"),
		levelFilter:   r.URL.Query().Get("level"),
	}

	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
