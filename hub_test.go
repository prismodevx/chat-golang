package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestHub_RegisterClient prueba el registro de clientes
func TestHub_RegisterClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear un cliente mock
	client := &Client{
		hub:      hub,
		conn:     nil, // No necesitamos una conexión real para esta prueba
		send:     make(chan []byte, 256),
		username: "TestUser",
	}

	// Registrar el cliente
	hub.register <- client

	// Esperar un poco para que se procese el registro
	time.Sleep(100 * time.Millisecond)

	// Verificar que el cliente se registró correctamente
	if count := hub.GetClientCount(); count != 1 {
		t.Errorf("Se esperaba 1 cliente, se obtuvo %d", count)
	}

	// Verificar que el cliente está en el mapa
	clients := hub.GetClients()
	if _, exists := clients[client]; !exists {
		t.Error("El cliente no se encontró en el mapa de clientes")
	}
}

// TestHub_UnregisterClient prueba el desregistro de clientes
func TestHub_UnregisterClient(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear un cliente mock
	client := &Client{
		hub:      hub,
		conn:     nil,
		send:     make(chan []byte, 256),
		username: "TestUser",
	}

	// Registrar el cliente
	hub.register <- client
	time.Sleep(100 * time.Millisecond)

	// Verificar que se registró
	if count := hub.GetClientCount(); count != 1 {
		t.Errorf("Se esperaba 1 cliente después del registro, se obtuvo %d", count)
	}

	// Desregistrar el cliente
	hub.unregister <- client
	time.Sleep(100 * time.Millisecond)

	// Verificar que se desregistró
	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Se esperaba 0 clientes después del desregistro, se obtuvo %d", count)
	}
}

// TestHub_BroadcastMessage prueba la difusión de mensajes
func TestHub_BroadcastMessage(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear múltiples clientes mock
	clients := make([]*Client, 3)
	for i := 0; i < 3; i++ {
		clients[i] = &Client{
			hub:      hub,
			conn:     nil,
			send:     make(chan []byte, 256),
			username: "TestUser" + string(rune('1'+i)),
		}
		hub.register <- clients[i]
	}

	// Esperar a que se registren todos los clientes
	time.Sleep(100 * time.Millisecond)

	// Crear un mensaje de prueba
	testMessage := Message{
		Type:      MessageTypeChat,
		Username:  "TestUser1",
		Content:   "Mensaje de prueba",
		Timestamp: time.Now(),
	}

	// Enviar el mensaje al hub
	hub.broadcast <- testMessage

	// Verificar que todos los clientes recibieron el mensaje
	messageBytes, _ := json.Marshal(testMessage)

	for i, client := range clients {
		select {
		case receivedMessage := <-client.send:
			if string(receivedMessage) != string(messageBytes) {
				t.Errorf("Cliente %d recibió un mensaje incorrecto", i)
			}
		case <-time.After(1 * time.Second):
			t.Errorf("Cliente %d no recibió el mensaje en el tiempo esperado", i)
		}
	}
}

// TestHub_ConcurrentAccess prueba el acceso concurrente seguro
func TestHub_ConcurrentAccess(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	const numGoroutines = 100
	const numClientsPerGoroutine = 10

	var wg sync.WaitGroup

	// Crear múltiples goroutines que registren y desregistren clientes
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(goroutineID int) {
			defer wg.Done()

			clients := make([]*Client, numClientsPerGoroutine)

			// Registrar clientes
			for j := 0; j < numClientsPerGoroutine; j++ {
				clients[j] = &Client{
					hub:      hub,
					conn:     nil,
					send:     make(chan []byte, 256),
					username: "User" + string(rune('A'+goroutineID)) + string(rune('0'+j)),
				}
				hub.register <- clients[j]
			}

			// Pequeña pausa
			time.Sleep(10 * time.Millisecond)

			// Desregistrar clientes
			for j := 0; j < numClientsPerGoroutine; j++ {
				hub.unregister <- clients[j]
			}
		}(i)
	}

	// Esperar a que todas las goroutines terminen
	wg.Wait()

	// Dar tiempo para que el hub procese todas las operaciones
	time.Sleep(500 * time.Millisecond)

	// Verificar que no queden clientes registrados
	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Se esperaba 0 clientes al final, se obtuvo %d", count)
	}
}

// TestMessage_JSONSerialization prueba la serialización JSON de mensajes
func TestMessage_JSONSerialization(t *testing.T) {
	originalMessage := Message{
		Type:      MessageTypeChat,
		Username:  "TestUser",
		Content:   "Mensaje de prueba",
		Timestamp: time.Now(),
	}

	// Serializar a JSON
	jsonData, err := json.Marshal(originalMessage)
	if err != nil {
		t.Fatalf("Error al serializar mensaje: %v", err)
	}

	// Deserializar desde JSON
	var deserializedMessage Message
	err = json.Unmarshal(jsonData, &deserializedMessage)
	if err != nil {
		t.Fatalf("Error al deserializar mensaje: %v", err)
	}

	// Verificar que los campos coinciden
	if deserializedMessage.Type != originalMessage.Type {
		t.Errorf("Tipo esperado %s, obtuvo %s", originalMessage.Type, deserializedMessage.Type)
	}

	if deserializedMessage.Username != originalMessage.Username {
		t.Errorf("Usuario esperado %s, obtuvo %s", originalMessage.Username, deserializedMessage.Username)
	}

	if deserializedMessage.Content != originalMessage.Content {
		t.Errorf("Contenido esperado %s, obtuvo %s", originalMessage.Content, deserializedMessage.Content)
	}
}

