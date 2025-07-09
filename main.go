package main

import (
	"log"
	"net/http"
)

func main() {
	// Crear el hub de chat
	hub := NewHub()

	// Iniciar el hub en una goroutine separada
	go hub.Run()

	// Configurar rutas
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		ServeWS(hub, w, r)
	})

	// Servir archivos estáticos
	http.Handle("/", http.FileServer(http.Dir("./static/")))

	log.Println("Servidor de chat iniciado en :8080")
	log.Println("Accede a http://localhost:8080 para usar el chat")

	// Iniciar el servidor HTTP
	if err := http.ListenAndServe(":8080", nil); err != nil {
		log.Fatal("Error al iniciar el servidor:", err)
	}
}
