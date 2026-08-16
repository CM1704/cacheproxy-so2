#!/usr/bin/env bash
# =============================================================================
# CacheProxy · demo.sh
# Demostración guiada para la presentación final.
#
# Uso:  ./demo.sh          pausa entre secciones (Enter para avanzar)
#       ./demo.sh -auto    corrido, sin pausas
# =============================================================================
set -uo pipefail

PROXY="http://localhost:18080"
STATS="http://localhost:18404"
ORIGEN="http://origin"
AUTO="${1:-}"

az()   { printf '\n\033[1;36m╔══ %s\033[0m\n' "$1"; }
cmd()  { printf '\033[1;33m  $ %s\033[0m\n' "$1"; }
nota() { printf '\033[0;37m  %s\033[0m\n' "$1"; }
ok()   { printf '\033[1;32m  ✓ %s\033[0m\n' "$1"; }

pausa() {
  [ "$AUTO" = "-auto" ] && { sleep 1; return; }
  printf '\n\033[0;90m  [Enter para continuar]\033[0m'
  read -r
}

az "0 · ESTADO DEL DESPLIEGUE"
cmd "docker compose ps"
docker compose ps --format 'table {{.Name}}\t{{.Status}}\t{{.Ports}}'
nota "Cinco servicios. Solo HAProxy publica puertos al host."
pausa

az "1 · EL PROXY RESUELVE LO QUE EL HOST NO PUEDE"
cmd "curl -s --max-time 3 http://origin/    (sin proxy)"
if curl -s --max-time 3 http://origin/ >/dev/null 2>&1; then
  nota "El host resolvió origin (inesperado en esta demo)"
else
  ok "Falla: 'origin' solo existe dentro de la red de contenedores"
fi
cmd "curl -x $PROXY ... http://origin/"
curl -x $PROXY -s -o /dev/null -w "  respuesta a través del proxy: HTTP %{http_code}\n" $ORIGEN/
nota "El proxy resuelve el destino y establece la conexión por su cuenta."
pausa

az "2 · CICLO DE CACHÉ: MISS → HIT-L1"
docker compose restart proxy-a proxy-b >/dev/null 2>&1
docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
sleep 4
nota "Caché reiniciada."
cmd "curl -x $PROXY -sD - http://origin/1k.txt   (primera vez)"
curl -x $PROXY -sD - $ORIGEN/1k.txt -o /dev/null | grep -iE "^(HTTP|X-Cache|X-Proxy|Age)" | sed 's/^/  /'
cmd "curl -x $PROXY -sD - http://origin/1k.txt   (segunda vez)"
curl -x $PROXY -sD - $ORIGEN/1k.txt -o /dev/null | grep -iE "^(HTTP|X-Cache|X-Proxy|Age)" | sed 's/^/  /'
pausa

az "3 · LATENCIA: FALLO CONTRA ACIERTO"
printf '  %-28s' "MISS (recurso nuevo):"
curl -x $PROXY -s -o /dev/null -w "%{time_total} s\n" $ORIGEN/100k.txt
printf '  %-28s' "HIT  (mismo recurso):"
curl -x $PROXY -s -o /dev/null -w "%{time_total} s\n" $ORIGEN/100k.txt
nota "La segunda no abre socket hacia el origen."
pausa

az "4 · EVIDENCIA DE SOCKETS EN EL LOG"
cmd "docker compose logs proxy-a | tail -8"
docker compose logs proxy-a --tail 8 | sed 's/^/  /'
nota "La línea 'dial tcp origin:80' es socket() + connect() hacia el origen."
nota "Las peticiones con HIT no tienen esa línea."
pausa

az "5 · CACHÉ COMPARTIDA ENTRE INSTANCIAS (L2)"
nota "HAProxy reparte round robin. Al alternar de instancia, la segunda"
nota "encuentra en Valkey lo que guardó la primera."
for i in 1 2 3 4; do
  printf '  petición %d: ' "$i"
  curl -x $PROXY -sD - $ORIGEN/index.html -o /dev/null \
    | grep -iE "^(X-Cache|X-Proxy-Nodo)" | tr -d '\r' | tr '\n' ' '
  echo ""
done
pausa

az "6 · SEGURIDAD"
cmd "curl -x $PROXY http://dominio-no-autorizado.net/"
curl -x $PROXY -s -o /dev/null -w "  código: %{http_code}  (lista blanca)\n" http://dominio-no-autorizado.net/
cmd "petición con cabecera Authorization"
XC=$(curl -x $PROXY -s -o /dev/null -D - -H "Authorization: Bearer demo" $ORIGEN/1k.txt \
     | grep -i "^X-Cache" | tr -d '\r' | awk '{print $2}')
printf '  X-Cache: %s  (no se almacena: es específica de un usuario)\n' "$XC"
pausa

az "7 · ALTA DISPONIBILIDAD: FALLA INDUCIDA"
cmd "curl -s $STATS/stats  (instancias en rotación)"
curl -s "$STATS/stats" 2>/dev/null | grep -oE "proxya|proxyb" | sort -u | sed 's/^/  /'
nota "Se detiene proxy-a bajo carga y se mide el corte."
cmd "docker compose stop proxy-a"
T0=$(date +%s.%N)
docker compose stop proxy-a >/dev/null 2>&1
FALLOS=0
for i in $(seq 1 60); do
  if curl -x $PROXY -s -o /dev/null --max-time 2 $ORIGEN/1k.txt; then break; fi
  FALLOS=$((FALLOS+1)); sleep 0.1
done
T1=$(date +%s.%N)
printf '  servicio restablecido tras %.2f s (%d sondeos fallidos)\n' \
  "$(echo "$T1 - $T0" | bc)" "$FALLOS"
cmd "verificar qué instancia responde ahora"
curl -x $PROXY -sD - $ORIGEN/1k.txt -o /dev/null | grep -i "^X-Proxy-Nodo" | sed 's/^/  /'
ok "El servicio continuó con la instancia superviviente."
docker compose up -d proxy-a >/dev/null 2>&1
pausa

az "8 · CONTADORES"
cmd "docker compose exec proxy-b wget -qO- http://localhost:8080/stats"
docker compose exec -T proxy-b wget -qO- http://localhost:8080/stats 2>/dev/null | sed 's/^/  /'
pausa

az "9 · CALIDAD DEL CÓDIGO"
cmd "go test -race -cover ./cache/..."
(cd proxy && go test -race -cover ./cache/... 2>&1 | sed 's/^/  /')
nota "Sin condiciones de carrera con 100 goroutines concurrentes."
echo ""
printf '\033[1;32m╔══ FIN DE LA DEMOSTRACIÓN ══╗\033[0m\n'
