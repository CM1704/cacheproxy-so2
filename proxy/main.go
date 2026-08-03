// CacheProxy — proxy HTTP con cache, concurrente y de alta disponibilidad.
// Proyecto de Investigacion, BISOF 18 Sistemas Operativos II.
//
// Semana 12: version minima funcional. Acepta conexiones en un socket TCP
// propio, reenvia la peticion al servidor de origen sobre otro socket TCP y
// devuelve la respuesta al cliente. Todavia sin cache (Semana 13).
package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"
)

// conexiones cuenta las conexiones aceptadas desde el arranque.
// Se usa atomic porque varias goroutines la incrementan a la vez.
var conexiones int64

func main() {
	addr := getenv("PROXY_ADDR", ":8080")
	nodo := getenv("NODO", "proxy")

	// --- Socket de escucha ---
	// net.Listen realiza socket(), bind() y listen() del sistema operativo.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[%s] no se pudo abrir el socket de escucha: %v", nodo, err)
	}
	log.Printf("[%s] escuchando en %s", nodo, addr)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", salud)  // comprobacion para HAProxy (Semana 14)
	mux.HandleFunc("/", manejar(nodo)) // todo lo demas se reenvia

	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	// Apagado ordenado: permite que las peticiones en curso terminen.
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		log.Printf("[%s] señal recibida, cerrando", nodo)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		srv.Shutdown(ctx)
	}()

	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		log.Fatalf("[%s] error del servidor: %v", nodo, err)
	}
	log.Printf("[%s] detenido", nodo)
}

// transporte usa un marcador propio para las conexiones salientes, de modo
// que el establecimiento del socket hacia el origen sea explicito y trazable.
var transporte = &http.Transport{
	DialContext: func(ctx context.Context, red, direccion string) (net.Conn, error) {
		d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
		// net.Dial realiza socket() y connect() hacia el servidor de origen.
		c, err := d.DialContext(ctx, red, direccion)
		if err != nil {
			log.Printf("  dial %s %s -> ERROR: %v", red, direccion, err)
			return nil, err
		}
		log.Printf("  dial %s %s -> %s", red, direccion, c.RemoteAddr())
		return c, nil
	},
	MaxIdleConns:        100,
	IdleConnTimeout:     90 * time.Second,
	TLSHandshakeTimeout: 10 * time.Second,
}

// manejar devuelve el manejador principal. Cada peticion entrante se atiende
// en su propia goroutine, creada por el servidor HTTP de la biblioteca.
func manejar(nodo string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&conexiones, 1)

		// El metodo CONNECT abre un tunel. Se implementa en la Semana 14.
		if r.Method == http.MethodConnect {
			http.Error(w, "CONNECT no implementado todavia", http.StatusNotImplemented)
			return
		}

		// Un proxy directo recibe la peticion con URI absoluta. Si llega
		// relativa, el cliente no nos esta usando como proxy.
		if !r.URL.IsAbs() {
			http.Error(w, "se esperaba una peticion de proxy con URI absoluta",
				http.StatusBadRequest)
			return
		}

		inicio := time.Now()
		log.Printf("[%s] #%d %s %s", nodo, n, r.Method, r.URL)

		salida, err := http.NewRequestWithContext(
			r.Context(), r.Method, r.URL.String(), r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		copiarCabeceras(salida.Header, r.Header)

		resp, err := transporte.RoundTrip(salida)
		if err != nil {
			log.Printf("[%s] #%d error hacia el origen: %v", nodo, n, err)
			http.Error(w, "error al contactar el servidor de origen: "+err.Error(),
				http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()

		copiarCabeceras(w.Header(), resp.Header)
		w.Header().Set("X-Cache", "MISS") // sin cache todavia: siempre MISS
		w.Header().Set("X-Proxy-Nodo", nodo)
		w.WriteHeader(resp.StatusCode)

		bytes, _ := io.Copy(w, resp.Body)
		log.Printf("[%s] #%d %d %d bytes en %v",
			nodo, n, resp.StatusCode, bytes, time.Since(inicio).Round(time.Millisecond))
	}
}

// salud responde a la comprobacion periodica del balanceador de carga.
func salud(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}

// saltoASalto son las cabeceras cuya validez se limita a una unica conexion.
// Un intermediario debe eliminarlas antes de reenviar el mensaje (RFC 9110).
var saltoASalto = map[string]bool{
	"Connection":          true,
	"Proxy-Connection":    true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func copiarCabeceras(destino, origen http.Header) {
	for clave, valores := range origen {
		if saltoASalto[http.CanonicalHeaderKey(clave)] {
			continue
		}
		for _, v := range valores {
			destino.Add(clave, v)
		}
	}
}

func getenv(clave, porDefecto string) string {
	if v := os.Getenv(clave); v != "" {
		return v
	}
	return porDefecto
}
