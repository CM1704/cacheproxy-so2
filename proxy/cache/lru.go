// Package cache implementa el almacen de respuestas del proxy.
//
// La estructura combina una lista doblemente enlazada con una tabla
// asociativa, lo que permite consulta, insercion y desalojo en tiempo
// constante amortizado. La lista mantiene el orden de uso: el frente
// es la entrada usada mas recientemente y el fondo la candidata a
// desalojo cuando se alcanza la capacidad.
//
// Todas las operaciones publicas son seguras para uso concurrente.
package cache

import (
	"container/list"
	"net/http"
	"sync"
	"time"
)

// Entrada es una respuesta almacenada junto con sus metadatos de frescura.
type Entrada struct {
	Clave     string
	Estado    int
	Cabeceras http.Header
	Cuerpo    []byte

	Guardada time.Time // momento en que se almaceno
	Expira   time.Time // momento a partir del cual deja de ser fresca
}

// Edad devuelve el tiempo transcurrido desde que la entrada se almaceno.
// Corresponde al campo Age del RFC 9111.
func (e *Entrada) Edad() time.Duration {
	return time.Since(e.Guardada)
}

// Vencida indica si la entrada dejo de ser fresca.
func (e *Entrada) Vencida() bool {
	return time.Now().After(e.Expira)
}

// Estadisticas resume el comportamiento del almacen.
type Estadisticas struct {
	Aciertos  int64
	Fallos    int64
	Desalojos int64
	Vencidas  int64
	Entradas  int
	Capacidad int
}

// TasaAciertos devuelve la proporcion de aciertos sobre el total de
// consultas. Devuelve 0 si todavia no hubo consultas.
func (s Estadisticas) TasaAciertos() float64 {
	total := s.Aciertos + s.Fallos
	if total == 0 {
		return 0
	}
	return float64(s.Aciertos) / float64(total)
}

// LRU es una cache con politica de reemplazo por uso menos reciente y
// expiracion por tiempo de vida.
type LRU struct {
	mu        sync.Mutex
	capacidad int
	ttl       time.Duration

	orden *list.List               // frente = mas reciente
	items map[string]*list.Element // clave -> nodo de la lista

	aciertos  int64
	fallos    int64
	desalojos int64
	vencidas  int64
}

// NuevaLRU construye una cache con la capacidad y el tiempo de vida
// indicados. Una capacidad menor o igual a cero se ajusta a 1.
func NuevaLRU(capacidad int, ttl time.Duration) *LRU {
	if capacidad <= 0 {
		capacidad = 1
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	return &LRU{
		capacidad: capacidad,
		ttl:       ttl,
		orden:     list.New(),
		items:     make(map[string]*list.Element, capacidad),
	}
}

// Obtener devuelve la entrada asociada a la clave si existe y sigue fresca.
// Un acierto promueve la entrada al frente de la lista. Una entrada vencida
// se elimina y se contabiliza como fallo.
func (c *LRU) Obtener(clave string) (*Entrada, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[clave]
	if !ok {
		c.fallos++
		return nil, false
	}

	ent := elem.Value.(*Entrada)
	if ent.Vencida() {
		c.orden.Remove(elem)
		delete(c.items, clave)
		c.vencidas++
		c.fallos++
		return nil, false
	}

	c.orden.MoveToFront(elem)
	c.aciertos++
	return ent, true
}

// Guardar almacena una respuesta. Si la clave ya existia, reemplaza su
// contenido y la promueve. Si se supera la capacidad, desaloja la entrada
// menos recientemente usada.
//
// ttl permite fijar un tiempo de vida propio para esta entrada, tomado por
// ejemplo de la directiva max-age del origen. Un valor menor o igual a cero
// aplica el tiempo de vida por omision de la cache.
func (c *LRU) Guardar(clave string, estado int, cabeceras http.Header, cuerpo []byte, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ttl <= 0 {
		ttl = c.ttl
	}
	ahora := time.Now()

	if elem, ok := c.items[clave]; ok {
		ent := elem.Value.(*Entrada)
		ent.Estado = estado
		ent.Cabeceras = cabeceras
		ent.Cuerpo = cuerpo
		ent.Guardada = ahora
		ent.Expira = ahora.Add(ttl)
		c.orden.MoveToFront(elem)
		return
	}

	ent := &Entrada{
		Clave:     clave,
		Estado:    estado,
		Cabeceras: cabeceras,
		Cuerpo:    cuerpo,
		Guardada:  ahora,
		Expira:    ahora.Add(ttl),
	}
	c.items[clave] = c.orden.PushFront(ent)

	// Desalojo por capacidad: se retira desde el fondo de la lista.
	for c.orden.Len() > c.capacidad {
		ultimo := c.orden.Back()
		if ultimo == nil {
			break
		}
		viejo := c.orden.Remove(ultimo).(*Entrada)
		delete(c.items, viejo.Clave)
		c.desalojos++
	}
}

// Eliminar retira una entrada. Devuelve true si existia.
func (c *LRU) Eliminar(clave string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[clave]
	if !ok {
		return false
	}
	c.orden.Remove(elem)
	delete(c.items, clave)
	return true
}

// Vaciar elimina todas las entradas y conserva los contadores.
func (c *LRU) Vaciar() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.orden.Init()
	c.items = make(map[string]*list.Element, c.capacidad)
}

// Longitud devuelve el numero de entradas almacenadas.
func (c *LRU) Longitud() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.orden.Len()
}

// Estadisticas devuelve una fotografia de los contadores.
func (c *LRU) Estadisticas() Estadisticas {
	c.mu.Lock()
	defer c.mu.Unlock()
	return Estadisticas{
		Aciertos:  c.aciertos,
		Fallos:    c.fallos,
		Desalojos: c.desalojos,
		Vencidas:  c.vencidas,
		Entradas:  c.orden.Len(),
		Capacidad: c.capacidad,
	}
}

// Claves devuelve las claves ordenadas de mas a menos reciente.
// Se usa en las pruebas y en el punto de diagnostico.
func (c *LRU) Claves() []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	claves := make([]string, 0, c.orden.Len())
	for e := c.orden.Front(); e != nil; e = e.Next() {
		claves = append(claves, e.Value.(*Entrada).Clave)
	}
	return claves
}
