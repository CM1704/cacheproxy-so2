# Semana 13 — Marco teórico

## 1. Procesos, concurrencia y el papel del sistema operativo

El sistema operativo administra la ejecución concurrente mediante la abstracción
de proceso y, dentro de él, la de hilo. Un servidor que atiende múltiples
clientes debe decidir cómo asocia esas abstracciones a las conexiones entrantes.
El modelo de un proceso por conexión ofrece aislamiento a un costo elevado de
creación y cambio de contexto; el de un hilo por conexión reduce ese costo pero
mantiene un consumo de memoria de pila apreciable; los modelos de multiplexación
de entrada y salida sobre un único hilo minimizan el consumo pero complican la
lógica del programa (Silberschatz et al., 2018).

El lenguaje Go introduce una posición intermedia mediante las goroutines, flujos
de ejecución ligeros gestionados por un planificador propio en espacio de usuario
que los multiplexa sobre un conjunto reducido de hilos del sistema operativo. El
costo inicial de una goroutine es del orden de unos pocos kilobytes de pila,
expandible dinámicamente, lo que permite mantener un modelo de programación
secuencial y legible sin incurrir en el costo del modelo equivalente basado en
hilos nativos (Donovan y Kernighan, 2016).

La concurrencia introduce el problema del acceso simultáneo a estado compartido.
La caché del proxy es leída y escrita por todas las goroutines activas, de modo
que su integridad depende de un mecanismo de exclusión mutua correcto. Una
condición de carrera en esa estructura no produce necesariamente un fallo
visible: puede manifestarse como corrupción intermitente bajo carga alta, lo que
hace indispensable la verificación instrumentada durante las pruebas.

## 2. Sockets y el modelo cliente-servidor

El socket constituye la interfaz mediante la cual un proceso solicita al núcleo
del sistema operativo el establecimiento y la gestión de un canal de
comunicación. Sobre el protocolo de control de transmisión, la secuencia canónica
del lado servidor comprende la creación del descriptor, su asociación a una
dirección y un puerto, la habilitación de la cola de conexiones pendientes y la
aceptación de conexiones entrantes; el lado cliente ejecuta la creación del
descriptor y la solicitud de conexión (Stevens et al., 2004).

Una propiedad del protocolo resulta especialmente relevante para el diseño de un
proxy: se trata de un servicio de flujo de bytes sin preservación de fronteras de
mensaje. El protocolo garantiza que los bytes lleguen ordenados y sin pérdidas,
pero no que una escritura del emisor corresponda a una lectura del receptor.
Corresponde al protocolo de aplicación establecer dónde termina un mensaje, lo
que en HTTP/1.1 se resuelve mediante la línea en blanco que cierra la sección de
cabeceras y, para el cuerpo, mediante el campo de longitud de contenido o la
codificación de transferencia por fragmentos (Fielding et al., 2022c).

El proxy ocupa una posición particular: mantiene simultáneamente un socket en el
rol de servidor, frente al cliente, y uno o varios sockets en el rol de cliente,
frente a los servidores de origen. Toda su lógica consiste en mediar entre ambos
extremos.

## 3. El protocolo HTTP y sus intermediarios

El protocolo de transferencia de hipertexto define una semántica de petición y
respuesta sin estado sobre una conexión de transporte confiable. La especificación
vigente distingue entre la semántica del protocolo (Fielding et al., 2022b), su
sintaxis de mensajes para la versión 1.1 (Fielding et al., 2022c) y sus reglas de
caché (Fielding et al., 2022a).

La especificación reconoce tres tipos de intermediario. El proxy directo es
seleccionado por el cliente, que le dirige peticiones con identificador de recurso
absoluto y actúa en su representación. La pasarela, o proxy inverso, se presenta
ante el cliente como el servidor de origen. El túnel se limita a retransmitir la
conexión sin interpretar su contenido. Este proyecto implementa un proxy directo,
con capacidad de operar como túnel cuando la petición emplea el método CONNECT.

Un requisito de corrección frecuentemente omitido es el tratamiento de los campos
de cabecera de salto a salto, aquellos cuya validez se limita a una única conexión
y que un intermediario debe eliminar antes de reenviar el mensaje. Su
retransmisión indebida constituye una violación del estándar y puede producir
comportamientos anómalos en los extremos.

## 4. Caché web: fundamentos y control de frescura

