package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// MockClient simula un cliente para pruebas
type MockClient struct {
	hub      *Hub
	send     chan []byte
	username string
	closed   bool
	mutex    sync.RWMutex
}

// NewMockClient crea un cliente simulado
func NewMockClient(hub *Hub, username string) *MockClient {
	return &MockClient{
		hub:      hub,
		send:     make(chan []byte, 256),
		username: username,
		closed:   false,
	}
}

// Close simula el cierre del cliente
func (mc *MockClient) Close() {
	mc.mutex.Lock()
	defer mc.mutex.Unlock()
	if !mc.closed {
		close(mc.send)
		mc.closed = true
	}
}

// IsClosed verifica si el cliente está cerrado
func (mc *MockClient) IsClosed() bool {
	mc.mutex.RLock()
	defer mc.mutex.RUnlock()
	return mc.closed
}

// ConvertToClient convierte MockClient a Client para las pruebas
func (mc *MockClient) ConvertToClient() *Client {
	return &Client{
		hub:      mc.hub,
		conn:     nil, // No necesitamos conexión real para las pruebas
		send:     mc.send,
		username: mc.username,
	}
}

// DrainMessages lee todos los mensajes pendientes del canal
func (mc *MockClient) DrainMessages() []Message {
	var messages []Message
	for {
		select {
		case msgBytes := <-mc.send:
			var msg Message
			if err := json.Unmarshal(msgBytes, &msg); err == nil {
				messages = append(messages, msg)
			}
		default:
			return messages
		}
	}
}

// WaitForMessage espera por un mensaje específico o timeout
func (mc *MockClient) WaitForMessage(timeout time.Duration, messageType MessageType) (*Message, error) {
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	for {
		select {
		case msgBytes := <-mc.send:
			var msg Message
			if err := json.Unmarshal(msgBytes, &msg); err != nil {
				continue
			}
			if msg.Type == messageType {
				return &msg, nil
			}
		case <-timer.C:
			return nil, fmt.Errorf("timeout waiting for message type %s", messageType)
		}
	}
}

// TestNewHub verifica la creación del hub
func TestNewHub(t *testing.T) {
	hub := NewHub()

	if hub == nil {
		t.Fatal("NewHub() debería retornar una instancia válida del hub")
	}

	if hub.clients == nil {
		t.Error("El mapa de clientes no debería ser nil")
	}

	if hub.broadcast == nil {
		t.Error("El canal broadcast no debería ser nil")
	}

	if hub.register == nil {
		t.Error("El canal register no debería ser nil")
	}

	if hub.unregister == nil {
		t.Error("El canal unregister no debería ser nil")
	}
}

// TestClientRegistration verifica el registro de clientes
func TestClientRegistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		// Dar tiempo para que se procesen los mensajes
		time.Sleep(100 * time.Millisecond)
	}()

	mockClient := NewMockClient(hub, "testuser1")
	client := mockClient.ConvertToClient()

	// Registrar cliente
	hub.register <- client

	// Esperar a que se procese el registro
	time.Sleep(50 * time.Millisecond)

	count := hub.GetClientCount()
	if count != 1 {
		t.Errorf("Se esperaba 1 cliente, se obtuvo %d", count)
	}

	clients := hub.GetClients()
	if len(clients) != 1 {
		t.Errorf("Se esperaba 1 cliente en el mapa, se obtuvo %d", len(clients))
	}

	// Verificar que el cliente esté en el mapa
	if _, exists := clients[client]; !exists {
		t.Error("El cliente no está registrado en el hub")
	}
}

// TestClientUnregistration verifica el desregistro de clientes
func TestClientUnregistration(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	mockClient := NewMockClient(hub, "testuser2")
	client := mockClient.ConvertToClient()

	// Registrar cliente
	hub.register <- client
	time.Sleep(50 * time.Millisecond)

	// Limpiar mensajes del sistema de registro
	mockClient.DrainMessages()

	// Verificar registro
	if hub.GetClientCount() != 1 {
		t.Error("El cliente no se registró correctamente")
	}

	// Desregistrar cliente
	hub.unregister <- client
	time.Sleep(50 * time.Millisecond)

	// Verificar desregistro
	count := hub.GetClientCount()
	if count != 0 {
		t.Errorf("Se esperaban 0 clientes después del desregistro, se obtuvo %d", count)
	}

	// El canal send se cierra en unregisterClient, pero nuestro MockClient
	// maneja esto de forma diferente. Verificamos que no haya más clientes activos.
	clients := hub.GetClients()
	if len(clients) != 0 {
		t.Error("El cliente no se eliminó correctamente del mapa de clientes")
	}
}

