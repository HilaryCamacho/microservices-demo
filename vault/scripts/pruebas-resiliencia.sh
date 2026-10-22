#!/usr/bin/env bash
# Paso 11 (pruebas de resiliencia): simula fallos críticos de Vault y comprueba que el diseño los aguanta.
# Para cada escenario: provoca el fallo, lanza el CD y verifica que
#   - el pipeline se detiene ANTES de tocar el clúster (no hay revisión nueva de Helm),
#   - la tienda sigue respondiendo durante el fallo,
#   - el puerto 8200 queda cerrado para el runner,
# y después recupera con los playbooks versionados en Git.
#
# Uso (desde la máquina que inicializó Vault, con kubectl y gh configurados):
#   vault/scripts/pruebas-resiliencia.sh
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
ANSIBLE_DIR="$ROOT/ansible"
SECRETS_DIR="$ANSIBLE_DIR/.secrets"
CA="$ANSIBLE_DIR/.artifacts/vault-ca.pem"
REPORT="$ANSIBLE_DIR/.artifacts/resiliencia-$(date +%Y%m%d-%H%M).md"
NS=online-boutique
RELEASE=online-boutique

VAULT_ADDR="$(terraform -chdir="$ROOT/terraform" output -raw vault_addr)"
VAULT_SG_ID="$(terraform -chdir="$ROOT/terraform" output -raw vault_security_group_id)"
FRONTEND="http://$(kubectl get svc frontend-external -n "$NS" -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')"

# El root token va en un archivo 0600 (no en los argumentos de curl, visibles con ps)
HEADER_FILE="$(mktemp)"
chmod 600 "$HEADER_FILE"
trap 'rm -f "$HEADER_FILE"' EXIT
python3 -c 'import json,sys;print("X-Vault-Token: " + json.load(open(sys.argv[1]))["root_token"])' \
  "$SECRETS_DIR/vault-init.json" > "$HEADER_FILE"

vault_api() { # método ruta [json]
  curl -fsS --cacert "$CA" -H @"$HEADER_FILE" -X "$1" ${3:+-d "$3"} "$VAULT_ADDR/v1/$2"
}

helm_revision() {
  helm history "$RELEASE" -n "$NS" --max 1 -o json | python3 -c 'import json,sys;print(json.load(sys.stdin)[-1]["revision"])'
}

runner_rules() { # reglas temporales del CD que siguen abiertas en el Security Group
  aws ec2 describe-security-group-rules --filters "Name=group-id,Values=$VAULT_SG_ID" \
    --query "length(SecurityGroupRules[?starts_with(Description || '', 'github-actions')])" --output text
}

# Disponibilidad de la tienda: una petición a /cart cada 5 s mientras corre el CD
monitor_start() {
  MONITOR_LOG="$(mktemp)"
  (while :; do
    curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "$FRONTEND/cart" >> "$MONITOR_LOG" || echo 000 >> "$MONITOR_LOG"
    sleep 5
  done) &
  MONITOR_PID=$!
}
monitor_stop() {
  kill "$MONITOR_PID" 2>/dev/null || true
  wait "$MONITOR_PID" 2>/dev/null || true
  echo "$(grep -c '^200$' "$MONITOR_LOG" || true)/$(wc -l < "$MONITOR_LOG" | tr -d ' ')"
  rm -f "$MONITOR_LOG"
}

