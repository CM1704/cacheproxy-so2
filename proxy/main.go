// CacheProxy — proxy HTTP con cache, concurrente y de alta disponibilidad.
// Proyecto de Investigacion, BISOF 18 Sistemas Operativos II.
//
// Semana 14: se incorporan el tunel CONNECT, la cache compartida de segundo
// nivel sobre Valkey, los controles de seguridad y el punto de comprobacion
// para el balanceador de carga.
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
	"github.com/CM1704/cacheproxy-so2/proxy/security"
)

var (
	conexiones int64
	l1         *cache.LRU
	l2         *cache.Compartida
	politica   *security.Politica
)

func main() {
	addr := getenv("PROXY_ADDR", ":8080")
	nodo := getenv("NODO", "proxy")
	capacidad := getenvInt("CACHE_MAX_ENTRADAS", 1000)
	ttl := time.Duration(getenvInt("CACHE_TTL_SEGUNDOS", 60)) * time.Second
	dirValkey := getenv("VALKEY_ADDR", "")
	maxBytes := int64(getenvInt("MAX_RESPUESTA_KB", 5120)) * 1024

	l1 = cache.NuevaLRU(capacidad, ttl)
	politica = security.NuevaPolitica(getenv("DOMINIOS_PERMITIDOS", ""), maxBytes)

	if dirValkey != "" {
		l2 = cache.NuevaCompartida(dirValkey, 2*time.Second)
		if err := l2.Ping(); err != nil {
			log.Printf("[%s] AVISO: cache compartida no disponible (%v); se opera solo con L1", nodo, err)
		} else {
			log.Printf("[%s] cache compartida conectada en %s", nodo, dirValkey)
		}
	}

	// --- Socket de escucha ---
	// net.Listen realiza socket(), bind() y listen() del sistema operativo.
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("[%s] no se pudo abrir el socket de escucha: %v", nodo, err)
	}
	log.Printf("[%s] escuchando en %s", nodo, addr)
	log.Printf("[%s] cache L1: capacidad=%d ttl=%v", nodo, capacidad, ttl)
	if politica.SinRestringir() {
		log.Printf("[%s] AVISO: sin lista blanca de dominios (proxy abierto)", nodo)
	} else {
		log.Printf("[%s] dominios permitidos: %v", nodo, politica.Dominios())
	}
	log.Printf("[%s] tamano maximo de respuesta: %d KB", nodo, maxBytes/1024)

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", salud)
	mux.HandleFunc("/stats", estadisticas)
	mux.HandleFunc("/", manejar(nodo, ttl))

	// El metodo CONNECT no puede pasar por ServeMux: en ese caso la URL trae
	// la autoridad y no una ruta, de modo que el mux responderia con una
	// redireccion. Se atiende antes de delegar en el enrutador.
	raiz := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodConnect {
			n := atomic.AddInt64(&conexiones, 1)
			if !politica.Permitido(r.Host) {
				politica.RegistrarBloqueo()
				log.Printf("[%s] #%d BLOQUEADO destino no autorizado: %s", nodo, n, r.Host)
				http.Error(w, "destino no autorizado por la politica del proxy",
					http.StatusForbidden)
				return
			}
			tunel(w, r, nodo, n)
			return
		}
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Handler:           raiz,
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

