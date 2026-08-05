package cache

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

func entradaDePrueba(c *LRU, clave string) {
	c.Guardar(clave, 200, http.Header{}, []byte("contenido de "+clave), 0)
}

func TestGuardarYObtener(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	entradaDePrueba(c, "GET /a")

	ent, ok := c.Obtener("GET /a")
	if !ok {
		t.Fatal("se esperaba un acierto")
	}
	if string(ent.Cuerpo) != "contenido de GET /a" {
		t.Errorf("cuerpo inesperado: %q", ent.Cuerpo)
	}
	if ent.Estado != 200 {
		t.Errorf("estado inesperado: %d", ent.Estado)
	}
}

func TestFalloEnClaveInexistente(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	if _, ok := c.Obtener("GET /no-existe"); ok {
		t.Fatal("se esperaba un fallo")
	}
	if s := c.Estadisticas(); s.Fallos != 1 {
		t.Errorf("fallos = %d, se esperaba 1", s.Fallos)
	}
}

func TestDesalojoPorCapacidad(t *testing.T) {
	c := NuevaLRU(3, time.Minute)
	for _, k := range []string{"GET /1", "GET /2", "GET /3"} {
		entradaDePrueba(c, k)
	}
	if c.Longitud() != 3 {
		t.Fatalf("longitud = %d, se esperaba 3", c.Longitud())
	}

	// La cuarta entrada debe desalojar a la mas antigua (GET /1).
	entradaDePrueba(c, "GET /4")

	if c.Longitud() != 3 {
		t.Errorf("longitud = %d, se esperaba 3 tras el desalojo", c.Longitud())
	}
	if _, ok := c.Obtener("GET /1"); ok {
		t.Error("GET /1 deberia haber sido desalojada")
	}
	if _, ok := c.Obtener("GET /4"); !ok {
		t.Error("GET /4 deberia estar presente")
	}
	if s := c.Estadisticas(); s.Desalojos != 1 {
		t.Errorf("desalojos = %d, se esperaba 1", s.Desalojos)
	}
}

func TestOrdenLRUSePromueveAlConsultar(t *testing.T) {
	c := NuevaLRU(3, time.Minute)
	for _, k := range []string{"GET /1", "GET /2", "GET /3"} {
		entradaDePrueba(c, k)
	}

	// Consultar GET /1 la promueve: ahora la menos reciente es GET /2.
	if _, ok := c.Obtener("GET /1"); !ok {
		t.Fatal("GET /1 deberia estar presente")
	}
	entradaDePrueba(c, "GET /4")

	if _, ok := c.Obtener("GET /2"); ok {
		t.Error("GET /2 deberia haber sido desalojada, no GET /1")
	}
	if _, ok := c.Obtener("GET /1"); !ok {
		t.Error("GET /1 fue promovida y deberia seguir presente")
	}
}

func TestExpiracionPorTTL(t *testing.T) {
	c := NuevaLRU(10, 50*time.Millisecond)
	entradaDePrueba(c, "GET /efimera")

	if _, ok := c.Obtener("GET /efimera"); !ok {
		t.Fatal("deberia estar fresca inmediatamente")
	}

	time.Sleep(80 * time.Millisecond)

	if _, ok := c.Obtener("GET /efimera"); ok {
		t.Error("deberia haber vencido")
	}
	if s := c.Estadisticas(); s.Vencidas != 1 {
		t.Errorf("vencidas = %d, se esperaba 1", s.Vencidas)
	}
	if c.Longitud() != 0 {
		t.Errorf("la entrada vencida deberia haberse eliminado")
	}
}

func TestTTLPropioPorEntrada(t *testing.T) {
	c := NuevaLRU(10, time.Hour)
	c.Guardar("GET /corta", 200, http.Header{}, []byte("x"), 40*time.Millisecond)

	time.Sleep(70 * time.Millisecond)
	if _, ok := c.Obtener("GET /corta"); ok {
		t.Error("el TTL propio deberia prevalecer sobre el de la cache")
	}
}

func TestReemplazoDeClaveExistente(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	c.Guardar("GET /a", 200, http.Header{}, []byte("v1"), 0)
	c.Guardar("GET /a", 200, http.Header{}, []byte("v2"), 0)

	if c.Longitud() != 1 {
		t.Errorf("longitud = %d, se esperaba 1", c.Longitud())
	}
	ent, _ := c.Obtener("GET /a")
	if string(ent.Cuerpo) != "v2" {
		t.Errorf("cuerpo = %q, se esperaba v2", ent.Cuerpo)
	}
}

func TestEdadCrece(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	entradaDePrueba(c, "GET /a")
	time.Sleep(30 * time.Millisecond)

	ent, _ := c.Obtener("GET /a")
	if ent.Edad() < 25*time.Millisecond {
		t.Errorf("edad = %v, se esperaba al menos 25ms", ent.Edad())
	}
}

func TestEliminarYVaciar(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	entradaDePrueba(c, "GET /a")
	entradaDePrueba(c, "GET /b")

	if !c.Eliminar("GET /a") {
		t.Error("Eliminar deberia devolver true")
	}
	if c.Eliminar("GET /a") {
		t.Error("Eliminar deberia devolver false la segunda vez")
	}
	c.Vaciar()
	if c.Longitud() != 0 {
		t.Error("Vaciar deberia dejar la cache sin entradas")
	}
}

