package main

// Tunel para el metodo CONNECT.
//
// Este es el unico punto del proxy que opera por debajo de la semantica de
// HTTP. Se toma el control del socket del cliente, se abre un socket hacia
// el destino y se retransmiten bytes en ambos sentidos sin interpretarlos.
// No se usa net/http para el transporte: solo net.Dial e io.Copy.

import (
	"io"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// tunel atiende una peticion CONNECT estableciendo una retransmision
// bidireccional entre el cliente y el destino.
func tunel(w http.ResponseWriter, r *http.Request, nodo string, n int64) {
	destino := r.Host // en CONNECT, la autoridad viaja en la linea de peticion
	inicio := time.Now()

	// --- Socket hacia el destino ---
	// net.Dial realiza socket() y connect(). No hay HTTP de por medio.
	salida, err := net.DialTimeout("tcp", destino, 10*time.Second)
	if err != nil {
		log.Printf("[%s] #%d CONNECT %s -> ERROR: %v", nodo, n, destino, err)
		http.Error(w, "no se pudo establecer el tunel: "+err.Error(),
			http.StatusBadGateway)
		return
	}
	defer salida.Close()
	log.Printf("[%s] #%d CONNECT %s -> %s", nodo, n, destino, salida.RemoteAddr())

	// --- Toma de control del socket del cliente ---
	// Hijack devuelve la conexion TCP cruda: a partir de aqui el servidor
	// HTTP deja de gestionarla y somos responsables de cerrarla.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "el servidor no permite tomar el control de la conexion",
			http.StatusInternalServerError)
		return
	}
	entrada, buffer, err := hj.Hijack()
	if err != nil {
		log.Printf("[%s] #%d hijack fallido: %v", nodo, n, err)
		return
	}
	defer entrada.Close()

	// Confirmacion al cliente. Se escribe a mano porque la conexion ya no
	// esta bajo el control del servidor HTTP.
	if _, err := buffer.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n"); err != nil {
		return
	}
	buffer.Flush()

	// Sin plazos: un tunel puede permanecer abierto indefinidamente.
	entrada.SetDeadline(time.Time{})
	salida.SetDeadline(time.Time{})

	// --- Retransmision bidireccional ---
	// Dos goroutines copian en sentidos opuestos. Cuando una termina cierra
	// su extremo de escritura, lo que provoca que la otra tambien termine.
	var wg sync.WaitGroup
	var haciaDestino, haciaCliente int64
	wg.Add(2)

	go func() {
		defer wg.Done()
		haciaDestino, _ = io.Copy(salida, buffer)
		if tcp, ok := salida.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()

	go func() {
		defer wg.Done()
		haciaCliente, _ = io.Copy(entrada, salida)
		if tcp, ok := entrada.(*net.TCPConn); ok {
			tcp.CloseWrite()
		}
	}()

	wg.Wait()

	log.Printf("[%s] #%d tunel cerrado: %d bytes al destino, %d al cliente, %v",
		nodo, n, haciaDestino, haciaCliente,
		time.Since(inicio).Round(time.Millisecond))
}
