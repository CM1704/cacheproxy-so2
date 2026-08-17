#!/usr/bin/env bash
set -uo pipefail
cd "$(dirname "$0")"

OK=0; FALLA=0; AVISO=0

sec()  { printf '\n\033[1;36m══ %s\033[0m\n' "$1"; }
ok()   { printf '  \033[1;32m[ OK ]\033[0m %s\n' "$1"; OK=$((OK+1)); }
mal()  { printf '  \033[1;31m[FALLA]\033[0m %s\n' "$1"; FALLA=$((FALLA+1)); }
avi()  { printf '  \033[1;33m[AVISO]\033[0m %s\n' "$1"; AVISO=$((AVISO+1)); }

archivo() { [ -f "$1" ] && ok "$1" || mal "FALTA $1"; }
contiene() {
  if [ -f "$1" ] && grep -q "$2" "$1" 2>/dev/null; then ok "$3"; else mal "$3"; fi
}
etiqueta() { git tag 2>/dev/null | grep -qx "$1" && ok "etiqueta $1" || mal "FALTA etiqueta $1"; }

sec "SEMANA 11 · Repositorio y documentación base"
archivo LICENSE
contiene LICENSE "MIT License" "LICENSE es la licencia MIT"
archivo .gitignore
archivo README.md
archivo .env
contiene .env "COMPOSE_PROJECT_NAME=cacheproxy" ".env aísla el proyecto de Compose"
contiene .env "PUERTO_PROXY=18080" ".env define PUERTO_PROXY=18080"
contiene .env "PUERTO_STATS=18404" ".env define PUERTO_STATS=18404"
archivo docs/01-propuesta.md
archivo docs/referencias.bib
archivo docs/entregas/avance.docx
etiqueta v0.1-semana11

sec "SEMANA 12 · Proxy mínimo y contenerización"
archivo proxy/go.mod
archivo proxy/main.go
archivo proxy/Dockerfile
archivo docker-compose.yml
archivo docs/02-plan-metodologia.md
archivo deploy/origin/html/index.html
archivo deploy/origin/html/1k.txt
archivo deploy/origin/html/100k.txt
contiene proxy/main.go "net.Listen" "main.go abre el socket con net.Listen"
contiene proxy/main.go "DialContext" "main.go registra las conexiones salientes"
contiene proxy/main.go "saltoASalto" "main.go elimina cabeceras de salto a salto"
etiqueta v0.2-semana12

sec "SEMANA 13 · Caché LRU y marco teórico"
archivo proxy/cache/lru.go
archivo proxy/cache/politica.go
archivo proxy/cache/lru_test.go
archivo docs/03-marco-teorico.md
contiene proxy/cache/lru.go "container/list" "la caché usa lista doblemente enlazada"
contiene proxy/cache/lru.go "sync.Mutex" "la caché protege el acceso concurrente"
contiene proxy/cache/politica.go "Set-Cookie" "la política excluye respuestas con Set-Cookie"
contiene proxy/cache/politica.go "s-maxage" "la política respeta s-maxage"
etiqueta v0.3-semana13

sec "SEMANA 14 · CONNECT, L2, seguridad y alta disponibilidad"
archivo proxy/cache/valkey.go
archivo proxy/connect.go
archivo proxy/security/politica.go
archivo deploy/haproxy/haproxy.cfg
contiene proxy/connect.go "Hijacker" "el túnel toma control del socket"
contiene proxy/connect.go "io.Copy" "el túnel retransmite bytes"
contiene deploy/haproxy/haproxy.cfg "/healthz" "HAProxy comprueba salud contra /healthz"
contiene deploy/haproxy/haproxy.cfg "proxy-a:8080" "HAProxy conoce proxy-a"
contiene deploy/haproxy/haproxy.cfg "proxy-b:8080" "HAProxy conoce proxy-b"

printf '\n  \033[1;35m--- Integración en main.go (causa habitual de fallo) ---\033[0m\n'
contiene proxy/main.go "HIT-L1" "main.go marca HIT-L1"
contiene proxy/main.go "HIT-L2" "main.go marca HIT-L2"
contiene proxy/main.go "VALKEY_ADDR" "main.go lee VALKEY_ADDR"
contiene proxy/main.go "cache.NuevaCompartida" "main.go instancia la caché compartida"
contiene proxy/main.go "tunel(w, r" "main.go enruta CONNECT al túnel"
contiene proxy/main.go "security.NuevaPolitica" "main.go aplica la política de seguridad"
contiene proxy/main.go "politica.Permitido" "main.go verifica la lista blanca"
contiene proxy/main.go "MethodConnect" "main.go atiende CONNECT antes del enrutador"

printf '\n  \033[1;35m--- Servicios en docker-compose.yml ---\033[0m\n'
for s in origin valkey proxy-a proxy-b haproxy; do
  contiene docker-compose.yml "^  $s:" "servicio $s definido"
done
archivo experiments/scripts/e7-seguridad.sh
etiqueta v0.9-semana14

sec "SEMANA 15 · Entrega final"
archivo docs/04-resultados.md
archivo demo-env.sh
contiene README.md "18080" "README documenta los puertos"
git tag 2>/dev/null | grep -qx "v1.0.0" && ok "etiqueta v1.0.0" || avi "etiqueta v1.0.0 pendiente"

