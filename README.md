# CacheProxy

Proxy HTTP concurrente con caché de dos niveles y alta disponibilidad,
implementado sobre sockets TCP en Go y desplegado con contenedores.

**Proyecto de Investigación · BISOF 18 Sistemas Operativos II**
Universidad Latina de Costa Rica · Facultad de Ingenierías en TICs

| | |
|---|---|
| Autor | Carlos Marín Abarca (20230111773) |
| Profesor | Carlos Andrés Méndez Rodríguez |
| Tema del enunciado | (c) Proxy HTTP con cache |
| Licencia | MIT |
| Estado | Semana 14 — desarrollo y análisis de resultados |

## Objetivo

Diseñar, implementar y evaluar experimentalmente un servidor proxy HTTP
concurrente con caché, desplegado en contenedores y en configuración de alta
disponibilidad, determinando su impacto sobre el tiempo de respuesta, su
comportamiento ante el aumento de clientes simultáneos y su continuidad de
servicio ante la falla de una de sus instancias.

## Arquitectura

    clientes
        |
        v
    HAProxy  (:18080, stats en :18404)
        |  round robin + health check contra /healthz
        |
        +--> proxy-a --+
        |              +--> Valkey  (cache L2 compartida)
        +--> proxy-b --+
                       |
                       v
                   origen (Nginx, retardo controlado con tc netem)

### Caché de dos niveles

| Nivel | Ubicación | Política | Marca en la respuesta |
|---|---|---|---|
| L1 | Memoria del proceso | LRU + expiración por TTL | `X-Cache: HIT-L1` |
| L2 | Valkey compartido | Expiración delegada al almacén | `X-Cache: HIT-L2` |
| — | Origen | — | `X-Cache: MISS` |

Un acierto en L2 se promueve a L1, de modo que la siguiente consulta al mismo
recurso no vuelva a salir del proceso. El L2 es lo que permite que una
instancia conserve el trabajo de calentamiento cuando la otra cae.

La decisión de almacenamiento sigue el RFC 9111 y es deliberadamente
conservadora: solo GET, solo códigos de estado almacenables, y se rechaza
cuando la petición trae `Authorization` o la respuesta trae `Set-Cookie`,
`no-store`, `private` o `no-cache`.

## Entorno de desarrollo

| Componente | Versión |
|---|---|
| Sistema | Ubuntu 26.04 LTS (Resolute Raccoon) sobre WSL2 |
| Kernel | Linux 7.0 |
| Docker Engine | 29.5.1 |
| Docker Compose | v5.1.3 |
| Go | 1.26.0 |

Sin dependencias externas de Go: el cliente RESP para Valkey está escrito
sobre `net.Conn` con la biblioteca estándar.

## Puesta en marcha

    docker compose up -d --build
    docker compose ps

Verificación del ciclo de caché:

    curl -x http://localhost:18080 -sD - http://origin/ -o /dev/null | grep -i x-cache   # MISS
    curl -x http://localhost:18080 -sD - http://origin/ -o /dev/null | grep -i x-cache   # HIT-L1

Estado del balanceador: http://localhost:18404/stats

## Puertos

| Puerto | Servicio |
|---|---|
| 18080 | HAProxy — entrada del proxy |
| 18404 | HAProxy — panel de estadísticas |

Solo estos dos se publican al host. Los servicios internos (proxy-a, proxy-b,
valkey, origin) viven únicamente en la red de contenedores. Se cambian en
`.env` y en ningún otro archivo.

## Configuración

Variables en `.env` y en `docker-compose.yml`:

| Variable | Por omisión | Descripción |
|---|---|---|
| `PUERTO_PROXY` | 18080 | Puerto publicado de HAProxy |
| `PUERTO_STATS` | 18404 | Puerto del panel de estadísticas |
| `CACHE_MAX_ENTRADAS` | 1000 | Capacidad de la caché L1 |
| `CACHE_TTL_SEGUNDOS` | 60 | Tiempo de vida por omisión |
| `VALKEY_ADDR` | valkey:6379 | Almacén compartido; vacío desactiva L2 |
| `DOMINIOS_PERMITIDOS` | origin | Lista blanca separada por coma |
| `MAX_RESPUESTA_KB` | 5120 | Tamaño máximo de respuesta almacenable |

## Endpoints de diagnóstico

| Ruta | Descripción |
|---|---|
| `/healthz` | Comprobación de salud para HAProxy |
| `/stats` | Contadores de L1, L2 y seguridad en JSON |

## Seguridad

- Lista blanca de dominios: los destinos no autorizados reciben 403.
- Límite de tamaño de respuesta, para evitar agotar la memoria del proceso.
- Eliminación de cabeceras de salto a salto conforme al RFC 9110.
- Exclusión del almacenamiento de respuestas específicas de usuario, que en
  una caché compartida expondrían la sesión de un usuario a otro.

## Experimentos

    ./experiments/scripts/e7-seguridad.sh          # instantáneo
    ./experiments/scripts/e1-acierto-vs-fallo.sh   # varios minutos
    ./experiments/scripts/e2-concurrencia.sh
    ./experiments/scripts/e5-falla.sh

| ID | Escenario | Variable manipulada |
|---|---|---|
| E1 | Acierto frente a fallo de caché | Estado de la caché |
| E2 | Escalabilidad ante concurrencia | 1, 10, 50, 100 clientes |
| E3 | Capacidad de la caché | 10, 100, 1000 entradas, carga Zipf |
| E4 | Sensibilidad a la latencia del origen | 0, 50, 200 ms |
| E5 | Falla de instancia | Detención bajo carga |
| E6 | Arranque en frío tras falla | L2 activo o desactivado |
| E7 | Restricciones de seguridad | Dominio y cabeceras |

Cada escenario se ejecuta cinco veces descartando la primera corrida, y se
reportan percentiles en lugar de promedios. Los resultados crudos quedan
versionados en `experiments/resultados/`.

## Estructura

| Ruta | Contenido |
|---|---|
| `docs/` | Documentación académica, marco teórico, referencias y entregas |
| `proxy/` | Código en Go |
| `proxy/cache/` | Caché LRU, política RFC 9111 y cliente RESP de Valkey |
| `proxy/security/` | Lista blanca y límites |
| `proxy/connect.go` | Túnel CONNECT sobre sockets crudos |
| `deploy/haproxy/` | Configuración del balanceador |
| `deploy/origin/` | Servidor de origen de pruebas |
| `experiments/` | Scripts de los escenarios y resultados crudos |
| `analysis/` | Procesamiento estadístico y gráficas |
| `setup/` | Scripts de preparación del entorno |

## Pruebas

    cd proxy
    go test ./cache/...          # pruebas unitarias
    go test -race ./cache/...    # detector de condiciones de carrera
    go test -cover ./cache/...   # cobertura

## Avance por semanas

| Semana | Etiqueta | Estado |
|---|---|---|
| 11 · Tema y objetivos | `v0.1-semana11` | Completado |
| 12 · Planificación y metodología | `v0.2-semana12` | Completado |
| 13 · Revisión bibliográfica | `v0.3-semana13` | Completado |
| 14 · Desarrollo y resultados | `v0.9-semana14` | En proceso |
| 15 · Entrega final | `v1.0.0` | Pendiente |

## Licencia

MIT. Ver [LICENSE](LICENSE).