Una caché HTTP es un almacén local de respuestas previas y el subsistema que
controla su reutilización. La especificación distingue entre cachés privadas,
dedicadas a un único usuario, y cachés compartidas, que atienden a múltiples
usuarios; la distinción es determinante porque una respuesta almacenable en una
caché privada puede no serlo en una compartida (Fielding et al., 2022a).

El estándar establece las condiciones que debe cumplir una respuesta para ser
almacenada: que el método de la petición sea susceptible de almacenamiento, que
el código de estado sea reconocido como almacenable y que ninguna directiva de
control de caché lo prohíba. Define asimismo el concepto de frescura: una
respuesta almacenada es fresca mientras su edad no supere el tiempo de vida
calculado a partir de las directivas recibidas del origen. El campo que expresa
la edad de la respuesta y las directivas de control constituyen el mecanismo
mediante el cual el servidor de origen conserva autoridad sobre el comportamiento
de las cachés intermedias.

Las implicaciones de seguridad merecen atención explícita. Una respuesta que
contiene información específica de un usuario, señalada típicamente por la
presencia de un campo de autorización en la petición o de directivas de
establecimiento de estado en la respuesta, no debe almacenarse en una caché
compartida, pues su reutilización expondría datos de un usuario a otro. Este
proyecto implementa dicha restricción como requisito funcional y la verifica
mediante pruebas unitarias.

## 5. Políticas de reemplazo y localidad de referencia

Toda caché opera bajo capacidad finita, de modo que la decisión sobre qué entrada
desalojar cuando el almacén se llena determina en buena medida su eficacia.
Podlipnig y Böszörmenyi (2003) sistematizan las estrategias de reemplazo para
caché web en familias según el criterio predominante: recencia de acceso,
frecuencia de acceso, tamaño del objeto, costo de recuperación y combinaciones de
estos factores. La política de uso menos reciente pertenece a la primera familia y
constituye la referencia práctica por su simplicidad de implementación —una lista
doblemente enlazada combinada con una tabla asociativa proporciona operaciones en
tiempo constante amortizado— y por su buen desempeño ante cargas con localidad
temporal.

La eficacia de cualquier política depende de la estructura estadística de la
carga. Breslau et al. (1999) establecieron que las peticiones web siguen una
distribución de tipo Zipf, en la que la frecuencia de acceso a un recurso es
inversamente proporcional a una potencia de su posición en el ordenamiento por
popularidad. La consecuencia práctica es doble: una fracción reducida del catálogo
concentra la mayor parte del tráfico, lo que hace que cachés relativamente
pequeñas alcancen tasas de acierto elevadas; pero la tasa de aciertos crece de
forma logarítmica respecto del tamaño de la caché, de modo que los incrementos
sucesivos de capacidad ofrecen rendimientos decrecientes. Wang (1999) ofrece una
sistematización complementaria de los esquemas de caché web y de sus arquitecturas
de cooperación.

## 6. Alta disponibilidad y tolerancia a fallos

La disponibilidad se define como la proporción de tiempo durante la cual un
sistema se encuentra en condiciones de prestar su servicio, y su mejora se apoya
en la eliminación de los puntos únicos de falla mediante redundancia. Un
balanceador de carga situado frente a varias instancias equivalentes proporciona
un punto de entrada único a los clientes y distribuye el tráfico entre las
instancias operativas, retirando de la rotación aquellas que no superan sus
comprobaciones periódicas de salud (Tanenbaum y Van Steen, 2023).

La redundancia de instancias resuelve la continuidad del servicio, pero no la del
estado. Si cada instancia mantiene su caché exclusivamente en memoria local, la
falla de una de ellas implica la pérdida de todo el trabajo de calentamiento
acumulado, y el tráfico redirigido a la instancia superviviente encuentra una
caché parcialmente ajena a su patrón de acceso. La introducción de un almacén
compartido de segundo nivel mitiga este efecto. El costo es una latencia adicional
en el segundo nivel y la introducción de una nueva dependencia, cuya criticidad
debe evaluarse.

Beyer et al. (2016) argumentan que la disponibilidad debe expresarse mediante
indicadores medibles y objetivos explícitos, y no como una propiedad cualitativa.
En coherencia con ese planteamiento, este proyecto define el tiempo de inactividad
ante falla y la proporción de peticiones fallidas durante la transición como
variables observadas, sujetas a medición experimental.

## 7. Virtualización ligera y reproducibilidad

