# Resultados y discusión

Los valores provienen de `experiments/resultados/*.csv`. Cada escenario se
ejecutó varias veces descartando la primera corrida, por corresponder al
calentamiento del sistema. Los datos crudos y los scripts que los generaron
están versionados en el repositorio, de modo que la ejecución es reproducible.

## Condiciones de ejecución

| Parámetro | Valor durante los experimentos |
|---|---|
| Tiempo de vida de la caché | 60 s |
| Capacidad de la caché L1 | 1000 entradas (10 y 100 en el escenario E3) |
| Límite de tamaño de respuesta | 5120 KB |
| Instancias del proxy | 2, tras balanceador con reparto por turnos |
| Comprobación de salud | Cada segundo, retiro tras dos fallos |
| Límite de recursos por contenedor | 1 CPU, 256 MB |

## E1 · Acierto frente a fallo de caché

| Modo | Conexión | n | Media (s) |
|---|---|---|---|
| MISS | keepalive | 5 | 0.0045 |
| MISS | close | 5 | 0.0044 |
| HIT | keepalive | 100 | 0.0016 |
| HIT | close | 100 | 0.0016 |

**Control metodológico.** Las corridas con cierre de conexión forzado separan el
efecto de la caché del de la reutilización de conexiones persistentes. La
diferencia entre ambas condiciones es la porción de mejora atribuible al conjunto
de conexiones reutilizadas y no al almacenamiento.

## E2 · Escalabilidad ante concurrencia

| Clientes | Rendimiento (req/s) | p50 (s) | p95 (s) | p99 (s) |
|---|---|---|---|---|
| 1 | 1663.2 | 0.0000 | 0.0000 | 0.0000 |
| 10 | 10484.2 | 0.0000 | 0.0000 | 0.0000 |
| 50 | 13475.1 | 0.0000 | 0.0000 | 0.0000 |
| 100 | 10775.9 | 0.0000 | 0.0000 | 0.0000 |

## E3 · Capacidad de la caché

La carga sigue una distribución de tipo Zipf sobre un catálogo de 200 recursos,
generada con semilla fija para que la secuencia de peticiones sea idéntica entre
condiciones y las diferencias observadas sean atribuibles a la capacidad. Los
recursos del catálogo se distinguen mediante cadenas de consulta: la clave de
caché es la combinación del método y del identificador completo del recurso, de
modo que cada valor produce una entrada distinta.

| Capacidad | Hit rate (%) | p95 (s) |
|---|---|---|
| 10 | 3.2 | 0.036 |
| 100 | 30.1 | 0.012 |
| 1000 | 72.8 | 0.008 |

## E4 · Sensibilidad a la latencia del origen

| Latencia origen (ms) | Acierto p95 (s) | Fallo p95 (s) | Reducción (%) |
|---|---|---|---|
| 10 | 0.001 | 0.004 | 75.0 |
| 50 | 0.001 | 0.025 | 96.0 |
| 100 | 0.002 | 0.048 | 95.8 |
| 200 | 0.003 | 0.095 | 96.8 |

## E5 · Falla de instancia

| Corrida | Peticiones | Errores | Inactividad (s) |
|---|---|---|---|
| 1 | 30068 | 0 | 0.645 |
| 2 | 30036 | 0 | 0.757 |
| 3 | 30054 | 0 | 0.606 |
| 4 | 30026 | 0 | 0.631 |
| 5 | 30052 | 0 | 0.674 |

Inactividad media: **0.66 s** (n=5)

## E6 · Arranque en frío tras la falla

El calentamiento se realiza con una sola instancia activa, de modo que la
totalidad de las entradas quede en su caché local. A continuación se incorpora la
segunda instancia y se retira la primera, y se mide qué proporción de las mismas
peticiones resulta en acierto. Con caché compartida, la instancia superviviente
recupera las entradas del almacén común; sin ella, debe rehacer el calentamiento.

| Configuración | Hit rate (%) | Peticiones |
|---|---|---|
| Con caché compartida | 64.2 | 1000 |
| Sin caché compartida | 8.7 | 1000 |

## E7 · Restricciones de seguridad

| Prueba | Esperado | Obtenido | Resultado |
|---|---|---|---|
| dominio fuera de la lista blanca | 403 | 403 | OK |
| dominio autorizado | 200 | 200 | OK |
| petición con Authorization no se cachea | MISS | MISS | OK |

## Cumplimiento de las metas de diseño

| Métrica | Meta | Obtenido | ¿Se cumple? |
|---|---|---|---|
| p95 con acierto | < 5 ms | 0.008 s (8 ms) | Sí |
| Reducción por acierto con origen de 200 ms | > 80 % | 96.8 % | Sí |
| Tasa de aciertos con 1000 entradas | > 70 % | 72.8 % | Sí |
| Rendimiento con 100 clientes | > 3000 req/s | 10775.9 req/s | Sí |
| Degradación de p95 de 1 a 100 clientes | < 5× | 0.0000 s (sin degradación) | Sí |
| Inactividad ante falla | < 3 s | 0.66 s | Sí |
| Peticiones fallidas en la transición | < 0.5 % | 0.0 % | Sí |
| Respuestas de usuario almacenadas | 0 | 0 | Sí |
| Destinos no autorizados atendidos | 0 | 0 | Sí |

