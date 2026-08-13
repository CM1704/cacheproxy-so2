package cache

// Cliente minimo del protocolo RESP para el almacen compartido (Valkey).
//
// Se implementa sobre net.Conn con la biblioteca estandar en lugar de usar
// una biblioteca externa. La razon es doble: evita una dependencia y, sobre
// todo, mantiene el manejo del socket a la vista, que es justamente lo que
// el proyecto debe demostrar. RESP es un protocolo de texto sencillo:
// las ordenes se envian como arreglos de cadenas con longitud explicita.

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ErrNoEncontrado indica que la clave no existe en el almacen compartido.
var ErrNoEncontrado = errors.New("clave no encontrada")

// entradaSerializada es la forma en que una respuesta viaja hacia el
// almacen compartido. El cuerpo, al ser []byte, se codifica en base64
// automaticamente por el paquete json.
type entradaSerializada struct {
	Estado    int         `json:"estado"`
	Cabeceras http.Header `json:"cabeceras"`
	Cuerpo    []byte      `json:"cuerpo"`
	Guardada  time.Time   `json:"guardada"`
}

// Compartida es un almacen de segundo nivel respaldado por Valkey.
type Compartida struct {
	direccion string
	timeout   time.Duration

	mu   sync.Mutex
	conn net.Conn
	lec  *bufio.Reader

	aciertos int64
	fallos   int64
	errores  int64
}

// NuevaCompartida construye el cliente. No conecta todavia: la conexion se
// establece de forma perezosa en la primera orden y se reintenta si se cae.
func NuevaCompartida(direccion string, timeout time.Duration) *Compartida {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Compartida{direccion: direccion, timeout: timeout}
}

// conectar abre el socket hacia Valkey. Debe llamarse con el mutex tomado.
func (c *Compartida) conectar() error {
	if c.conn != nil {
		return nil
	}
	// net.Dial realiza socket() y connect() hacia el almacen compartido.
	conn, err := net.DialTimeout("tcp", c.direccion, c.timeout)
	if err != nil {
		return err
	}
	c.conn = conn
	c.lec = bufio.NewReader(conn)
	return nil
}

// cerrar descarta la conexion actual para forzar una reconexion.
func (c *Compartida) cerrar() {
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.lec = nil
	}
}

// enviar escribe una orden en formato RESP y devuelve la respuesta cruda.
func (c *Compartida) enviar(partes ...string) (any, error) {
	if err := c.conectar(); err != nil {
		return nil, err
	}
	c.conn.SetDeadline(time.Now().Add(c.timeout))

	// Arreglo RESP: *N\r\n seguido de $len\r\ndato\r\n por cada parte.
	var b []byte
	b = append(b, '*')
	b = append(b, strconv.Itoa(len(partes))...)
	b = append(b, '\r', '\n')
	for _, p := range partes {
		b = append(b, '$')
		b = append(b, strconv.Itoa(len(p))...)
		b = append(b, '\r', '\n')
		b = append(b, p...)
		b = append(b, '\r', '\n')
	}

	if _, err := c.conn.Write(b); err != nil {
		c.cerrar()
		return nil, err
	}
	resp, err := c.leerRespuesta()
	if err != nil {
		c.cerrar()
		return nil, err
	}
	return resp, nil
}

// leerRespuesta interpreta los tipos basicos de RESP.
func (c *Compartida) leerRespuesta() (any, error) {
	linea, err := c.lec.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if len(linea) < 3 {
		return nil, errors.New("respuesta RESP truncada")
	}
	tipo := linea[0]
	cuerpo := linea[1 : len(linea)-2] // se descarta el CRLF

	switch tipo {
	case '+': // cadena simple
		return cuerpo, nil
	case '-': // error
		return nil, errors.New("valkey: " + cuerpo)
	case ':': // entero
		return strconv.ParseInt(cuerpo, 10, 64)
	case '$': // cadena masiva
		n, err := strconv.Atoi(cuerpo)
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, ErrNoEncontrado // $-1 significa clave inexistente
		}
		datos := make([]byte, n+2) // +2 por el CRLF final
		if _, err := ioReadFull(c.lec, datos); err != nil {
			return nil, err
		}
		return datos[:n], nil
	case '*': // arreglo, no se usa en este cliente
		return nil, errors.New("arreglo RESP no soportado")
	}
	return nil, errors.New("tipo RESP desconocido: " + string(tipo))
}

// Ping comprueba que el almacen responde.
func (c *Compartida) Ping() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	r, err := c.enviar("PING")
	if err != nil {
		return err
	}
	if s, ok := r.(string); ok && s == "PONG" {
		return nil
	}
	return errors.New("respuesta inesperada a PING")
}

// Obtener recupera una entrada del almacen compartido.
func (c *Compartida) Obtener(clave string) (*Entrada, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	r, err := c.enviar("GET", clave)
	if err != nil {
		if err == ErrNoEncontrado {
			c.fallos++
		} else {
			c.errores++
		}
		return nil, false
	}
	datos, ok := r.([]byte)
	if !ok {
		c.errores++
		return nil, false
	}

	var es entradaSerializada
	if err := json.Unmarshal(datos, &es); err != nil {
		c.errores++
		return nil, false
	}
	c.aciertos++
	return &Entrada{
		Clave:     clave,
		Estado:    es.Estado,
		Cabeceras: es.Cabeceras,
		Cuerpo:    es.Cuerpo,
		Guardada:  es.Guardada,
		Expira:    time.Now().Add(time.Hour), // la expiracion la aplica Valkey
	}, true
}

// Guardar deposita una entrada con expiracion delegada al almacen.
func (c *Compartida) Guardar(clave string, estado int, cabeceras http.Header, cuerpo []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	datos, err := json.Marshal(entradaSerializada{
		Estado: estado, Cabeceras: cabeceras, Cuerpo: cuerpo, Guardada: time.Now(),
	})
	if err != nil {
		return err
	}
	seg := int(ttl.Seconds())
	if seg < 1 {
		seg = 1
	}
	// SET clave valor EX segundos
	_, err = c.enviar("SET", clave, string(datos), "EX", strconv.Itoa(seg))
	if err != nil {
		c.errores++
	}
	return err
}

// EstadisticasCompartida resume el uso del segundo nivel.
type EstadisticasCompartida struct {
	Aciertos int64
	Fallos   int64
	Errores  int64
}

func (c *Compartida) Estadisticas() EstadisticasCompartida {
	c.mu.Lock()
	defer c.mu.Unlock()
	return EstadisticasCompartida{c.aciertos, c.fallos, c.errores}
}

// ioReadFull evita importar io solo para una funcion.
func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