Los contenedores constituyen una forma de virtualización a nivel de sistema
operativo que aísla procesos mediante espacios de nombres y grupos de control del
núcleo, compartiendo el núcleo del anfitrión en lugar de emular hardware. El
resultado es un aislamiento suficiente para la mayoría de los casos de uso con un
costo de arranque y de recursos sustancialmente menor que el de la virtualización
completa (Bernstein, 2014; Merkel, 2014).

Para esta investigación, la virtualización ligera aporta un beneficio metodológico
específico: la definición declarativa del banco de pruebas convierte el entorno
experimental en un artefacto versionado y reconstruible con un único comando. La
reproducibilidad de los experimentos deja de depender de la documentación de una
configuración manual.

## 8. Trabajos relacionados y brecha identificada

La literatura sobre caché web abarca desde las caracterizaciones estadísticas de
carga y las taxonomías de políticas de reemplazo hasta las arquitecturas de caché
cooperativa y jerárquica (Wang, 1999; Wessels, 2001; Podlipnig y Böszörmenyi,
2003). Los trabajos de referencia se concentran en el análisis del comportamiento
de la caché a partir de trazas o de simulación, o bien en la propuesta de
políticas de reemplazo más elaboradas evaluadas mediante métricas de tasa de
acierto.

La brecha que este proyecto aborda es de naturaleza integradora antes que
algorítmica. No se propone una política de reemplazo nueva, sino la
caracterización empírica del efecto conjunto de tres factores —la caché, el nivel
de concurrencia y la redundancia de instancias— sobre una implementación
construida desde los sockets y evaluada como un sistema completo. La literatura de
caché tiende a aislar el comportamiento del almacén; la literatura de
disponibilidad tiende a tratar el servicio como una caja negra. Medir ambos
aspectos sobre el mismo artefacto, con particular atención al efecto del arranque
en frío tras un evento de falla, constituye el aporte específico de este trabajo.

## Referencias

Bernstein, D. (2014). Containers and cloud: From LXC to Docker to Kubernetes.
IEEE Cloud Computing, 1(3), 81–84. https://doi.org/10.1109/MCC.2014.51

Beyer, B., Jones, C., Petoff, J., y Murphy, N. R. (2016). Site reliability
engineering: How Google runs production systems. O'Reilly Media.

Breslau, L., Cao, P., Fan, L., Phillips, G., y Shenker, S. (1999). Web caching and
Zipf-like distributions: Evidence and implications. En Proceedings of IEEE INFOCOM
'99 (Vol. 1, pp. 126–134). IEEE. https://doi.org/10.1109/INFCOM.1999.749260

Donovan, A. A. A., y Kernighan, B. W. (2016). The Go programming language.
Addison-Wesley.

Fielding, R., Nottingham, M., y Reschke, J. (Eds.). (2022a). HTTP caching (RFC
9111). Internet Engineering Task Force. https://doi.org/10.17487/RFC9111

Fielding, R., Nottingham, M., y Reschke, J. (Eds.). (2022b). HTTP semantics (RFC
9110). Internet Engineering Task Force. https://doi.org/10.17487/RFC9110

Fielding, R., Nottingham, M., y Reschke, J. (Eds.). (2022c). HTTP/1.1 (RFC 9112).
Internet Engineering Task Force. https://doi.org/10.17487/RFC9112

Merkel, D. (2014). Docker: Lightweight Linux containers for consistent development
and deployment. Linux Journal, 2014(239), Artículo 2.

Podlipnig, S., y Böszörmenyi, L. (2003). A survey of web cache replacement
strategies. ACM Computing Surveys, 35(4), 374–398.
https://doi.org/10.1145/954339.954341

Silberschatz, A., Galvin, P. B., y Gagne, G. (2018). Operating system concepts
(10.ª ed.). Wiley.

Stevens, W. R., Fenner, B., y Rudoff, A. M. (2004). UNIX network programming: The
sockets networking API (Vol. 1, 3.ª ed.). Addison-Wesley.

Tanenbaum, A. S., y Van Steen, M. (2023). Distributed systems (4.ª ed.). Maarten
van Steen.

Wang, J. (1999). A survey of web caching schemes for the Internet. ACM SIGCOMM
Computer Communication Review, 29(5), 36–46. https://doi.org/10.1145/505696.505701

Wessels, D. (2001). Web caching. O'Reilly Media.
