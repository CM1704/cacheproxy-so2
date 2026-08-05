package cache

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// estadosAlmacenables son los codigos de estado que el RFC 9111 define como
// almacenables por omision.
var estadosAlmacenables = map[int]bool{
	200: true, 203: true, 204: true, 300: true, 301: true,
	404: true, 405: true, 410: true, 414: true, 501: true,
}

// Decision explica por que una respuesta es o no almacenable. El motivo se
// registra para poder auditar el comportamiento de la cache.
type Decision struct {
	Almacenable bool
	Motivo      string
	TTL         time.Duration
}

// Almacenable determina si una respuesta puede guardarse en una cache
// compartida, conforme al RFC 9111.
//
// El criterio es deliberadamente conservador: ante la duda, no se almacena.
// Una cache compartida que guarda de mas puede exponer datos de un usuario
// a otro, que es un riesgo mayor que perder una oportunidad de acierto.
func Almacenable(req *http.Request, resp *http.Response, ttlPorOmision time.Duration) Decision {
	// Solo el metodo GET. HEAD no tiene cuerpo que almacenar y el resto de
	// metodos no son seguros ni idempotentes.
	if req.Method != http.MethodGet {
		return Decision{false, "metodo no almacenable: " + req.Method, 0}
	}

	if !estadosAlmacenables[resp.StatusCode] {
		return Decision{false, "estado no almacenable: " + strconv.Itoa(resp.StatusCode), 0}
	}

	// Peticion autenticada: la respuesta puede ser especifica de un usuario.
	if req.Header.Get("Authorization") != "" {
		return Decision{false, "peticion con Authorization", 0}
	}

	// Respuesta que establece estado en el cliente: reutilizarla entregaria
	// la sesion de un usuario a otro.
	if resp.Header.Get("Set-Cookie") != "" {
		return Decision{false, "respuesta con Set-Cookie", 0}
	}

	ccPeticion := directivas(req.Header.Get("Cache-Control"))
	if _, no := ccPeticion["no-store"]; no {
		return Decision{false, "peticion con no-store", 0}
	}

	ccRespuesta := directivas(resp.Header.Get("Cache-Control"))
	if _, no := ccRespuesta["no-store"]; no {
		return Decision{false, "respuesta con no-store", 0}
	}
	// private prohibe el almacenamiento en caches compartidas.
	if _, priv := ccRespuesta["private"]; priv {
		return Decision{false, "respuesta marcada private", 0}
	}
	// no-cache permite almacenar pero exige revalidar antes de reutilizar.
	// La revalidacion condicional no esta implementada, asi que no se guarda.
	if _, nc := ccRespuesta["no-cache"]; nc {
		return Decision{false, "respuesta con no-cache", 0}
	}

	// Tiempo de vida: s-maxage tiene prioridad en caches compartidas,
	// luego max-age, y en ultimo lugar el valor por omision del proxy.
	ttl := ttlPorOmision
	if v, ok := ccRespuesta["s-maxage"]; ok {
		if seg, err := strconv.Atoi(v); err == nil && seg > 0 {
			ttl = time.Duration(seg) * time.Second
		}
	} else if v, ok := ccRespuesta["max-age"]; ok {
		if seg, err := strconv.Atoi(v); err == nil {
			if seg <= 0 {
				return Decision{false, "max-age igual a cero", 0}
			}
			ttl = time.Duration(seg) * time.Second
		}
	}

	return Decision{true, "almacenable", ttl}
}

// directivas descompone el valor de una cabecera Cache-Control en un mapa.
// Las directivas sin valor quedan asociadas a la cadena vacia.
func directivas(valor string) map[string]string {
	res := make(map[string]string)
	if valor == "" {
		return res
	}
	for _, parte := range strings.Split(valor, ",") {
		parte = strings.TrimSpace(strings.ToLower(parte))
		if parte == "" {
			continue
		}
		if i := strings.IndexByte(parte, '='); i >= 0 {
			clave := strings.TrimSpace(parte[:i])
			val := strings.Trim(strings.TrimSpace(parte[i+1:]), "\"")
			res[clave] = val
		} else {
			res[parte] = ""
		}
	}
	return res
}

// Clave construye el identificador de una respuesta almacenada.
// El RFC 9111 define la clave primaria como la combinacion del metodo y
// del identificador del recurso.
func Clave(metodo, uri string) string {
	return metodo + " " + uri
}
