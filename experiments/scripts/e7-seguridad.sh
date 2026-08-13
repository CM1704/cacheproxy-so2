#!/usr/bin/env bash
# E7: verificación de las restricciones de seguridad.
set -uo pipefail
source "$(dirname "$0")/comun.sh"
SALIDA="$RES/e7.csv"
echo "prueba,esperado,obtenido,resultado" > "$SALIDA"

verificar() {
  local nombre="$1" esperado="$2" obtenido="$3"
  local r="FALLA"; [ "$esperado" = "$obtenido" ] && r="OK"
  echo "$nombre,$esperado,$obtenido,$r" >> "$SALIDA"
  printf "  %-38s esperado=%s obtenido=%s  %s\n" "$nombre" "$esperado" "$obtenido" "$r"
}

reiniciar_cache
cod=$(curl -x $PROXY -s -o /dev/null -w "%{http_code}" http://dominio-no-autorizado.net/)
verificar "dominio fuera de la lista blanca" "403" "$cod"

cod=$(curl -x $PROXY -s -o /dev/null -w "%{http_code}" $ORIGEN/1k.txt)
verificar "dominio autorizado" "200" "$cod"

curl -x $PROXY -s -o /dev/null -H "Authorization: Bearer prueba" $ORIGEN/1k.txt
xc=$(curl -x $PROXY -s -o /dev/null -H "Authorization: Bearer prueba" -D - $ORIGEN/1k.txt | grep -i "^X-Cache" | tr -d '\r' | awk '{print $2}')
verificar "peticion con Authorization no se cachea" "MISS" "$xc"

echo "E7 -> $SALIDA"
