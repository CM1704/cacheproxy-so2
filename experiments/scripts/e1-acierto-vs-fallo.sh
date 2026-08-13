#!/usr/bin/env bash
# E1: tiempo de respuesta con acierto frente a fallo de caché.
#
# Control metodológico: se repite con Connection: close para separar el
# efecto de la caché del de la reutilización de conexiones persistentes.
set -uo pipefail
source "$(dirname "$0")/comun.sh"
SALIDA="$RES/e1.csv"
echo "corrida,modo,conexion,latencia_s" > "$SALIDA"

for c in $(seq 1 $CORRIDAS); do
  for conexion in keepalive close; do
    EXTRA=""
    [ "$conexion" = "close" ] && EXTRA="-H Connection:close"
    reiniciar_cache
    # Primera petición: caché fría
    t=$(curl -x $PROXY $EXTRA -s -o /dev/null -w "%{time_total}" $ORIGEN/1k.txt)
    echo "$c,MISS,$conexion,$t" >> "$SALIDA"
    # Siguientes: caché caliente
    for i in $(seq 1 20); do
      t=$(curl -x $PROXY $EXTRA -s -o /dev/null -w "%{time_total}" $ORIGEN/1k.txt)
      echo "$c,HIT,$conexion,$t" >> "$SALIDA"
    done
  done
done
echo "E1 -> $SALIDA"
awk -F, 'NR>1 {k=$2"-"$3; s[k]+=$4; n[k]++} END {for(i in s) printf "  %-14s media %.4f s (n=%d)\n", i, s[i]/n[i], n[i]}' "$SALIDA"