// TestWebSocketHandler prueba el manejador de WebSocket
func TestWebSocketHandler(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear un servidor de prueba
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWS(hub, w, r)
	}))
	defer server.Close()

	// Convertir http://127.0.0.1 a ws://127.0.0.1
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "?username=TestUser"

	// Conectar al WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatalf("Error al conectar al WebSocket: %v", err)
	}
	defer ws.Close()

	// Enviar un mensaje
	testMessage := Message{
		Type:    MessageTypeChat,
		Content: "Hola, mundo!",
	}

	if err := ws.WriteJSON(testMessage); err != nil {
		t.Fatalf("Error al enviar mensaje: %v", err)
	}

	// Verificar que el cliente se registró
	time.Sleep(100 * time.Millisecond)
	if count := hub.GetClientCount(); count != 1 {
		t.Errorf("Se esperaba 1 cliente conectado, se obtuvo %d", count)
	}
}

// TestSystemMessages prueba los mensajes del sistema
func TestSystemMessages(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear un cliente para recibir mensajes del sistema
	receiver := &Client{
		hub:      hub,
		conn:     nil,
		send:     make(chan []byte, 256),
		username: "Receiver",
	}
	hub.register <- receiver

	// Esperar a que se registre
	time.Sleep(100 * time.Millisecond)

	// Crear otro cliente que genere un mensaje del sistema
	newClient := &Client{
		hub:      hub,
		conn:     nil,
		send:     make(chan []byte, 256),
		username: "NewUser",
	}

	// Registrar el nuevo cliente (esto debería generar un mensaje del sistema)
	hub.register <- newClient

	// Verificar que el receptor recibió el mensaje del sistema
	select {
	case messageBytes := <-receiver.send:
		var msg Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			t.Fatalf("Error al deserializar mensaje del sistema: %v", err)
		}

		if msg.Type != MessageTypeSystem {
			t.Errorf("Se esperaba mensaje del sistema, obtuvo %s", msg.Type)
		}

		if msg.Username != "Sistema" {
			t.Errorf("Se esperaba usuario 'Sistema', obtuvo %s", msg.Username)
		}

		if !strings.Contains(msg.Content, "NewUser") || !strings.Contains(msg.Content, "conectado") {
			t.Errorf("Contenido del mensaje del sistema incorrecto: %s", msg.Content)
		}

	case <-time.After(1 * time.Second):
		t.Error("No se recibió el mensaje del sistema en el tiempo esperado")
	}
}

// BenchmarkHub_BroadcastMessage benchmarks la difusión de mensajes
func BenchmarkHub_BroadcastMessage(b *testing.B) {
	hub := NewHub()
	go hub.Run()

	// Crear múltiples clientes
	numClients := 100
	clients := make([]*Client, numClients)
	for i := 0; i < numClients; i++ {
		clients[i] = &Client{
			hub:      hub,
			conn:     nil,
			send:     make(chan []byte, 256),
			username: "BenchUser" + string(rune('0'+i%10)),
		}
		hub.register <- clients[i]
	}

	// Esperar a que se registren todos los clientes
	time.Sleep(100 * time.Millisecond)

	// Mensaje de prueba
	testMessage := Message{
		Type:      MessageTypeChat,
		Username:  "BenchUser",
		Content:   "Mensaje de benchmark",
		Timestamp: time.Now(),
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		hub.broadcast <- testMessage

		// Drenar los canales de envío para evitar bloqueos
		for _, client := range clients {
			select {
			case <-client.send:
			default:
			}
		}
	}
}

// TestHub_MessageOrder prueba que los mensajes se entreguen en orden
func TestHub_MessageOrder(t *testing.T) {
	hub := NewHub()
	go hub.Run()

	// Crear un cliente receptor
	client := &Client{
		hub:      hub,
		conn:     nil,
		send:     make(chan []byte, 256),
		username: "TestUser",
	}
	hub.register <- client

	// Esperar a que se registre
	time.Sleep(100 * time.Millisecond)

	// Enviar múltiples mensajes en secuencia
	numMessages := 10
	for i := 0; i < numMessages; i++ {
		message := Message{
			Type:      MessageTypeChat,
			Username:  "Sender",
			Content:   "Mensaje " + string(rune('0'+i)),
			Timestamp: time.Now(),
		}
		hub.broadcast <- message
	}

	// Verificar que los mensajes se reciben en el orden correcto
	for i := 0; i < numMessages; i++ {
		select {
		case messageBytes := <-client.send:
			var msg Message
			if err := json.Unmarshal(messageBytes, &msg); err != nil {
				t.Fatalf("Error al deserializar mensaje %d: %v", i, err)
			}

			expectedContent := "Mensaje " + string(rune('0'+i))
			if msg.Content != expectedContent {
				t.Errorf("Mensaje %d: se esperaba '%s', obtuvo '%s'", i, expectedContent, msg.Content)
			}

		case <-time.After(1 * time.Second):
			t.Errorf("Timeout esperando mensaje %d", i)
		}
	}
}
