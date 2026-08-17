#!/usr/bin/env bash
# E4 · Sensibilidad del beneficio de la cache a la latencia del origen.
set -uo pipefail
cd "$(dirname "$0")/../.."

PROXY="http://localhost:18080"
ORIGEN="http://origin/1k.txt"
CORRIDAS=5
RES="experiments/resultados"
SALIDA="$RES/e4.csv"
mkdir -p "$RES"

echo "=== comprobando disponibilidad de tc netem ==="
docker compose exec -T origin sh -c 'command -v tc >/dev/null || apk add --no-cache iproute2 >/dev/null 2>&1' 2>/dev/null
if ! docker compose exec -T origin tc qdisc show dev eth0 >/dev/null 2>&1; then
  echo "NO DISPONIBLE: el contenedor del origen no puede usar tc."
  echo "Documentar E4 como no ejecutado por indisponibilidad de sch_netem."
  exit 1
fi
echo "tc disponible."

echo "retardo_ms,corrida,miss_s,hit_s" > "$SALIDA"

for D in 0 50 200; do
  echo ""
  echo "=== retardo del origen: ${D} ms ==="
  docker compose exec -T origin tc qdisc del dev eth0 root >/dev/null 2>&1
  if [ "$D" != "0" ]; then
    docker compose exec -T origin tc qdisc add dev eth0 root netem delay "${D}ms" >/dev/null 2>&1 \
      || { echo "  no se pudo aplicar el retardo"; continue; }
  fi
  for c in $(seq 1 $CORRIDAS); do
    docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
    docker compose restart proxy-a proxy-b >/dev/null 2>&1
    sleep 10
    U="$ORIGEN?d=$D&c=$c"
    M=$(curl -x $PROXY -s -o /dev/null -w "%{time_total}" "$U")
    curl -x $PROXY -s -o /dev/null "$U"
    H=$(curl -x $PROXY -s -o /dev/null -w "%{time_total}" "$U")
    echo "$D,$c,$M,$H" >> "$SALIDA"
    echo "  corrida $c: MISS ${M}s  HIT ${H}s"
  done
done

docker compose exec -T origin tc qdisc del dev eth0 root >/dev/null 2>&1
echo ""
echo "E4 -> $SALIDA"
awk -F, 'NR>1 {m[$1]+=$3; h[$1]+=$4; n[$1]++}
  END {printf "\n| Retardo | MISS (s) | HIT (s) | Reducción |\n|---|---|---|---|\n";
       for (d in n) printf "| %s ms | %.4f | %.4f | %.1f %% |\n", d, m[d]/n[d], h[d]/n[d],
         (m[d]/n[d] > 0 ? 100*(1-(h[d]/n[d])/(m[d]/n[d])) : 0)}' "$SALIDA"