sec "CÓDIGO · Compilación y pruebas"
if (cd proxy && gofmt -l . | grep -q .); then avi "hay archivos sin formato canónico"; else ok "gofmt limpio"; fi
if (cd proxy && go vet ./... >/dev/null 2>&1); then ok "go vet sin observaciones"; else mal "go vet reporta problemas"; fi
if (cd proxy && go build -o /tmp/cp-verif . >/dev/null 2>&1); then ok "el proyecto compila"; else mal "EL PROYECTO NO COMPILA"; fi
if (cd proxy && go test ./cache/... >/dev/null 2>&1); then ok "pruebas unitarias pasan"; else mal "pruebas unitarias fallan"; fi
if (cd proxy && go test -race ./cache/... >/dev/null 2>&1); then ok "sin condiciones de carrera"; else mal "el detector de carrera reporta problemas"; fi

sec "GIT · Estado del repositorio"
[ -z "$(git status --porcelain)" ] && ok "sin cambios sin confirmar" || avi "hay cambios sin confirmar"
git remote -v 2>/dev/null | grep -q github && ok "remoto de GitHub configurado" || mal "sin remoto de GitHub"
LOCAL=$(git rev-parse @ 2>/dev/null); REMOTO=$(git rev-parse @{u} 2>/dev/null || echo x)
[ "$LOCAL" = "$REMOTO" ] && ok "sincronizado con el remoto" || avi "hay commits sin publicar"
printf '  etiquetas: %s\n' "$(git tag | tr '\n' ' ')"

sec "EJECUCIÓN · Estado de los contenedores"
N=$(docker compose ps --status running -q 2>/dev/null | wc -l)
[ "$N" -eq 5 ] && ok "5 servicios en ejecución" || mal "hay $N servicios en ejecución (se esperan 5)"

if docker compose logs proxy-a 2>/dev/null | grep -q "cache compartida conectada"; then
  ok "proxy-a conectado a la caché compartida"
else
  mal "proxy-a SIN caché compartida: la imagen es vieja o falta la integración"
fi
if docker compose logs proxy-a 2>/dev/null | grep -q "dominios permitidos"; then
  ok "lista blanca activa"
else
  mal "lista blanca no aplicada"
fi

sec "EJECUCIÓN · Comportamiento del proxy"
P=http://localhost:18080
O=http://origin

COD=$(curl -x $P -s -o /dev/null -w "%{http_code}" $O/ 2>/dev/null)
[ "$COD" = "200" ] && ok "el proxy responde 200" || mal "el proxy respondió $COD"

docker compose exec -T valkey valkey-cli FLUSHALL >/dev/null 2>&1
docker compose restart proxy-a proxy-b >/dev/null 2>&1
sleep 6

X1=$(curl -x $P -sD - $O/1k.txt -o /dev/null 2>/dev/null | grep -i "^x-cache" | tr -d '\r' | awk '{print $2}')
X2=$(curl -x $P -sD - $O/1k.txt -o /dev/null 2>/dev/null | grep -i "^x-cache" | tr -d '\r' | awk '{print $2}')
printf '  primera petición: %s   segunda: %s\n' "$X1" "$X2"
[ "$X1" = "MISS" ] && ok "la primera petición es MISS" || mal "la primera petición dio $X1"
case "$X2" in
  HIT-L1|HIT-L2) ok "la segunda petición acierta ($X2)" ;;
  HIT)           mal "devuelve 'HIT' sin nivel: código de la semana 13, falta reconstruir" ;;
  *)             mal "la segunda petición dio $X2" ;;
esac

COD=$(curl -x $P -s -o /dev/null -w "%{http_code}" http://dominio-no-autorizado.net/ 2>/dev/null)
[ "$COD" = "403" ] && ok "lista blanca bloquea destinos no autorizados" || mal "destino no autorizado devolvió $COD"

XA=$(curl -x $P -sD - -H "Authorization: Bearer x" $O/1k.txt -o /dev/null 2>/dev/null | grep -i "^x-cache" | tr -d '\r' | awk '{print $2}')
[ "$XA" = "MISS" ] && ok "peticiones autenticadas no se sirven de caché" || mal "petición con Authorization dio $XA"

S=$(docker compose exec -T proxy-a wget -qO- http://localhost:8080/stats 2>/dev/null)
echo "$S" | grep -q "l2_aciertos" && ok "/stats expone contadores de L2" || mal "/stats sin contadores de L2"

docker compose exec -T proxy-a wget -qO- http://localhost:8080/healthz 2>/dev/null | grep -q ok \
  && ok "/healthz responde" || mal "/healthz no responde"

curl -s http://localhost:18404/stats 2>/dev/null | grep -q proxya \
  && ok "HAProxy tiene las instancias en rotación" || mal "HAProxy no reporta las instancias"

TUNEL=$(python3 - <<'PY' 2>/dev/null
import socket
try:
    s = socket.create_connection(("127.0.0.1", 18080), timeout=5)
    s.sendall(b"CONNECT origin:80 HTTP/1.1\r\nHost: origin:80\r\n\r\n")
    r = s.recv(100); s.close()
    print("OK" if b"200" in r else "FALLA")
except Exception:
    print("FALLA")
PY
)
[ "$TUNEL" = "OK" ] && ok "el túnel CONNECT se establece" || mal "el túnel CONNECT no responde 200"

printf '\n\033[1;36m══ RESUMEN\033[0m\n'
printf '  \033[1;32mOK: %d\033[0m   \033[1;31mFALLAS: %d\033[0m   \033[1;33mAVISOS: %d\033[0m\n' "$OK" "$FALLA" "$AVISO"
if [ "$FALLA" -eq 0 ]; then
  printf '\n  \033[1;32mTodo verificado. El proyecto está listo para entregar.\033[0m\n'
else
  printf '\n  \033[1;31mHay %d fallas. Revisá las líneas marcadas arriba.\033[0m\n' "$FALLA"
fi