// TestMessageBroadcast verifica la difusión de mensajes
func TestMessageBroadcast(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Crear múltiples clientes simulados
	clients := make([]*MockClient, 3)
	realClients := make([]*Client, 3)

	for i := 0; i < 3; i++ {
		clients[i] = NewMockClient(hub, "testuser"+string(rune('1'+i)))
		realClients[i] = clients[i].ConvertToClient()
		hub.register <- realClients[i]
	}

	time.Sleep(50 * time.Millisecond)

	// Limpiar mensajes del sistema de los clientes
	for _, client := range clients {
		client.DrainMessages()
	}

	// Crear mensaje de prueba
	testMessage := Message{
		Type:      MessageTypeChat,
		Username:  "testuser1",
		Content:   "Hola mundo",
		Timestamp: time.Now(),
	}

	// Enviar mensaje
	hub.broadcast <- testMessage
	time.Sleep(50 * time.Millisecond)

	// Verificar que todos los clientes recibieron el mensaje
	for i, client := range clients {
		chatMsg, err := client.WaitForMessage(1*time.Second, MessageTypeChat)
		if err != nil {
			t.Errorf("Cliente %d no recibió mensaje de chat: %v", i, err)
			continue
		}

		if chatMsg.Content != testMessage.Content {
			t.Errorf("Cliente %d: contenido esperado '%s', obtuvo '%s'",
				i, testMessage.Content, chatMsg.Content)
		}
	}
}

// TestSystemMessages verifica los mensajes del sistema
func TestSystemMessages(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	// Crear un cliente observador que recibirá los mensajes del sistema
	observerClient := NewMockClient(hub, "observer")
	observerRealClient := observerClient.ConvertToClient()
	hub.register <- observerRealClient
	time.Sleep(50 * time.Millisecond)

	// Leer y descartar el mensaje de conexión del observador
	observerClient.WaitForMessage(1*time.Second, MessageTypeSystem)

	// Registrar un nuevo cliente
	newClient := NewMockClient(hub, "newuser")
	newRealClient := newClient.ConvertToClient()
	hub.register <- newRealClient
	time.Sleep(50 * time.Millisecond)

	// Verificar mensaje de conexión del nuevo usuario
	systemMsg, err := observerClient.WaitForMessage(1*time.Second, MessageTypeSystem)
	if err != nil {
		t.Fatalf("No se recibió mensaje de sistema de conexión: %v", err)
	}

	if !strings.Contains(systemMsg.Content, "se ha conectado") {
		t.Errorf("Contenido del mensaje no contiene 'se ha conectado': %s", systemMsg.Content)
	}

	if !strings.Contains(systemMsg.Content, "newuser") {
		t.Errorf("Contenido del mensaje no contiene 'newuser': %s", systemMsg.Content)
	}

	// Desregistrar el nuevo cliente
	hub.unregister <- newRealClient
	time.Sleep(50 * time.Millisecond)

	// Verificar mensaje de desconexión
	systemMsg, err = observerClient.WaitForMessage(1*time.Second, MessageTypeSystem)
	if err != nil {
		t.Fatalf("No se recibió mensaje de sistema de desconexión: %v", err)
	}

	if !strings.Contains(systemMsg.Content, "se ha desconectado") {
		t.Errorf("Contenido del mensaje no contiene 'se ha desconectado': %s", systemMsg.Content)
	}

	if !strings.Contains(systemMsg.Content, "newuser") {
		t.Errorf("Contenido del mensaje no contiene 'newuser': %s", systemMsg.Content)
	}
}

