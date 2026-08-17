#!/usr/bin/env bash
# E3 · Capacidad de la cache frente a tasa de aciertos, con carga Zipf.
# El catalogo se genera con cadenas de consulta distintas sobre un mismo
# archivo: la clave de cache es metodo + URI completa, de modo que cada
# consulta es una entrada distinta sin crear archivos.
set -uo pipefail
cd "$(dirname "$0")/../.."

PROXY="http://localhost:18080"
ORIGEN="http://origin/1k.txt"
CATALOGO=200
PETICIONES=400
CORRIDAS=3
RES="experiments/resultados"
SALIDA="$RES/e3.csv"

mkdir -p "$RES"
echo "entradas,tasa_aciertos,desalojos,aciertos,fallos,peticiones" > "$SALIDA"

python3 - "$CATALOGO" "$PETICIONES" > /tmp/zipf.txt <<'PY'
import sys, random
cat, n = int(sys.argv[1]), int(sys.argv[2])
random.seed(42)
pesos = [1.0/(i+1) for i in range(cat)]
total = sum(pesos)
acum, c = [], 0.0
for w in pesos:
    c += w/total
    acum.append(c)
for _ in range(n):
    r = random.random()
    lo, hi = 0, cat-1
    while lo < hi:
        mid = (lo+hi)//2
        if acum[mid] < r: lo = mid+1
        else: hi = mid
    print(lo)
PY
echo "secuencia Zipf: $(wc -l < /tmp/zipf.txt) peticiones sobre $CATALOGO recursos"

for CAP in 10 100 1000; do
  echo ""
  echo "=== capacidad = $CAP entradas ==="
  sed -i "s/^CACHE_MAX_ENTRADAS=.*/CACHE_MAX_ENTRADAS=$CAP/" .env
  docker compose up -d --force-recreate --remove-orphans proxy-a proxy-b >/dev/null 2>&1
  sleep 12

  TOT_A=0; TOT_F=0
  for c in $(seq 1 $CORRIDAS); do
    docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
    docker compose restart proxy-a proxy-b >/dev/null 2>&1
    sleep 10
    A=0; F=0
    while read -r i; do
      XC=$(curl -x $PROXY -sD - "$ORIGEN?r=$i" -o /dev/null 2>/dev/null \
           | grep -i "^x-cache" | tr -d '\r' | awk '{print $2}')
      case "$XC" in HIT*) A=$((A+1)) ;; *) F=$((F+1)) ;; esac
    done < /tmp/zipf.txt
    echo "  corrida $c: $A aciertos, $F fallos"
    [ "$c" -gt 1 ] && { TOT_A=$((TOT_A+A)); TOT_F=$((TOT_F+F)); }
  done

  DES=$(docker compose exec -T proxy-a wget -qO- http://localhost:8080/stats 2>/dev/null \
        | grep -o '"l1_desalojos":[0-9]*' | cut -d: -f2)
  TASA=$(awk -v a="$TOT_A" -v f="$TOT_F" 'BEGIN{t=a+f; printf "%.4f", (t>0? a/t : 0)}')
  echo "$CAP,$TASA,${DES:-0},$TOT_A,$TOT_F,$((TOT_A+TOT_F))" >> "$SALIDA"
  echo "  --> tasa de aciertos: $TASA   desalojos: ${DES:-0}"
done

sed -i "s/^CACHE_MAX_ENTRADAS=.*/CACHE_MAX_ENTRADAS=1000/" .env
docker compose up -d --force-recreate --remove-orphans proxy-a proxy-b >/dev/null 2>&1
echo ""
echo "E3 -> $SALIDA"
column -t -s, "$SALIDA"
