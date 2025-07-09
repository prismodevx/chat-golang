# Chat en Tiempo Real con WebSockets

Una aplicación de chat en tiempo real implementada en Go utilizando WebSockets, diseñada para manejar múltiples clientes concurrentes de manera eficiente y segura.

## Características

- **Comunicación en tiempo real** mediante WebSockets
- **Manejo concurrente** de múltiples clientes
- **Mensajes del sistema** para conexiones/desconexiones
- **Interface web** intuitiva y responsive
- **Pruebas unitarias** completas con detección de race conditions
- **Arquitectura modular** y escalable

## Arquitectura del Sistema

### Componentes Principales

1. **Hub (`hub.go`)**: Núcleo central que gestiona todos los clientes y la difusión de mensajes
2. **Client (`client.go`)**: Representa una conexión WebSocket individual
3. **Main (`main.go`)**: Servidor HTTP que maneja las conexiones WebSocket
4. **Frontend (`static/index.html`)**: Cliente web con JavaScript

### Flujo de Datos

```
Cliente Web → WebSocket → Client.readPump() → Hub.broadcast → Hub.broadcastMessage() → Client.writePump() → Cliente Web
```

### Concurrencia y Seguridad

#### Goroutines por Cliente
Cada cliente maneja **dos goroutines**:
- **readPump()**: Lee mensajes del cliente y los envía al hub
- **writePump()**: Escucha el canal `send` y envía mensajes al cliente

#### Hub Central
El hub ejecuta una **goroutine principal** que maneja:
- Registro de nuevos clientes
- Desregistro de clientes desconectados
- Difusión de mensajes a todos los clientes

#### Protección Concurrente
- **sync.RWMutex**: Protege el mapa de clientes conectados
- **Canales buffered**: Comunicación segura entre goroutines
- **Lectura con copia**: Evita race conditions durante la difusión

```go
// Ejemplo de protección concurrente
h.mutex.RLock()
clientsCopy := make(map[*Client]bool, len(h.clients))
for client := range h.clients {
    clientsCopy[client] = true
}
h.mutex.RUnlock()
```

## Gestión de Conexiones

### Registro de Clientes
1. Cliente se conecta via WebSocket
2. Se crea estructura `Client` con canales de comunicación
3. Se registra en el hub via canal `register`
4. Se inician goroutines de lectura/escritura
5. Se envía mensaje del sistema de conexión

### Desconexión de Clientes
1. Detección automática de desconexión (ping/pong, errores de red)
2. Limpieza de recursos via `defer`
3. Desregistro del hub via canal `unregister`
4. Cierre del canal `send`
5. Mensaje del sistema de desconexión

### Manejo de Errores
- **Timeouts**: Configurables para lectura/escritura
- **Ping/Pong**: Detección de conexiones muertas
- **Graceful shutdown**: Limpieza automática de recursos

## Canales de Comunicación

### Diseño de Canales

| Canal | Tipo | Tamaño | Propósito |
|-------|------|--------|-----------|
| `hub.register` | `chan *Client` | Sin buffer | Registro de clientes |
| `hub.unregister` | `chan *Client` | Sin buffer | Desregistro de clientes |
| `hub.broadcast` | `chan Message` | Sin buffer | Difusión de mensajes |
| `client.send` | `chan []byte` | 256 | Cola de mensajes por cliente |

### Justificación del Diseño

- **Canales sin buffer** para operaciones críticas (registro/desregistro) garantizan procesamiento inmediato
- **Canal buffered** para `client.send` permite manejar ráfagas de mensajes sin bloquear
- **Comunicación unidireccional** simplifica la lógica y evita deadlocks

## Elección de Paquete WebSocket

### Gorilla WebSocket vs net/websocket

**Elegimos `github.com/gorilla/websocket`** por:

1. **Mejor rendimiento**: Optimizado para aplicaciones de alta concurrencia
2. **Más características**: Soporte completo para extensiones WebSocket
3. **Mejor manejo de errores**: Detección robusta de conexiones cerradas
4. **Ping/Pong automático**: Detección de conexiones muertas
5. **Comunidad activa**: Mantenimiento regular y documentación extensa