func TestTasaAciertos(t *testing.T) {
	c := NuevaLRU(10, time.Minute)
	entradaDePrueba(c, "GET /a")

	c.Obtener("GET /a") // acierto
	c.Obtener("GET /a") // acierto
	c.Obtener("GET /z") // fallo

	s := c.Estadisticas()
	if s.Aciertos != 2 || s.Fallos != 1 {
		t.Fatalf("aciertos=%d fallos=%d", s.Aciertos, s.Fallos)
	}
	esperado := 2.0 / 3.0
	if diff := s.TasaAciertos() - esperado; diff > 0.001 || diff < -0.001 {
		t.Errorf("tasa = %f, se esperaba %f", s.TasaAciertos(), esperado)
	}
}

// TestAccesoConcurrente ejercita la cache desde muchas goroutines a la vez.
// Con la bandera -race, el detector senala cualquier acceso sin sincronizar.
func TestAccesoConcurrente(t *testing.T) {
	c := NuevaLRU(50, time.Minute)
	var wg sync.WaitGroup

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			clave := "GET /r" + strconv.Itoa(n%20)
			for j := 0; j < 50; j++ {
				c.Guardar(clave, 200, http.Header{}, []byte("x"), 0)
				c.Obtener(clave)
				c.Estadisticas()
				c.Longitud()
			}
		}(i)
	}
	wg.Wait()

	if c.Longitud() > 50 {
		t.Errorf("la cache excedio su capacidad: %d", c.Longitud())
	}
}

// --- Politica de almacenamiento ---

func respuestaDePrueba(codigo int, cabeceras map[string]string) *http.Response {
	rec := httptest.NewRecorder()
	for k, v := range cabeceras {
		rec.Header().Set(k, v)
	}
	rec.WriteHeader(codigo)
	return rec.Result()
}

func TestPoliticaCasosAlmacenables(t *testing.T) {
	req := httptest.NewRequest("GET", "http://origin/a", nil)
	resp := respuestaDePrueba(200, nil)

	d := Almacenable(req, resp, time.Minute)
	if !d.Almacenable {
		t.Errorf("deberia ser almacenable, motivo: %s", d.Motivo)
	}
	if d.TTL != time.Minute {
		t.Errorf("TTL = %v, se esperaba el valor por omision", d.TTL)
	}
}

func TestPoliticaRechazos(t *testing.T) {
	casos := []struct {
		nombre   string
		metodo   string
		codigo   int
		cabPet   map[string]string
		cabResp  map[string]string
		esperaSi bool
	}{
		{"metodo POST", "POST", 200, nil, nil, false},
		{"estado 500", "GET", 500, nil, nil, false},
		{"peticion con Authorization", "GET", 200, map[string]string{"Authorization": "Bearer x"}, nil, false},
		{"respuesta con Set-Cookie", "GET", 200, nil, map[string]string{"Set-Cookie": "sid=1"}, false},
		{"respuesta no-store", "GET", 200, nil, map[string]string{"Cache-Control": "no-store"}, false},
		{"respuesta private", "GET", 200, nil, map[string]string{"Cache-Control": "private"}, false},
		{"respuesta no-cache", "GET", 200, nil, map[string]string{"Cache-Control": "no-cache"}, false},
		{"max-age cero", "GET", 200, nil, map[string]string{"Cache-Control": "max-age=0"}, false},
		{"GET simple 200", "GET", 200, nil, nil, true},
		{"estado 404", "GET", 404, nil, nil, true},
	}

	for _, c := range casos {
		t.Run(c.nombre, func(t *testing.T) {
			req := httptest.NewRequest(c.metodo, "http://origin/a", nil)
			for k, v := range c.cabPet {
				req.Header.Set(k, v)
			}
			resp := respuestaDePrueba(c.codigo, c.cabResp)

			d := Almacenable(req, resp, time.Minute)
			if d.Almacenable != c.esperaSi {
				t.Errorf("almacenable = %v (motivo: %s), se esperaba %v",
					d.Almacenable, d.Motivo, c.esperaSi)
			}
		})
	}
}

func TestPoliticaTTLDesdeMaxAge(t *testing.T) {
	req := httptest.NewRequest("GET", "http://origin/a", nil)
	resp := respuestaDePrueba(200, map[string]string{"Cache-Control": "max-age=120"})

	d := Almacenable(req, resp, time.Minute)
	if !d.Almacenable {
		t.Fatalf("deberia ser almacenable: %s", d.Motivo)
	}
	if d.TTL != 120*time.Second {
		t.Errorf("TTL = %v, se esperaba 120s", d.TTL)
	}
}

func TestPoliticaSMaxagePrevalece(t *testing.T) {
	req := httptest.NewRequest("GET", "http://origin/a", nil)
	resp := respuestaDePrueba(200, map[string]string{
		"Cache-Control": "max-age=60, s-maxage=300",
	})

	d := Almacenable(req, resp, time.Minute)
	if d.TTL != 300*time.Second {
		t.Errorf("TTL = %v, s-maxage deberia prevalecer en cache compartida", d.TTL)
	}
}

func TestClave(t *testing.T) {
	if k := Clave("GET", "http://origin/a"); k != "GET http://origin/a" {
		t.Errorf("clave = %q", k)
	}
}