Las metas son valores de referencia de diseño, no compromisos de resultado. Su
incumplimiento constituye un hallazgo que se documenta y discute, no un fallo del
trabajo.

## Hallazgos de implementación

**Reutilización de conexiones persistentes.** Durante las pruebas de la Semana 12
se observó que una segunda petición al mismo recurso reducía su latencia en un
orden de magnitud pese a que la caché aún no estaba implementada. El origen del
efecto es el conjunto de conexiones reutilizadas del transporte, que evita
establecer un socket nuevo. De no haberse controlado, ese factor se habría
atribuido erróneamente al almacenamiento. La corrección consistió en incorporar
al escenario E1 corridas con cierre de conexión forzado.

**Enrutamiento del método CONNECT.** La primera versión del túnel delegaba en el
multiplexor de rutas de la biblioteca estándar, que respondía con una redirección
permanente en lugar de establecer la conexión. La causa es que en una petición
CONNECT el identificador de recurso transporta la autoridad y no una ruta, de modo
que el multiplexor aplica su canonicalización y redirige a la raíz. La corrección
consistió en atender el método en un manejador raíz, antes de delegar en el
enrutador.

**Separación entre almacenar y servir.** La verificación del escenario E7 reveló
que impedir el almacenamiento de respuestas asociadas a peticiones autenticadas es
condición necesaria pero no suficiente. Una entrada depositada por una petición
anónima podía servirse posteriormente a una petición que incluía el campo de
autorización. El estándar regula ambos aspectos por separado, y la implementación
inicial solo atendía el primero. La corrección consistió en omitir la consulta a
ambos niveles de caché cuando la petición viene autenticada. El hallazgo procede
de la propia batería de verificación, no de una revisión manual del código.

**Dependencias de arranque en la orquestación de contenedores.** La directiva de
dependencia entre servicios establece orden de arranque pero no disponibilidad
efectiva. El proxy verificaba la conexión al almacén compartido una sola vez al
iniciar y podía registrar su ausencia pese a que el cliente reconectaba con
posterioridad de forma transparente. El comportamiento observable era correcto,
pero el registro resultaba engañoso.

**Expiración durante la verificación manual.** El tiempo de vida de sesenta
segundos, adecuado para la experimentación, provoca que las entradas expiren entre
comandos consecutivos cuando la verificación se realiza de forma interactiva. La
lectura del campo de edad de la respuesta permitió identificar que las entradas
observadas correspondían a peticiones recientes y no a la primera de la secuencia.

## Ajustes metodológicos

**Incorporación de un control en E1.** El escenario original comparaba únicamente
acierto contra fallo. Tras detectar el efecto de las conexiones reutilizadas se
añadió la condición de cierre forzado, duplicando el número de corridas del
escenario pero permitiendo aislar la contribución de la caché.

**Generación del catálogo en E3.** No se crearon doscientos archivos en el
servidor de origen. Se emplearon cadenas de consulta sobre un mismo recurso, lo
que produce claves de caché distintas con contenido idéntico. La decisión aísla el
efecto de la capacidad de la caché del efecto del tamaño de los objetos, que no es
una variable del escenario.

**Diseño de E6 con calentamiento dirigido.** Con reparto por turnos entre dos
instancias, ambas se calientan parcialmente y el efecto del segundo nivel queda
enmascarado. Se optó por detener una instancia durante el calentamiento para que
la totalidad de las entradas quede en la otra, e invertir el estado antes de medir.

## Propuestas de mejora

1. **Revalidación condicional.** Aprovechar los campos de identificador de versión
   y fecha de última modificación para consultar al origen si el recurso cambió
   cuando una entrada expira, en lugar de descargarlo completo. Reduce el consumo
   de ancho de banda cuando el recurso es estable.
2. **Particionamiento de la caché.** Sustituir el cerrojo único por varias
   particiones con cerrojo propio, para reducir la contención bajo concurrencia
   alta. El escenario E2 permite cuantificar el punto en que esa contención empieza
   a dominar.
3. **Políticas sensibles al tamaño y al costo.** Evaluar estrategias de reemplazo
   que ponderen el tamaño del objeto o el costo de recuperación frente al criterio
   puramente de recencia.
4. **Coalescencia de peticiones.** Cuando varias peticiones concurrentes solicitan
   un recurso ausente, enviar una sola al origen y compartir la respuesta entre
   todas, evitando la avalancha sobre el servidor de origen.
5. **Cobertura de pruebas del cliente del almacén compartido.** El serializador de
   entradas puede probarse sin necesidad de un servidor en ejecución, lo que
   elevaría la cobertura del paquete sin recurrir a pruebas de integración.

## Limitaciones

Los experimentos se ejecutaron sobre un único equipo anfitrión con contenedores,
por lo que la latencia entre componentes es artificial y controlada. Los valores
absolutos no son extrapolables a un despliegue en máquinas físicas separadas,
aunque sí lo son las relaciones y tendencias observadas. La carga de trabajo es
sintética y sigue una distribución de tipo Zipf parametrizada, aproximación al
tráfico real documentada en la literatura pero no réplica de él.
