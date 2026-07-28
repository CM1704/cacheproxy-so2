# Semana 11 — Definición del tema y objetivos

> El documento formal en Word está en `docs/entregas/avance.docx`.

## Tema

Tema asignado (c) del enunciado: **Proxy HTTP con cache**.
Título: *CacheProxy: diseño, implementación y evaluación de un proxy HTTP con
caché, concurrente y de alta disponibilidad*.

## Pregunta de investigación

¿En qué medida un servidor proxy HTTP concurrente con caché, implementado sobre
sockets TCP y desplegado en alta disponibilidad con contenedores, reduce el
tiempo de respuesta y sostiene la continuidad del servicio ante el incremento de
clientes concurrentes y ante la falla de una de sus instancias?

## Objetivo general

Diseñar, implementar y evaluar experimentalmente un servidor proxy HTTP
concurrente con caché, desplegado en contenedores y en configuración de alta
disponibilidad, determinando su impacto sobre el tiempo de respuesta, su
comportamiento ante el aumento de clientes simultáneos y su continuidad de
servicio ante la falla de una de sus instancias.

## Objetivos específicos

1. Implementar un proxy HTTP/1.1 sobre sockets TCP, con soporte de CONNECT.
2. Desarrollar una caché en memoria con reemplazo LRU y expiración por TTL,
   conforme al RFC 9111.
3. Garantizar atención concurrente con acceso sincronizado a la caché,
   verificado con el detector de carrera de Go.
4. Cuantificar la mejora de latencia bajo cargas de 1 a 100 clientes.
5. Evaluar la alta disponibilidad midiendo el tiempo de inactividad ante falla.
6. Proponer mejoras a partir de los resultados.

## Estado

- [x] Tema definido
- [x] Objetivos redactados
- [x] Alcance delimitado
- [x] Repositorio inicializado
- [x] Aprobación del profesor
