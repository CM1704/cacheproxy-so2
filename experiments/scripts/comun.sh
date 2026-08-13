#!/usr/bin/env bash
# Funciones compartidas por los escenarios.
PROXY="http://localhost:18080"
ORIGEN="http://origin"
RES="$(cd "$(dirname "${BASH_SOURCE[0]}")/../resultados" && pwd)"
CORRIDAS=5

reiniciar_cache() {
  docker compose restart proxy-a proxy-b >/dev/null 2>&1
  docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
  sleep 4
}

retardo_origen() {
  docker compose exec -T origin tc qdisc del dev eth0 root 2>/dev/null
  [ "$1" != "0" ] && docker compose exec -T origin tc qdisc add dev eth0 root netem delay "$1"ms
  return 0
}

percentiles() {  # lee latencias por stdin, imprime n,media,p50,p95,p99
  sort -n | awk '{v[NR]=$1; s+=$1}
    END { if(NR==0){print "0,0,0,0,0"; exit}
      printf "%d,%.4f,%.4f,%.4f,%.4f\n", NR, s/NR,
        v[int(NR*0.50)+((NR*0.50)==int(NR*0.50)?0:1)],
        v[int(NR*0.95)+((NR*0.95)==int(NR*0.95)?0:1)],
        v[int(NR*0.99)+((NR*0.99)==int(NR*0.99)?0:1)] }'
}
