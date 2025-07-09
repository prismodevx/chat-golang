package main

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// Tiempo máximo de espera para escribir un mensaje
	writeWait = 10 * time.Second

	// Tiempo máximo de espera para leer el siguiente mensaje pong
	pongWait = 60 * time.Second

	// Intervalo para enviar pings al peer
	pingPeriod = (pongWait * 9) / 10

	// Tamaño máximo del mensaje
	maxMessageSize = 512
)

var (
	newline = []byte{'\n'}
	space   = []byte{' '}
)

// Upgrader para convertir conexiones HTTP a WebSocket
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		return true // Permitir todas las conexiones para simplificar
	},
}

// Client representa un cliente conectado al chat
type Client struct {
	// El hub de chat al que pertenece este cliente
	hub *Hub

	// La conexión WebSocket
	conn *websocket.Conn

	// Canal con buffer para mensajes salientes
	send chan []byte

	// Nombre de usuario del cliente
	username string
}

// readPump maneja la lectura de mensajes del cliente
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, messageBytes, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("Error de WebSocket: %v", err)
			}
			break
		}

		// Parsear el mensaje JSON
		var msg Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("Error al parsear mensaje: %v", err)
			continue
		}

		// Completar la información del mensaje
		msg.Username = c.username
		msg.Timestamp = time.Now()

		// Enviar el mensaje al hub para difusión
		c.hub.broadcast <- msg
	}
}

// writePump maneja el envío de mensajes al cliente
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// El hub cerró el canal
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			// Agregar mensajes en cola al mensaje actual
			n := len(c.send)
			for i := 0; i < n; i++ {
				w.Write(newline)
				w.Write(<-c.send)
			}

			if err := w.Close(); err != nil {
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

// ServeWS maneja las solicitudes de WebSocket desde el peer
func ServeWS(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("Error al actualizar conexión:", err)
		return
	}

	// Obtener el nombre de usuario de los parámetros de consulta
	username := r.URL.Query().Get("username")
	if username == "" {
		username = "Usuario Anónimo"
	}

	// Crear nuevo cliente
	client := &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		username: username,
	}

	// Registrar el cliente en el hub
	client.hub.register <- client

	// Permitir la recolección de memoria referenciada por el llamador
	// iniciando goroutines en nuevas goroutines
	go client.writePump()
	go client.readPump()
}
