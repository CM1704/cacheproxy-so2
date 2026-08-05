// CacheProxy — proxy HTTP con cache, concurrente y de alta disponibilidad.
// Proyecto de Investigacion, BISOF 18 Sistemas Operativos II.
//
// Semana 13: se incorpora la cache en memoria con reemplazo por uso menos
// reciente y expiracion por tiempo de vida. Antes de salir al origen, el
// proxy consulta el almacen; si la respuesta esta presente y sigue fresca,
// la devuelve sin abrir conexion saliente.
package main

import (
	"bytes"
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/CM1704/cacheproxy-so2/proxy/cache"
)

// conexiones cuenta las conexiones aceptadas desde el arranque.
// Se usa atomic porque varias goroutines la incrementan a la vez.
var (
	conexiones int64
	almacen    *cache.LRU
)

func main() {
	addr := getenv("PROXY_ADDR", ":8080")
	nodo := getenv("NODO", "proxy")
	capacidad := getenvInt("CACHE_MAX_ENTRADAS", 1000)
	ttl := time.Duration(getenvInt("CACHE_TTL_SEGUNDOS", 60)) * time.Second

	almacen = cache.NuevaLRU(capacidad, ttl)

	// --- Socket de escucha ---
	// net.Listen realiza socket(), bind() y listen() del sistema operativo.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[%s] no se pudo abrir el socket de escucha: %v", nodo, err)
	}
	log.Printf("[%s] escuchando en %s", nodo, addr)
	log.Printf("[%s] cache: capacidad=%d ttl=%v", nodo, capacidad, ttl)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", salud)       // comprobacion para HAProxy
	mux.HandleFunc("/stats", estadisticas)  // diagnostico de la cache
	mux.HandleFunc("/", manejar(nodo, ttl)) // todo lo demas se reenvia

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
func manejar(nodo string, ttlPorOmision time.Duration) http.HandlerFunc {
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
		clave := cache.Clave(r.Method, r.URL.String())
		log.Printf("[%s] #%d %s %s", nodo, n, r.Method, r.URL)

		// --- Consulta a la cache ---
		if ent, ok := almacen.Obtener(clave); ok {
			for k, vs := range ent.Cabeceras {
				for _, v := range vs {
					w.Header().Add(k, v)
				}
			}
			edad := int(ent.Edad().Seconds())
			w.Header().Set("Age", strconv.Itoa(edad))
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("X-Proxy-Nodo", nodo)
			w.WriteHeader(ent.Estado)
			w.Write(ent.Cuerpo)

			log.Printf("[%s] #%d HIT %d %d bytes edad=%ds en %v",
				nodo, n, ent.Estado, len(ent.Cuerpo), edad,
				time.Since(inicio).Round(time.Microsecond))
			return
		}

		// --- Fallo: se acude al servidor de origen ---
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

		cuerpo, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("[%s] #%d error al leer la respuesta: %v", nodo, n, err)
			http.Error(w, "respuesta incompleta del origen", http.StatusBadGateway)
			return
		}

		// --- Decision de almacenamiento (RFC 9111) ---
		respParaPolitica := *resp
		respParaPolitica.Body = io.NopCloser(bytes.NewReader(cuerpo))
		d := cache.Almacenable(r, &respParaPolitica, ttlPorOmision)

		cabecerasLimpias := http.Header{}
		copiarCabeceras(cabecerasLimpias, resp.Header)

		if d.Almacenable {
			almacen.Guardar(clave, resp.StatusCode, cabecerasLimpias.Clone(), cuerpo, d.TTL)
			log.Printf("[%s] #%d almacenada (ttl=%v)", nodo, n, d.TTL)
		} else {
			log.Printf("[%s] #%d no almacenada: %s", nodo, n, d.Motivo)
		}

		for k, vs := range cabecerasLimpias {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("X-Cache", "MISS")
		w.Header().Set("X-Proxy-Nodo", nodo)
		w.WriteHeader(resp.StatusCode)
		w.Write(cuerpo)

		log.Printf("[%s] #%d MISS %d %d bytes en %v",
			nodo, n, resp.StatusCode, len(cuerpo),
			time.Since(inicio).Round(time.Millisecond))
	}
}

// salud responde a la comprobacion periodica del balanceador de carga.
func salud(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}

// estadisticas expone los contadores de la cache para los experimentos.
func estadisticas(w http.ResponseWriter, r *http.Request) {
	s := almacen.Estadisticas()
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "{"+
		"\"aciertos\":"+strconv.FormatInt(s.Aciertos, 10)+","+
		"\"fallos\":"+strconv.FormatInt(s.Fallos, 10)+","+
		"\"desalojos\":"+strconv.FormatInt(s.Desalojos, 10)+","+
		"\"vencidas\":"+strconv.FormatInt(s.Vencidas, 10)+","+
		"\"entradas\":"+strconv.Itoa(s.Entradas)+","+
		"\"capacidad\":"+strconv.Itoa(s.Capacidad)+","+
		"\"tasa_aciertos\":"+strconv.FormatFloat(s.TasaAciertos(), 'f', 4, 64)+
		"}\n")
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

func getenvInt(clave string, porDefecto int) int {
	if v := os.Getenv(clave); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return porDefecto
}