run_cd() { # lanza el CD desde main y deja RUN_ID, RUN_CONCLUSION y RUN_ERROR
  local since
  since="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  gh workflow run cd.yaml --ref main > /dev/null
  RUN_ID=""
  for _ in $(seq 1 30); do
    RUN_ID="$(gh run list --workflow cd.yaml --event workflow_dispatch --limit 5 --json databaseId,createdAt \
      --jq "[.[] | select(.createdAt >= \"$since\")][0].databaseId // empty")"
    [ -n "$RUN_ID" ] && break
    sleep 5
  done
  [ -n "$RUN_ID" ] || { echo "No se encontró la ejecución del CD" >&2; exit 1; }
  gh run watch "$RUN_ID" --interval 15 > /dev/null 2>&1 || true
  RUN_CONCLUSION="$(gh run view "$RUN_ID" --json conclusion --jq .conclusion)"
  RUN_ERROR="$(gh run view "$RUN_ID" --log-failed 2>/dev/null | grep -m1 -o '##\[error\].*' | sed 's/##\[error\]//' || true)"
}

ROWS=()
scenario() { # nombre función_falla función_recuperación resultado_esperado(success|failure)
  local name="$1" inject="$2" restore="$3" expected="$4"
  echo "==> $name"
  local rev_before rev_after availability open_rules verdict
  rev_before="$(helm_revision)"
  "$inject"
  monitor_start
  run_cd
  availability="$(monitor_stop)"
  rev_after="$(helm_revision)"
  open_rules="$(runner_rules)"
  "$restore"

  verdict="✅"
  [ "$RUN_CONCLUSION" = "$expected" ] || verdict="❌"
  [ "${availability%%/*}" = "${availability##*/}" ] || verdict="❌"
  [ "$open_rules" = "0" ] || verdict="❌"
  if [ "$expected" = "failure" ]; then
    [ "$rev_before" = "$rev_after" ] || verdict="❌"
  else
    [ "$rev_after" -gt "$rev_before" ] || verdict="❌"
  fi
  ROWS+=("| $name | $RUN_CONCLUSION | ${RUN_ERROR:-—} | $rev_before → $rev_after | $availability | $open_rules | $verdict |")
  echo "    CD: $RUN_CONCLUSION · Helm $rev_before → $rev_after · tienda $availability OK · reglas abiertas: $open_rules · $verdict"
}

nothing() { :; }

# --- Escenario 1: el token expira ---------------------------------------------------------
inject_expired_token() {
  vault_api POST auth/token/create-orphan \
    '{"policies":["online-boutique-ci"],"no_default_policy":true,"ttl":"30s","display_name":"prueba-expiracion"}' \
    | python3 -c 'import json,sys;print(json.load(sys.stdin)["auth"]["client_token"],end="")' \
    | gh secret set VAULT_TOKEN
  echo "    Token de 30 s cargado en GitHub; esperando a que expire..."
  sleep 40
}
restore_token() { gh secret set VAULT_TOKEN < "$SECRETS_DIR/ci-token"; }

# --- Escenario 2: alguien cambia la política ACL ------------------------------------------
inject_policy_change() {
  # Simula una "reorganización" de rutas: la política ya no cubre la ruta de la app
  vault_api PUT sys/policies/acl/online-boutique-ci \
    '{"policy":"path \"secret/data/otra-app/*\" { capabilities = [\"read\"] }\npath \"auth/token/lookup-self\" { capabilities = [\"read\"] }"}' > /dev/null
}
restore_policy() { (cd "$ANSIBLE_DIR" && ansible-playbook configure.yml < /dev/null > /dev/null); }

# --- Escenario 3: Vault se sella ----------------------------------------------------------
inject_seal() { vault_api PUT sys/seal > /dev/null; }
restore_unseal() { (cd "$ANSIBLE_DIR" && ansible-playbook init.yml < /dev/null > /dev/null); }

START=$(date +%s)
scenario "0. Línea base (todo sano)" nothing nothing success
scenario "1. Token expirado" inject_expired_token restore_token failure
scenario "2. Política ACL cambiada" inject_policy_change restore_policy failure
scenario "3. Vault sellado" inject_seal restore_unseal failure
scenario "4. Recuperación (todo restaurado)" nothing nothing success

{
  echo "## Pruebas de resiliencia de Vault ($(date '+%Y-%m-%d %H:%M'))"
  echo
  echo "| Escenario | CD | Causa reportada por el pipeline | Revisión de Helm | Tienda (/cart 200) | Reglas 8200 abiertas | Resultado |"
  echo "|---|---|---|---|---|---|---|"
  printf '%s\n' "${ROWS[@]}"
  echo
  echo "Duración total: $(( ($(date +%s) - START) / 60 )) min"
} | tee "$REPORT"

echo
echo "Reporte guardado en $REPORT"
! printf '%s\n' "${ROWS[@]}" | grep -q '❌'
