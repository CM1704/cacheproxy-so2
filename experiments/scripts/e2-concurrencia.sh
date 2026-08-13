#!/usr/bin/env bash
# E2: escalabilidad ante clientes concurrentes.
set -uo pipefail
source "$(dirname "$0")/comun.sh"
SALIDA="$RES/e2.csv"
echo "corrida,clientes,rps,p50_s,p95_s,p99_s" > "$SALIDA"

for c in $(seq 1 $CORRIDAS); do
  for cl in 1 10 50 100; do
    reiniciar_cache
    curl -x $PROXY -s -o /dev/null $ORIGEN/1k.txt   # calentar
    out=$(hey -n 2000 -c $cl -x $PROXY $ORIGEN/1k.txt 2>/dev/null)
    echo "$out" > "$RES/e2_c${cl}_r${c}.txt"
    rps=$(echo "$out" | awk '/Requests\/sec/{print $2}')
    p50=$(echo "$out" | awk '/50% in/{print $3}')
    p95=$(echo "$out" | awk '/95% in/{print $3}')
    p99=$(echo "$out" | awk '/99% in/{print $3}')
    echo "$c,$cl,$rps,$p50,$p95,$p99" >> "$SALIDA"
    echo "  corrida $c, $cl clientes: $rps req/s, p95=$p95 s"
  done
done
echo "E2 -> $SALIDA"
