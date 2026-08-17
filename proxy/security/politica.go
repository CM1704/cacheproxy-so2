package security

// Controles de seguridad del proxy.
//
// Un proxy sin restricciones es un proxy abierto: cualquiera puede usarlo
// como intermediario hacia destinos arbitrarios, lo que lo convierte en
// herramienta de anonimizacion y en un riesgo para quien lo opera. La lista
// blanca acota los destinos alcanzables y el limite de tamano evita que una
// respuesta desmedida agote la memoria del proceso.

import (
	"net"
	"strings"
	"sync"
)

// Politica agrupa las restricciones aplicables a una peticion.
type Politica struct {
	mu            sync.RWMutex
	dominios      []string
	sinRestringir bool
	maxBytes      int64

	bloqueados int64
	excedidos  int64
}

// NuevaPolitica construye la politica a partir de una lista de dominios
// separados por coma. Una lista vacia deja el proxy sin restriccion de
// destino, situacion que se registra explicitamente al arrancar.
//
// Un dominio se acepta tal cual o como sufijo: "ejemplo.com" habilita
// tambien "www.ejemplo.com" y "api.ejemplo.com".
func NuevaPolitica(lista string, maxBytes int64) *Politica {
	p := &Politica{maxBytes: maxBytes}
	lista = strings.TrimSpace(lista)
	if lista == "" || lista == "*" {
		p.sinRestringir = true
		return p
	}
	for _, d := range strings.Split(lista, ",") {
		d = strings.ToLower(strings.TrimSpace(d))
		if d != "" {
			p.dominios = append(p.dominios, d)
		}
	}
	return p
}

// Permitido indica si el destino esta autorizado. Acepta tanto "host" como
// "host:puerto", que es la forma en que llega en una peticion CONNECT.
func (p *Politica) Permitido(host string) bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.sinRestringir {
		return true
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))

	for _, d := range p.dominios {
		if host == d || strings.HasSuffix(host, "."+d) {
			return true
		}
	}
	return false
}

// RegistrarBloqueo incrementa el contador de destinos rechazados.
func (p *Politica) RegistrarBloqueo() {
	p.mu.Lock()
	p.bloqueados++
	p.mu.Unlock()
}

// RegistrarExceso incrementa el contador de respuestas descartadas por tamano.
func (p *Politica) RegistrarExceso() {
	p.mu.Lock()
	p.excedidos++
	p.mu.Unlock()
}

// MaxBytes devuelve el limite de tamano de respuesta en bytes.
func (p *Politica) MaxBytes() int64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.maxBytes
}

// SinRestringir indica si el proxy acepta cualquier destino.
func (p *Politica) SinRestringir() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.sinRestringir
}

// Dominios devuelve la lista autorizada.
func (p *Politica) Dominios() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	res := make([]string, len(p.dominios))
	copy(res, p.dominios)
	return res
}

// Contadores devuelve las estadisticas de rechazo.
func (p *Politica) Contadores() (bloqueados, excedidos int64) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.bloqueados, p.excedidos
}