// TestConcurrentClientOperations verifica operaciones concurrentes de clientes
func TestConcurrentClientOperations(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(200 * time.Millisecond)
	}()

	const numClients = 50
	const numMessages = 20

	var wg sync.WaitGroup
	clients := make([]*MockClient, numClients)
	realClients := make([]*Client, numClients)

	// Registrar clientes concurrentemente
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(index int) {
			defer wg.Done()
			clients[index] = NewMockClient(hub, "user"+string(rune('0'+index%10)))
			realClients[index] = clients[index].ConvertToClient()
			hub.register <- realClients[index]
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Verificar que todos los clientes se registraron
	if count := hub.GetClientCount(); count != numClients {
		t.Errorf("Se esperaban %d clientes, se obtuvo %d", numClients, count)
	}

	// Enviar mensajes concurrentemente
	wg.Add(numMessages)
	for i := 0; i < numMessages; i++ {
		go func(msgIndex int) {
			defer wg.Done()
			message := Message{
				Type:      MessageTypeChat,
				Username:  "testuser",
				Content:   "Mensaje concurrente " + string(rune('0'+msgIndex%10)),
				Timestamp: time.Now(),
			}
			hub.broadcast <- message
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Desregistrar clientes concurrentemente
	wg.Add(numClients)
	for i := 0; i < numClients; i++ {
		go func(index int) {
			defer wg.Done()
			hub.unregister <- realClients[index]
		}(i)
	}

	wg.Wait()
	time.Sleep(100 * time.Millisecond)

	// Verificar que todos los clientes se desregistraron
	if count := hub.GetClientCount(); count != 0 {
		t.Errorf("Se esperaban 0 clientes después del desregistro, se obtuvo %d", count)
	}
}

// TestRaceConditions verifica la ausencia de condiciones de carrera bajo carga
func TestRaceConditions(t *testing.T) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(200 * time.Millisecond)
	}()

	const numGoroutines = 100
	const opsPerGoroutine = 50

	var wg sync.WaitGroup

	// Ejecutar operaciones concurrentes intensivas
	wg.Add(numGoroutines * 3) // 3 tipos de operaciones

	// Goroutines para registro/desregistro de clientes
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				client := &Client{
					hub:      hub,
					send:     make(chan []byte, 256),
					username: "raceuser" + string(rune('0'+index%10)),
				}
				hub.register <- client
				hub.unregister <- client
			}
		}(i)
	}

	// Goroutines para envío de mensajes
	for i := 0; i < numGoroutines; i++ {
		go func(index int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				message := Message{
					Type:      MessageTypeChat,
					Username:  "raceuser" + string(rune('0'+index%10)),
					Content:   "Mensaje de carrera",
					Timestamp: time.Now(),
				}
				hub.broadcast <- message
			}
		}(i)
	}

	// Goroutines para lectura del estado del hub
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				_ = hub.GetClientCount()
				_ = hub.GetClients()
			}
		}()
	}

	wg.Wait()

	// El test pasa si no hay panic por condiciones de carrera
	t.Log("Test de condiciones de carrera completado exitosamente")
}

// TestWebSocketIntegration prueba la integración completa con WebSocket real
func TestWebSocketIntegration(t *testing.T) {
	// Crear hub y servidor de prueba
	hub := NewHub()
	go hub.Run()

	// Crear servidor HTTP de prueba
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ServeWS(hub, w, r)
	}))
	defer server.Close()

	// Convertir HTTP URL a WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "?username=integrationtest"

	// Conectar cliente WebSocket real
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Error al conectar WebSocket: %v", err)
	}
	defer conn.Close()

	// Esperar un momento para el registro
	time.Sleep(100 * time.Millisecond)

	// Verificar que el cliente se registró
	if count := hub.GetClientCount(); count != 1 {
		t.Errorf("Se esperaba 1 cliente conectado, se obtuvo %d", count)
	}

	// Leer el mensaje de sistema de conexión primero
	var systemMsg Message
	if err := conn.ReadJSON(&systemMsg); err != nil {
		t.Fatalf("Error al leer mensaje de sistema: %v", err)
	}

	if systemMsg.Type != MessageTypeSystem {
		t.Errorf("Se esperaba mensaje de sistema, obtuvo tipo '%s'", systemMsg.Type)
	}

	// Enviar mensaje de chat
	testMsg := Message{
		Type:    MessageTypeChat,
		Content: "Hola desde prueba de integración",
	}

	if err := conn.WriteJSON(testMsg); err != nil {
		t.Fatalf("Error al enviar mensaje: %v", err)
	}

	// Leer respuesta (nuestro propio mensaje que se reenvía)
	var receivedMsg Message
	if err := conn.ReadJSON(&receivedMsg); err != nil {
		t.Fatalf("Error al leer mensaje: %v", err)
	}

	// Verificar mensaje recibido
	if receivedMsg.Content != testMsg.Content {
		t.Errorf("Contenido esperado '%s', obtuvo '%s'", testMsg.Content, receivedMsg.Content)
	}

	if receivedMsg.Username != "integrationtest" {
		t.Errorf("Username esperado 'integrationtest', obtuvo '%s'", receivedMsg.Username)
	}

	if receivedMsg.Type != MessageTypeChat {
		t.Errorf("Tipo esperado '%s', obtuvo '%s'", MessageTypeChat, receivedMsg.Type)
	}
}

