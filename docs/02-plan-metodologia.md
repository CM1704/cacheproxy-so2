# Semana 12 — Planificación y metodología

## Enfoque

Investigación cuantitativa, de alcance descriptivo y correlacional, con diseño
experimental de tipo laboratorio. Se manipulan variables independientes sobre un
banco de pruebas controlado y se registran los efectos sobre variables de
desempeño y disponibilidad. La unidad de análisis es la petición HTTP individual.

## Variables

| Tipo | Variable | Definición operacional | Instrumento |
|---|---|---|---|
| Independiente | Estado de la caché | Fría o caliente | Control por script |
| Independiente | Concurrencia | 1, 10, 50, 100 clientes | Parámetro de hey |
| Independiente | Capacidad de la caché | 10, 100, 1000 entradas | Variable de entorno |
| Independiente | Latencia del origen | 0, 50, 200 ms | tc netem |
| Independiente | Disponibilidad | Instancia activa o detenida | docker stop |
| Independiente | Caché compartida | Segundo nivel activo o no | Configuración |
| Dependiente | Tiempo de respuesta | ms entre envío y recepción completa | hey |
| Dependiente | Rendimiento sostenido | Peticiones por segundo en régimen estable | hey |
| Dependiente | Tasa de aciertos | Proporción de respuestas con X-Cache HIT | Contador interno |
| Dependiente | Tiempo de inactividad | Segundos sin respuestas tras la falla | Serie temporal |
| Dependiente | Peticiones fallidas | Proporción de errores en la transición | hey |
| Control | Entorno | Mismo anfitrión, mismos límites por contenedor | Compose |

## Escenarios experimentales

| ID | Escenario | Variable manipulada | Objetivo atendido |
|---|---|---|---|
| E1 | Acierto frente a fallo de caché | Estado de la caché | OE4 |
| E2 | Escalabilidad ante concurrencia | 1, 10, 50, 100 clientes | OE3 y OE4 |
| E3 | Capacidad de la caché | 10, 100, 1000 entradas con carga Zipf | OE2 |
| E4 | Sensibilidad a la latencia del origen | 0, 50, 200 ms | OE4 |
| E5 | Falla de instancia | Detención bajo carga | OE5 |
| E6 | Arranque en frío tras falla | Caché compartida activa o no | OE5 |
| E7 | Restricciones de seguridad | Dominio no autorizado, estado de usuario | OE1 y OE2 |

Cada escenario se ejecuta un mínimo de cinco veces. La primera corrida se
descarta por calentamiento. Se reportan media, desviación estándar y percentiles
50, 95 y 99, privilegiando percentiles sobre promedios porque la distribución de
latencias en sistemas concurrentes presenta cola derecha pronunciada.

## Métricas y valores de referencia

| Atributo | Métrica | Referencia |
|---|---|---|
| Desempeño | Tiempo de respuesta p95 con acierto | Menor a 5 ms |
| Desempeño | Reducción de latencia por acierto | Superior al 80 % con origen de 200 ms |
| Desempeño | Tasa de aciertos con carga Zipf | Superior al 70 % con 1000 entradas |
| Escalabilidad | Rendimiento con 100 clientes | Superior a 3000 peticiones por segundo |
| Escalabilidad | Degradación de p95 de 1 a 100 clientes | Inferior a cinco veces |
| Fiabilidad | Tiempo de inactividad ante falla | Inferior a 3 s |
| Fiabilidad | Peticiones fallidas en la transición | Inferior al 0,5 % |
| Seguridad | Respuestas con estado de usuario almacenadas | Cero |
| Seguridad | Peticiones a dominios no autorizados atendidas | Cero |

Son metas de diseño, no compromisos de resultado. Su incumplimiento constituye
un hallazgo que será documentado y discutido.

## Estado de la implementación

Versión mínima funcional operativa: el proxy acepta conexiones en un socket TCP
propio, valida que la petición traiga URI absoluta, elimina las cabeceras de
salto a salto conforme al RFC 9110, establece una conexión saliente hacia el
servidor de origen y devuelve la respuesta. Expone un punto de comprobación de
salud para el balanceador de carga y registra cada conexión saliente.

Pendiente: caché con reemplazo y expiración (Semana 13); túnel CONNECT, caché
compartida, alta disponibilidad y controles de seguridad (Semana 14).