```go
// Configuración del upgrader
var upgrader = websocket.Upgrader{
    ReadBufferSize:  1024,
    WriteBufferSize: 1024,
    CheckOrigin: func(r *http.Request) bool {
        return true // Configurar según necesidades de seguridad
    },
}
```

## Instalación y Uso

### Requisitos
- Go 1.21 o superior
- Dependencias: `github.com/gorilla/websocket`

### Instalación

```bash
# Clonar el repositorio
git clone <repo-url>
cd websocket-chat

# Instalar dependencias
go mod tidy

# Ejecutar el servidor
go run .
```

### Uso

1. Abrir navegador en `http://localhost:8080`
2. Ingresar nombre de usuario
3. Comenzar a chatear en tiempo real

## Pruebas

### Ejecutar Pruebas

```bash
# Pruebas unitarias
go test -v

# Pruebas con detección de race conditions
go test -race -v

# Benchmarks
go test -bench=. -v
```

### Tipos de Pruebas

- **Registro/Desregistro**: Verificación de gestión de clientes
- **Difusión de mensajes**: Pruebas de broadcast concurrente
- **Acceso concurrente**: Verificación de seguridad thread-safe
- **Mensajes del sistema**: Validación de notificaciones automáticas
- **Orden de mensajes**: Garantía de entrega secuencial
- **Benchmarks**: Medición de rendimiento bajo carga

## Estructura del Proyecto

```
websocket-chat/
├── main.go           # Servidor HTTP principal
├── client.go         # Gestión de clientes WebSocket
├── hub.go           # Hub central de chat
├── hub_test.go      # Pruebas unitarias
├── go.mod           # Dependencias
├── README.md        # Documentación
└── static/
    └── index.html   # Cliente web
```

## Escalabilidad

### Optimizaciones Implementadas

1. **Goroutines eficientes**: Una por cliente para lectura/escritura
2. **Canales buffered**: Evitan bloqueos en picos de tráfico
3. **Mutex readers/writers**: Optimizan acceso de lectura concurrente
4. **Ping/Pong**: Limpieza automática de conexiones muertas
5. **Copia de mapas**: Evita bloqueos durante difusión

### Consideraciones de Producción

- **Load balancing**: Usar múltiples instancias con sticky sessions
- **Persistencia**: Agregar base de datos para historial de mensajes
- **Autenticación**: Implementar sistema de usuarios
- **Rate limiting**: Prevenir spam y abuso
- **Monitoring**: Métricas de conexiones y rendimiento

## Configuración Avanzada

### Variables de Entorno

```bash
# Puerto del servidor
export PORT=8080

# Timeouts de WebSocket
export WRITE_WAIT=10s
export PONG_WAIT=60s
export PING_PERIOD=54s

# Tamaño de buffers
export READ_BUFFER_SIZE=1024
export WRITE_BUFFER_SIZE=1024
```

### Personalización

- Modificar `maxMessageSize` para mensajes más largos
- Ajustar tamaño de canal `send` según carga esperada
- Configurar timeouts según latencia de red
- Personalizar mensajes del sistema

## Troubleshooting

### Problemas Comunes

1. **Conexiones no se cierran**: Verificar implementación de ping/pong
2. **Race conditions**: Ejecutar con `go test -race`
3. **Memory leaks**: Verificar cierre de canales y goroutines
4. **Mensajes perdidos**: Revisar tamaño de buffer del canal `send`

### Debugging

```bash
# Habilitar logs detallados
export DEBUG=true

# Profiling de memoria
go tool pprof http://localhost:8080/debug/pprof/heap

# Profiling de CPU
go tool pprof http://localhost:8080/debug/pprof/profile
```

## Contribución

1. Fork del repositorio
2. Crear branch para feature (`git checkout -b feature/nueva-funcionalidad`)
3. Commit de cambios (`git commit -am 'Agregar nueva funcionalidad'`)
4. Push al branch (`git push origin feature/nueva-funcionalidad`)
5. Crear Pull Request

## Licencia

Este proyecto está licenciado bajo la MIT License - ver archivo LICENSE para detalles.