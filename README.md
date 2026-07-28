# CacheProxy

Proxy HTTP concurrente con caché y alta disponibilidad, implementado sobre
sockets TCP en Go y desplegado con contenedores.

**Proyecto de Investigación · BISOF 18 Sistemas Operativos II**
Universidad Latina de Costa Rica · Facultad de Ingenierías en TICs

| | |
|---|---|
| Autor | Carlos Marín Abarca (20230111773) |
| Profesor | Carlos Andrés Méndez Rodríguez |
| Tema del enunciado | (c) Proxy HTTP con cache |
| Licencia | MIT |
| Estado | Semana 11 — definición del tema y objetivos |

## Objetivo

Diseñar, implementar y evaluar experimentalmente un servidor proxy HTTP
concurrente con caché, desplegado en contenedores y en configuración de alta
disponibilidad, determinando su impacto sobre el tiempo de respuesta, su
comportamiento ante el aumento de clientes simultáneos y su continuidad de
servicio ante la falla de una de sus instancias.

## Arquitectura prevista

    clientes
        |
        v
    HAProxy  (:18080)
        |
        +--> proxy-a --+
        |              +--> Valkey  (cache L2 compartida)
        +--> proxy-b --+
                       |
                       v
                   origen (Nginx, retardo controlado)

## Entorno de desarrollo

| Componente | Versión |
|---|---|
| Sistema | Ubuntu 26.04 LTS (Resolute Raccoon) sobre WSL2 |
| Kernel | Linux 7.0 |
| Docker Engine | 29.5.1 |
| Docker Compose | v5.1.3 |
| Go | 1.26.0 |

## Puertos

| Puerto | Servicio |
|---|---|
| 18080 | HAProxy — entrada del proxy |
| 18404 | HAProxy — panel de estadísticas |

Se cambian en `.env` y en ningún otro archivo. Los servicios internos
(proxy-a, proxy-b, valkey, origin) no publican puertos al host.

## Estructura

| Ruta | Contenido |
|---|---|
| `docs/` | Documentación académica, referencias y entregas |
| `proxy/` | Código en Go: peticiones, caché y seguridad |
| `deploy/` | Balanceador y servidor de origen de pruebas |
| `experiments/` | Escenarios E1–E7 y resultados crudos |
| `analysis/` | Gráficas y estadística |
| `setup/` | Scripts de preparación del entorno |

## Avance por semanas

| Semana | Etiqueta | Estado |
|---|---|---|
| 11 · Tema y objetivos | `v0.1-semana11` | En curso |
| 12 · Planificación y metodología | `v0.2-semana12` | Pendiente |
| 13 · Revisión bibliográfica | `v0.3-semana13` | Pendiente |
| 14 · Desarrollo y resultados | `v0.9-semana14` | Pendiente |
| 15 · Entrega final | `v1.0.0` | Pendiente |

## Licencia

MIT. Ver [LICENSE](LICENSE).
