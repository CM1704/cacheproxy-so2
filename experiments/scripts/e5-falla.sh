#!/usr/bin/env bash
# E5: tiempo de inactividad y errores ante la falla de una instancia.
set -uo pipefail
source "$(dirname "$0")/comun.sh"
SALIDA="$RES/e5.csv"
echo "corrida,total,errores,pct_error,inactividad_s" > "$SALIDA"

for c in $(seq 1 $CORRIDAS); do
  docker compose up -d proxy-a >/dev/null 2>&1
  reiniciar_cache
  ( hey -z 30s -c 20 -x $PROXY $ORIGEN/1k.txt > "$RES/e5_carga_r${c}.txt" 2>&1 ) &
  CARGA=$!
  sleep 10
  T0=$(date +%s.%N)
  docker compose stop proxy-a >/dev/null 2>&1
  # Sondeo hasta que el servicio vuelva a responder
  while ! curl -x $PROXY -s -o /dev/null --max-time 2 $ORIGEN/1k.txt; do sleep 0.1; done
  T1=$(date +%s.%N)
  wait $CARGA
  INACT=$(echo "$T1 - $T0" | bc)
  TOT=$(awk '/Total:/{print $2; exit}' "$RES/e5_carga_r${c}.txt")
  ERR=$(awk '/Error distribution/,0' "$RES/e5_carga_r${c}.txt" | grep -cE "^\s+\[" || echo 0)
  echo "$c,$TOT,$ERR,,$INACT" >> "$SALIDA"
  echo "  corrida $c: inactividad ${INACT}s"
  docker compose up -d proxy-a >/dev/null 2>&1
  sleep 5
done
echo "E5 -> $SALIDA"
