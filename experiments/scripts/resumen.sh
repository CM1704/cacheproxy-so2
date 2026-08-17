#!/usr/bin/env bash
set -uo pipefail
RES="$(cd "$(dirname "${BASH_SOURCE[0]}")/../resultados" && pwd)"

titulo() { printf '\n### %s\n\n' "$1"; }
falta()  { printf '_Escenario no ejecutado._\n'; }

titulo "E1 · Acierto frente a fallo de caché"
if [ -f "$RES/e1.csv" ]; then
  echo "| Modo | Conexión | n | Media (s) |"
  echo "|---|---|---|---|"
  for modo in MISS HIT; do
    for con in keepalive close; do
      awk -F, -v m="$modo" -v c="$con" 'NR>1 && $2==m && $3==c {s+=$4; n++}
        END {if(n==0) printf "| %s | %s | 0 | — |\n", m, c;
             else printf "| %s | %s | %d | %.4f |\n", m, c, n, s/n}' "$RES/e1.csv"
    done
  done
else falta; fi

titulo "E2 · Escalabilidad ante concurrencia"
if [ -f "$RES/e2.csv" ]; then
  echo "| Clientes | Rendimiento (req/s) | p50 (s) | p95 (s) | p99 (s) |"
  echo "|---|---|---|---|---|"
  for cl in 1 10 50 100; do
    awk -F, -v c="$cl" 'NR>1 && $2==c {r+=$3; a+=$4; b+=$5; d+=$6; n++}
      END {if(n>0) printf "| %d | %.1f | %.4f | %.4f | %.4f |\n", c, r/n, a/n, b/n, d/n;
           else printf "| %d | — | — | — | — |\n", c}' "$RES/e2.csv"
  done
  echo ""
  awk -F, 'NR>1 && $2==1 {p1+=$5; n1++} NR>1 && $2==100 {p100+=$5; n100++}
    END {if(n1>0 && n100>0) printf "Factor de degradación de p95 entre 1 y 100 clientes: **%.2f×**\n", (p100/n100)/(p1/n1)}' "$RES/e2.csv"
else falta; fi

titulo "E5 · Falla de instancia"
if [ -f "$RES/e5.csv" ]; then
  echo "| Corrida | Peticiones | Errores | Inactividad (s) |"
  echo "|---|---|---|---|"
  awk -F, 'NR>1 {printf "| %s | %s | %s | %s |\n", $1, $2, $3, $5}' "$RES/e5.csv"
  echo ""
  awk -F, 'NR>1 {s+=$5; n++} END {if(n>0) printf "Inactividad media: **%.2f s** (n=%d)\n", s/n, n}' "$RES/e5.csv"
else falta; fi

titulo "E7 · Restricciones de seguridad"
if [ -f "$RES/e7.csv" ]; then
  echo "| Prueba | Esperado | Obtenido | Resultado |"
  echo "|---|---|---|---|"
  awk -F, 'NR>1 {printf "| %s | %s | %s | %s |\n", $1, $2, $3, $4}' "$RES/e7.csv"
else falta; fi

printf '\n---\nArchivos en %s:\n' "$RES"
ls -1 "$RES" 2>/dev/null | sed 's/^/  /' || echo "  (vacío)"