func manejar(nodo string, ttlPorOmision time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&conexiones, 1)

		// --- Control de destino ---
		destino := r.Host
		if r.URL.IsAbs() {
			destino = r.URL.Host
		}
		if !politica.Permitido(destino) {
			politica.RegistrarBloqueo()
			log.Printf("[%s] #%d BLOQUEADO destino no autorizado: %s", nodo, n, destino)
			http.Error(w, "destino no autorizado por la politica del proxy",
				http.StatusForbidden)
			return
		}

		if !r.URL.IsAbs() {
			http.Error(w, "se esperaba una peticion de proxy con URI absoluta",
				http.StatusBadRequest)
			return
		}

		inicio := time.Now()
		clave := cache.Clave(r.Method, r.URL.String())
		log.Printf("[%s] #%d %s %s", nodo, n, r.Method, r.URL)

		// RFC 9111, seccion 3.5: una cache compartida no debe reutilizar una
		// respuesta almacenada para satisfacer una peticion que lleva el campo
		// Authorization. La politica de almacenamiento impide guardarlas, pero
		// eso no basta: hay que impedir tambien que se sirvan desde la cache,
		// porque la entrada pudo haberla depositado una peticion anonima.
		autenticada := r.Header.Get("Authorization") != ""
		if autenticada {
			log.Printf("[%s] #%d peticion autenticada: se omite la cache", nodo, n)
		}

		// --- Nivel 1: cache local en memoria ---
		if ent, ok := l1.Obtener(clave); ok && !autenticada {
			responderDesdeCache(w, ent, nodo, "HIT-L1")
			log.Printf("[%s] #%d HIT-L1 %d bytes en %v",
				nodo, n, len(ent.Cuerpo), time.Since(inicio).Round(time.Microsecond))
			return
		}

		// --- Nivel 2: cache compartida ---
		if l2 != nil && !autenticada {
			if ent, ok := l2.Obtener(clave); ok {
				// Se promueve a L1 para que la siguiente consulta no salga del proceso.
				l1.Guardar(clave, ent.Estado, ent.Cabeceras, ent.Cuerpo, ttlPorOmision)
				responderDesdeCache(w, ent, nodo, "HIT-L2")
				log.Printf("[%s] #%d HIT-L2 %d bytes en %v",
					nodo, n, len(ent.Cuerpo), time.Since(inicio).Round(time.Microsecond))
				return
			}
		}

		// --- Fallo en ambos niveles: se acude al origen ---
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

		// Limite de tamano: se lee un byte de mas para detectar el exceso.
		limite := politica.MaxBytes()
		cuerpo, err := io.ReadAll(io.LimitReader(resp.Body, limite+1))
		if err != nil {
			log.Printf("[%s] #%d error al leer la respuesta: %v", nodo, n, err)
			http.Error(w, "respuesta incompleta del origen", http.StatusBadGateway)
			return
		}
		if int64(len(cuerpo)) > limite {
			politica.RegistrarExceso()
			log.Printf("[%s] #%d RECHAZADA: respuesta supera %d bytes", nodo, n, limite)
			http.Error(w, "la respuesta del origen supera el limite permitido",
				http.StatusBadGateway)
			return
		}

		// --- Decision de almacenamiento (RFC 9111) ---
		respParaPolitica := *resp
		respParaPolitica.Body = io.NopCloser(bytes.NewReader(cuerpo))
		d := cache.Almacenable(r, &respParaPolitica, ttlPorOmision)

		limpias := http.Header{}
		copiarCabeceras(limpias, resp.Header)

		if d.Almacenable {
			l1.Guardar(clave, resp.StatusCode, limpias.Clone(), cuerpo, d.TTL)
			if l2 != nil {
				if err := l2.Guardar(clave, resp.StatusCode, limpias.Clone(), cuerpo, d.TTL); err != nil {
					log.Printf("[%s] #%d aviso: no se pudo escribir en L2: %v", nodo, n, err)
				}
			}
			log.Printf("[%s] #%d almacenada (ttl=%v)", nodo, n, d.TTL)
		} else {
			log.Printf("[%s] #%d no almacenada: %s", nodo, n, d.Motivo)
		}

		for k, vs := range limpias {
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

func responderDesdeCache(w http.ResponseWriter, ent *cache.Entrada, nodo, marca string) {
	for k, vs := range ent.Cabeceras {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Age", strconv.Itoa(int(ent.Edad().Seconds())))
	w.Header().Set("X-Cache", marca)
	w.Header().Set("X-Proxy-Nodo", nodo)
	w.WriteHeader(ent.Estado)
	w.Write(ent.Cuerpo)
}

// salud responde a la comprobacion periodica del balanceador de carga.
func salud(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "ok\n")
}

// estadisticas expone los contadores para los experimentos.
func estadisticas(w http.ResponseWriter, r *http.Request) {
	s := l1.Estadisticas()
	bloq, exc := politica.Contadores()

	var a2, f2, e2 int64
	if l2 != nil {
		s2 := l2.Estadisticas()
		a2, f2, e2 = s2.Aciertos, s2.Fallos, s2.Errores
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	io.WriteString(w, "{"+
		"\"nodo\":\""+getenv("NODO", "proxy")+"\","+
		"\"l1_aciertos\":"+strconv.FormatInt(s.Aciertos, 10)+","+
		"\"l1_fallos\":"+strconv.FormatInt(s.Fallos, 10)+","+
		"\"l1_desalojos\":"+strconv.FormatInt(s.Desalojos, 10)+","+
		"\"l1_vencidas\":"+strconv.FormatInt(s.Vencidas, 10)+","+
		"\"l1_entradas\":"+strconv.Itoa(s.Entradas)+","+
		"\"l1_capacidad\":"+strconv.Itoa(s.Capacidad)+","+
		"\"l1_tasa_aciertos\":"+strconv.FormatFloat(s.TasaAciertos(), 'f', 4, 64)+","+
		"\"l2_aciertos\":"+strconv.FormatInt(a2, 10)+","+
		"\"l2_fallos\":"+strconv.FormatInt(f2, 10)+","+
		"\"l2_errores\":"+strconv.FormatInt(e2, 10)+","+
		"\"seg_bloqueados\":"+strconv.FormatInt(bloq, 10)+","+
		"\"seg_excedidos\":"+strconv.FormatInt(exc, 10)+
		"}\n")
}

// saltoASalto son las cabeceras cuya validez se limita a una unica conexion.
// Un intermediario debe eliminarlas antes de reenviar el mensaje (RFC 9110).
var saltoASalto = map[string]bool{
	"Connection": true, "Proxy-Connection": true, "Keep-Alive": true,
	"Proxy-Authenticate": true, "Proxy-Authorization": true, "Te": true,
	"Trailer": true, "Transfer-Encoding": true, "Upgrade": true,
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
