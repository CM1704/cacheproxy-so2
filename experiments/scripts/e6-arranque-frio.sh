#!/usr/bin/env bash
# E6 · Efecto de la cache compartida sobre el arranque en frio tras una falla.
# Se detiene proxy-b durante el calentamiento para que todas las entradas
# queden en proxy-a. Luego entra proxy-b, sale proxy-a, y se mide que
# proporcion de las mismas peticiones acierta.
set -uo pipefail
cd "$(dirname "$0")/../.."

PROXY="http://localhost:18080"
ORIGEN="http://origin/1k.txt"
RECURSOS=40
CORRIDAS=3
RES="experiments/resultados"
SALIDA="$RES/e6.csv"
mkdir -p "$RES"
echo "configuracion,tasa_aciertos,aciertos,fallos" > "$SALIDA"

corrida() {
  docker compose stop proxy-b >/dev/null 2>&1
  docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
  docker compose restart proxy-a >/dev/null 2>&1
  sleep 10
  for i in $(seq 1 $RECURSOS); do
    curl -x $PROXY -s -o /dev/null "$ORIGEN?e6=$i" 2>/dev/null
  done
  docker compose start proxy-b >/dev/null 2>&1
  sleep 12
  docker compose stop proxy-a >/dev/null 2>&1
  sleep 4
  A=0; F=0
  for i in $(seq 1 $RECURSOS); do
    XC=$(curl -x $PROXY -sD - "$ORIGEN?e6=$i" -o /dev/null 2>/dev/null \
         | grep -i "^x-cache" | tr -d '\r' | awk '{print $2}')
    case "$XC" in HIT*) A=$((A+1)) ;; *) F=$((F+1)) ;; esac
  done
  docker compose start proxy-a >/dev/null 2>&1
  sleep 8
  echo "$A $F"
}

echo "=== CON cache compartida (L2 activa) ==="
TA=0; TF=0
for c in $(seq 1 $CORRIDAS); do
  read -r A F <<< "$(corrida)"
  echo "  corrida $c: $A aciertos, $F fallos"
  TA=$((TA+A)); TF=$((TF+F))
done
echo "con cache compartida,$(awk -v a=$TA -v f=$TF 'BEGIN{printf "%.4f", (a+f>0? a/(a+f):0)}'),$TA,$TF" >> "$SALIDA"

echo ""
echo "=== SIN cache compartida (solo L1) ==="
sed -i 's|VALKEY_ADDR: "valkey:6379"|VALKEY_ADDR: ""|g' docker-compose.yml
docker compose up -d --force-recreate --remove-orphans proxy-a proxy-b >/dev/null 2>&1
sleep 12
TA=0; TF=0
for c in $(seq 1 $CORRIDAS); do
  read -r A F <<< "$(corrida)"
  echo "  corrida $c: $A aciertos, $F fallos"
  TA=$((TA+A)); TF=$((TF+F))
done
echo "solo cache local,$(awk -v a=$TA -v f=$TF 'BEGIN{printf "%.4f", (a+f>0? a/(a+f):0)}'),$TA,$TF" >> "$SALIDA"

sed -i 's|VALKEY_ADDR: ""|VALKEY_ADDR: "valkey:6379"|g' docker-compose.yml
docker compose up -d --force-recreate --remove-orphans >/dev/null 2>&1
sleep 12

echo ""
echo "E6 -> $SALIDA"
column -t -s, "$SALIDA"