// TestMessageValidation verifica la validación de mensajes
func TestMessageValidation(t *testing.T) {
	// Probar serialización/deserialización de mensajes
	originalMsg := Message{
		Type:      MessageTypeChat,
		Username:  "testuser",
		Content:   "Mensaje de prueba con caracteres especiales: áéíóú ñ 🎉",
		Timestamp: time.Now(),
	}

	// Serializar
	data, err := json.Marshal(originalMsg)
	if err != nil {
		t.Fatalf("Error al serializar mensaje: %v", err)
	}

	// Deserializar
	var deserializedMsg Message
	if err := json.Unmarshal(data, &deserializedMsg); err != nil {
		t.Fatalf("Error al deserializar mensaje: %v", err)
	}

	// Comparar
	if deserializedMsg.Type != originalMsg.Type {
		t.Errorf("Tipo esperado '%s', obtuvo '%s'", originalMsg.Type, deserializedMsg.Type)
	}

	if deserializedMsg.Username != originalMsg.Username {
		t.Errorf("Username esperado '%s', obtuvo '%s'", originalMsg.Username, deserializedMsg.Username)
	}

	if deserializedMsg.Content != originalMsg.Content {
		t.Errorf("Contenido esperado '%s', obtuvo '%s'", originalMsg.Content, deserializedMsg.Content)
	}
}

// BenchmarkHubOperations realiza benchmarking de operaciones del hub
func BenchmarkHubOperations(b *testing.B) {
	hub := NewHub()
	go hub.Run()
	defer func() {
		time.Sleep(100 * time.Millisecond)
	}()

	b.ResetTimer()

	b.Run("ClientRegistration", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			client := &Client{
				hub:      hub,
				send:     make(chan []byte, 256),
				username: "benchuser",
			}
			hub.register <- client
		}
	})

	b.Run("MessageBroadcast", func(b *testing.B) {
		// Registrar algunos clientes para el benchmark
		for i := 0; i < 10; i++ {
			client := &Client{
				hub:      hub,
				send:     make(chan []byte, 256),
				username: "benchuser",
			}
			hub.register <- client
		}

		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			message := Message{
				Type:      MessageTypeChat,
				Username:  "benchuser",
				Content:   "Mensaje de benchmark",
				Timestamp: time.Now(),
			}
			hub.broadcast <- message
		}
	})
}

// TestEdgeCases prueba casos límite
func TestEdgeCases(t *testing.T) {
	t.Run("UnregisterNonExistentClient", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()
		defer func() {
			time.Sleep(50 * time.Millisecond)
		}()

		// Intentar desregistrar un cliente que nunca se registró
		client := &Client{
			hub:      hub,
			send:     make(chan []byte, 256),
			username: "nonexistent",
		}

		hub.unregister <- client
		time.Sleep(50 * time.Millisecond)

		// No debería causar panic o error
		if count := hub.GetClientCount(); count != 0 {
			t.Errorf("Se esperaban 0 clientes, se obtuvo %d", count)
		}
	})

	t.Run("EmptyMessage", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()
		defer func() {
			time.Sleep(50 * time.Millisecond)
		}()

		// Enviar mensaje vacío
		emptyMsg := Message{}
		hub.broadcast <- emptyMsg

		time.Sleep(50 * time.Millisecond)
		// No debería causar panic
	})

	t.Run("LongUsername", func(t *testing.T) {
		hub := NewHub()
		go hub.Run()
		defer func() {
			time.Sleep(50 * time.Millisecond)
		}()

		longUsername := strings.Repeat("a", 1000)
		client := &Client{
			hub:      hub,
			send:     make(chan []byte, 256),
			username: longUsername,
		}

		hub.register <- client
		time.Sleep(50 * time.Millisecond)

		if count := hub.GetClientCount(); count != 1 {
			t.Errorf("Se esperaba 1 cliente, se obtuvo %d", count)
		}
	})
}
