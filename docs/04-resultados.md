# Semana 14–15 — Resultados y discusión

> Los valores de este documento provienen de `experiments/resultados/*.csv`.
> Cada escenario se ejecutó cinco veces descartando la primera corrida.

## E1 · Acierto frente a fallo de caché

| Modo | Conexión | n | Media (s) | p50 | p95 | p99 |
|---|---|---|---|---|---|---|
| MISS | keepalive | | | | | |
| HIT | keepalive | | | | | |
| MISS | close | | | | | |
| HIT | close | | | | | |

**Control metodológico.** Las corridas con `Connection: close` separan el efecto
de la caché del de la reutilización de conexiones persistentes. La diferencia
entre ambas columnas es la porción de mejora atribuible al pool de conexiones y
no al almacenamiento.

## E2 · Escalabilidad ante concurrencia

| Clientes | Rendimiento (req/s) | p50 (s) | p95 (s) | p99 (s) |
|---|---|---|---|---|
| 1 | | | | |
| 10 | | | | |
| 50 | | | | |
| 100 | | | | |

Factor de degradación de p95 entre 1 y 100 clientes: ___×

## E3 · Capacidad de la caché

| Entradas | Tasa de aciertos | Desalojos |
|---|---|---|
| 10 | | |
| 100 | | |
| 1000 | | |

## E4 · Sensibilidad a la latencia del origen

| Retardo del origen | MISS (s) | HIT (s) | Reducción |
|---|---|---|---|
| 0 ms | | | |
| 50 ms | | | |
| 200 ms | | | |

## E5 · Falla de instancia

| Corrida | Peticiones | Errores | % error | Inactividad (s) |
|---|---|---|---|---|
| 1 | | | | |

## E6 · Arranque en frío tras la falla

| Configuración | Tasa de aciertos tras el failover |
|---|---|
| Con caché compartida (L2) | |
| Solo caché local (L1) | |

## E7 · Restricciones de seguridad

| Prueba | Esperado | Obtenido | Resultado |
|---|---|---|---|
| Dominio fuera de la lista blanca | 403 | | |
| Dominio autorizado | 200 | | |
| Petición con Authorization no se cachea | MISS | | |

## Discusión

### Cumplimiento de las metas de diseño

| Métrica | Meta | Obtenido | ¿Se cumple? |
|---|---|---|---|
| p95 con acierto | < 5 ms | | |
| Reducción por acierto con origen de 200 ms | > 80 % | | |
| Tasa de aciertos con 1000 entradas | > 70 % | | |
| Rendimiento con 100 clientes | > 3000 req/s | | |
| Degradación de p95 | < 5× | | |
| Inactividad ante falla | < 3 s | | |
| Peticiones fallidas en la transición | < 0,5 % | | |
| Respuestas de usuario almacenadas | 0 | | |

### Hallazgos de implementación

**Reutilización de conexiones persistentes.** Durante las pruebas de la
Semana 12 se observó que una segunda petición al mismo recurso reducía su
latencia en un orden de magnitud aun sin caché implementada, por efecto del
pool de conexiones del transporte. Este factor de confusión motivó la
incorporación del control con cierre de conexión forzado en el escenario E1.

**Enrutamiento del método CONNECT.** La primera versión del túnel delegaba en el
enrutador HTTP de la biblioteca estándar, que respondía con una redirección en
lugar de establecer la conexión. La causa es que en una petición CONNECT la URL
transporta la autoridad y no una ruta, de modo que el enrutador aplica su
canonicalización de rutas. La corrección consistió en atender el método antes de
delegar en el enrutador.

**Promoción de segundo a primer nivel.** Un acierto en la caché compartida se
copia a la caché local de la instancia. Sin esta promoción, cada acierto de
segundo nivel pagaría indefinidamente el costo de la consulta remota.

### Ajustes metodológicos

_(Describir aquí cualquier cambio al diseño experimental derivado de los
resultados preliminares.)_

## Propuestas de mejora

1. **Revalidación condicional.** Aprovechar los campos `ETag` y `Last-Modified`
   para consultar al origen si el recurso cambió cuando una entrada expira, en
   lugar de descargarlo completo. Ahorra ancho de banda cuando el recurso es
   estable.
2. **Particionamiento de la caché.** Sustituir el mutex único por varias
   particiones con cerrojo propio, para reducir la contención con concurrencia
   alta.
3. **Políticas sensibles al tamaño y al costo.** Evaluar estrategias que
   ponderen el tamaño del objeto o el costo de recuperación frente al criterio
   puramente de recencia.
4. **Coalescencia de peticiones.** Cuando varias peticiones simultáneas piden un
   recurso ausente, enviar una sola al origen y compartir la respuesta.
