package main

import (
	"encoding/json"
	"log"
	"sync"
	"time"
)

// MessageType define los tipos de mensajes
type MessageType string

const (
	MessageTypeChat   MessageType = "chat"
	MessageTypeSystem MessageType = "system"
)

// Message representa un mensaje en el chat
type Message struct {
	Type      MessageType `json:"type"`
	Username  string      `json:"username"`
	Content   string      `json:"content"`
	Timestamp time.Time   `json:"timestamp"`
}

// Hub mantiene el conjunto de clientes activos y difunde mensajes a los clientes
type Hub struct {
	// Clientes registrados
	clients map[*Client]bool

	// Mensajes entrantes de los clientes
	broadcast chan Message

	// Registrar solicitudes de los clientes
	register chan *Client

	// Desregistrar solicitudes de los clientes
	unregister chan *Client

	// Mutex para proteger el acceso concurrente al mapa de clientes
	mutex sync.RWMutex
}

// NewHub crea una nueva instancia del hub
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan Message),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run ejecuta el hub principal
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.registerClient(client)

		case client := <-h.unregister:
			h.unregisterClient(client)

		case message := <-h.broadcast:
			h.broadcastMessage(message)
		}
	}
}

// registerClient registra un nuevo cliente
func (h *Hub) registerClient(client *Client) {
	h.mutex.Lock()
	h.clients[client] = true
	h.mutex.Unlock()

	// Enviar mensaje de sistema sobre la nueva conexión
	systemMessage := Message{
		Type:      MessageTypeSystem,
		Username:  "Sistema",
		Content:   client.username + " se ha conectado",
		Timestamp: time.Now(),
	}

	log.Printf("Cliente registrado: %s", client.username)

	// Difundir el mensaje de sistema
	go func() {
		h.broadcast <- systemMessage
	}()
}

// unregisterClient desregistra un cliente
func (h *Hub) unregisterClient(client *Client) {
	h.mutex.Lock()
	if _, ok := h.clients[client]; ok {
		delete(h.clients, client)
		close(client.send)
		h.mutex.Unlock()

		// Enviar mensaje de sistema sobre la desconexión
		systemMessage := Message{
			Type:      MessageTypeSystem,
			Username:  "Sistema",
			Content:   client.username + " se ha desconectado",
			Timestamp: time.Now(),
		}

		log.Printf("Cliente desregistrado: %s", client.username)

		// Difundir el mensaje de sistema
		go func() {
			h.broadcast <- systemMessage
		}()
	} else {
		h.mutex.Unlock()
	}
}

// broadcastMessage difunde un mensaje a todos los clientes conectados
func (h *Hub) broadcastMessage(message Message) {
	messageBytes, err := json.Marshal(message)
	if err != nil {
		log.Printf("Error al serializar mensaje: %v", err)
		return
	}

	h.mutex.RLock()
	clientsCopy := make(map[*Client]bool, len(h.clients))
	for client := range h.clients {
		clientsCopy[client] = true
	}
	h.mutex.RUnlock()

	// Enviar mensaje a todos los clientes
	for client := range clientsCopy {
		select {
		case client.send <- messageBytes:
		default:
			// El cliente no puede recibir el mensaje, desconectarlo
			h.mutex.Lock()
			delete(h.clients, client)
			close(client.send)
			h.mutex.Unlock()
		}
	}
}

// GetClientCount retorna el número de clientes conectados
func (h *Hub) GetClientCount() int {
	h.mutex.RLock()
	defer h.mutex.RUnlock()
	return len(h.clients)
}

// GetClients retorna una copia de los clientes conectados
func (h *Hub) GetClients() map[*Client]bool {
	h.mutex.RLock()
	defer h.mutex.RUnlock()

	clients := make(map[*Client]bool, len(h.clients))
	for client := range h.clients {
		clients[client] = true
	}
	return clients
}
