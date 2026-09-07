// internal/api/websocket.go
package api

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"
	"github.com/logstream/internal/models"
	"github.com/prometheus/client_golang/prometheus"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 512

	clientSendBuffer = 256
	hubBroadcastBuf  = 512
)

// FEATURE L: Prometheus Metrics for WebSocket Connections
var (
	ActiveWSClients = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "logstream_active_websocket_clients",
		Help: "Number of active WebSocket connections streaming logs",
	})
	WSDroppedBatches = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "logstream_websocket_dropped_batches_total",
		Help: "Log batches dropped because a client or the hub could not keep up",
	})
)

func init() {
	prometheus.MustRegister(ActiveWSClients, WSDroppedBatches)
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub           *Hub
	conn          *websocket.Conn
	send          chan []models.LogEntry // Buffered channel for outbound messages
	serviceFilter string                 // Filter logs by service (optional)
	levelFilter   string                 // Filter logs by level (optional)
}

// Hub maintains the set of active clients and broadcasts messages to them.
//
// The clients map is owned exclusively by the Run goroutine: registration,
// unregistration and fan-out all happen there, so no mutex is needed and a slow
// client can never block a producer (H3). Producers hand batches to Run through
// the buffered broadcast channel and never touch client channels directly.
type Hub struct {
	clients        map[*Client]bool
	broadcast      chan []models.LogEntry
	register       chan *Client
	unregister     chan *Client
	allowedOrigins []string
	jwtSecret      []byte
}

// NewHub creates a new Hub instance.
func NewHub(jwtSecret []byte, allowedOrigins []string) *Hub {
	return &Hub{
		broadcast:      make(chan []models.LogEntry, hubBroadcastBuf),
		register:       make(chan *Client),
		unregister:     make(chan *Client),
		clients:        make(map[*Client]bool),
		allowedOrigins: allowedOrigins,
		jwtSecret:      jwtSecret,
	}
}

// Run starts the hub's main loop. It self-restarts on panic (H5).
func (h *Hub) Run() {
	for {
		func() {
			defer func() {
				if r := recover(); r != nil {
					slog.Error("hub: recovered from panic, restarting loop", slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
				}
			}()
			h.loop()
		}()
	}
}

func (h *Hub) loop() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = true
			ActiveWSClients.Inc()
			slog.Info("websocket client registered", slog.Int("active", len(h.clients)))
		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				ActiveWSClients.Dec()
				slog.Info("websocket client unregistered", slog.Int("active", len(h.clients)))
			}
		case batch := <-h.broadcast:
			h.fanout(batch)
		}
	}
}

// fanout runs on the Run goroutine only.
func (h *Hub) fanout(batch []models.LogEntry) {
	for client := range h.clients {
		filtered := batch
		if client.serviceFilter != "" || client.levelFilter != "" {
			filtered = filtered[:0:0]
			for _, e := range batch {
				if client.serviceFilter != "" && client.serviceFilter != e.Service {
					continue
				}
				if client.levelFilter != "" && client.levelFilter != e.Level {
					continue
				}
				filtered = append(filtered, e)
			}
		}
		if len(filtered) == 0 {
			continue
		}
		// Strictly non-blocking: a client that cannot keep up drops this batch.
		select {
		case client.send <- filtered:
		default:
			WSDroppedBatches.Inc()
		}
	}
}

// Broadcast is called by producers (the batcher). Non-blocking: if the hub is
// backed up the batch is dropped rather than stalling ingestion (H3).
func (h *Hub) Broadcast(batch []models.LogEntry) {
	if len(batch) == 0 {
		return
	}
	select {
	case h.broadcast <- batch:
	default:
		WSDroppedBatches.Inc()
	}
}

// writePump pumps messages from the hub to the websocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()
	defer recoverLog("writePump")

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
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()
	defer recoverLog("readPump")

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error { c.conn.SetReadDeadline(time.Now().Add(pongWait)); return nil })
	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				slog.Warn("websocket read error", slog.Any("err", err))
			}
			break
		}
	}
}

// bearerFromProtocols extracts a JWT from a Sec-WebSocket-Protocol header value.
// The browser sends e.g. "logstream, auth.<jwt>"; we return the "<jwt>" part.
func bearerFromProtocols(header string) string {
	for _, p := range strings.Split(header, ",") {
		p = strings.TrimSpace(p)
		if after, ok := strings.CutPrefix(p, "auth."); ok {
			return after
		}
	}
	return ""
}

// recoverLog swallows and logs a panic in a long-lived goroutine (H5).
func recoverLog(where string) {
	if r := recover(); r != nil {
		slog.Error("websocket goroutine recovered from panic", slog.String("where", where), slog.Any("panic", r), slog.String("stack", string(debug.Stack())))
	}
}

// ServeWS handles websocket requests from the peer.
func (h *Hub) ServeWS(w http.ResponseWriter, r *http.Request) {
	// Origin check for the upgrade (H6).
	origin := r.Header.Get("Origin")
	if !originAllowed(origin, h.allowedOrigins) && !sameHostOrigin(origin, r.Host) {
		http.Error(w, "forbidden origin", http.StatusForbidden)
		return
	}

	// Secure JWT validation for the upgrade: HS256 only, expiry required (H1).
	// Prefer the token in the Sec-WebSocket-Protocol header so it stays out of
	// URLs and proxy access logs (M9); fall back to ?token= for non-browser tools.
	tokenStr := bearerFromProtocols(r.Header.Get("Sec-WebSocket-Protocol"))
	if tokenStr == "" {
		tokenStr = r.URL.Query().Get("token")
	}
	if tokenStr == "" {
		http.Error(w, "Missing JWT token", http.StatusUnauthorized)
		return
	}
	token, err := jwt.Parse(
		tokenStr,
		func(t *jwt.Token) (interface{}, error) { return h.jwtSecret, nil },
		jwt.WithValidMethods([]string{"HS256"}),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !token.Valid {
		http.Error(w, "Unauthorized - Invalid or Expired Token", http.StatusUnauthorized)
		return
	}

	upgrader := websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		// Echo back only the non-secret "logstream" subprotocol; the "auth.<jwt>"
		// entry the browser also offers is read from the request header above and
		// never reflected.
		Subprotocols: []string{"logstream"},
		CheckOrigin: func(r *http.Request) bool {
			o := r.Header.Get("Origin")
			return originAllowed(o, h.allowedOrigins) || sameHostOrigin(o, r.Host)
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", slog.Any("err", err))
		return
	}

	client := &Client{
		hub:           h,
		conn:          conn,
		send:          make(chan []models.LogEntry, clientSendBuffer),
		serviceFilter: r.URL.Query().Get("service"),
		levelFilter:   r.URL.Query().Get("level"),
	}

	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
	go client.readPump()
}
