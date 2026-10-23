#!/usr/bin/env bash
# Paso 11 (pruebas de resiliencia): simula fallos críticos de Vault y comprueba que el diseño los aguanta.
# Para cada escenario: provoca el fallo, lanza el CD y verifica que
#   - el pipeline se detiene ANTES de tocar el clúster (no hay revisión nueva de Helm),
#   - la tienda sigue respondiendo durante el fallo (medido desde un pod dentro del clúster),
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
export AWS_REGION="${AWS_REGION:-us-east-2}"

VAULT_ADDR="$(terraform -chdir="$ROOT/terraform" output -raw vault_addr)"
VAULT_SG_ID="$(terraform -chdir="$ROOT/terraform" output -raw vault_security_group_id)"

# Reintenta un comando ante cortes de red de la máquina local (DNS, API de GitHub o AWS)
retry() {
  local attempt
  for attempt in 1 2 3 4 5 6; do
    "$@" && return 0
    echo "    (falla de red, reintento $attempt/6)" >&2
    sleep 10
  done
  return 1
}

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
  retry helm history "$RELEASE" -n "$NS" --max 1 -o json | python3 -c 'import json,sys;print(json.load(sys.stdin)[-1]["revision"])'
}

runner_rules() { # reglas temporales del CD que siguen abiertas en el Security Group
  retry aws ec2 describe-security-group-rules --filters "Name=group-id,Values=$VAULT_SG_ID" \
    --query "length(SecurityGroupRules[?starts_with(Description || '', 'github-actions')])" --output text
}

# Disponibilidad de la tienda medida DENTRO del clúster (no depende de la red local):
# un pod consulta http://frontend/cart cada 5 s mientras corre el CD.
monitor_start() {
  MONITOR_POD="monitor-resiliencia-$(date +%s)"
  retry kubectl run "$MONITOR_POD" -n "$NS" --image=curlimages/curl:8.11.1 --restart=Never --quiet --command -- \
    sh -c 'while true; do curl -s -o /dev/null -w "%{http_code}\n" --max-time 5 http://frontend/cart; sleep 5; done' > /dev/null
  retry kubectl wait -n "$NS" --for=condition=Ready "pod/$MONITOR_POD" --timeout=120s > /dev/null
}
monitor_stop() { # deja MONITOR_RESULT (200/total) y MONITOR_DETAIL (conteo por código HTTP)
  local codes
  codes="$(retry kubectl logs -n "$NS" "$MONITOR_POD")"
  retry kubectl delete pod -n "$NS" "$MONITOR_POD" --wait=false > /dev/null || true
  MONITOR_RESULT="$(grep -c '^200$' <<< "$codes" || true)/$(grep -c . <<< "$codes" || true)"
  MONITOR_DETAIL="$(sort <<< "$codes" | uniq -c | awk '{printf "%s×%s ", $1, $2}')"
}

run_cd() { # lanza el CD desde main y deja RUN_ID, RUN_CONCLUSION y RUN_ERROR
  local since
  since="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
  retry gh workflow run cd.yaml --ref main > /dev/null
  RUN_ID=""
  for _ in $(seq 1 30); do
    RUN_ID="$(gh run list --workflow cd.yaml --event workflow_dispatch --limit 5 --json databaseId,createdAt \
      --jq "[.[] | select(.createdAt >= \"$since\")][0].databaseId // empty" 2> /dev/null || true)"
    [ -n "$RUN_ID" ] && break
    sleep 5
  done
  [ -n "$RUN_ID" ] || { echo "No se encontró la ejecución del CD" >&2; exit 1; }
  # Espera a que termine; los cortes de red durante la espera no cuentan como resultado
  until [ "$(gh run view "$RUN_ID" --json status --jq .status 2> /dev/null || true)" = "completed" ]; do
    sleep 15
  done
  RUN_CONCLUSION="$(retry gh run view "$RUN_ID" --json conclusion --jq .conclusion)"
  RUN_ERROR="$(retry gh run view "$RUN_ID" --log-failed 2> /dev/null | grep -m1 -o '##\[error\].*' | sed 's/##\[error\]//' || true)"
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
  monitor_stop
  availability="$MONITOR_RESULT"
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
  ROWS+=("| $name | $RUN_CONCLUSION | ${RUN_ERROR:-—} | $rev_before → $rev_after | $availability (${MONITOR_DETAIL% }) | $open_rules | $verdict |")
  echo "    CD: $RUN_CONCLUSION · Helm $rev_before → $rev_after · tienda $availability OK (${MONITOR_DETAIL% }) · reglas abiertas: $open_rules · $verdict"
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
set_ci_token() { gh secret set VAULT_TOKEN < "$SECRETS_DIR/ci-token"; }
restore_token() { retry set_ci_token; }

# --- Escenario 2: alguien cambia la política ACL ------------------------------------------
inject_policy_change() {
  # Simula una "reorganización" de rutas: la política ya no cubre la ruta de la app
  vault_api PUT sys/policies/acl/online-boutique-ci \
    '{"policy":"path \"secret/data/otra-app/*\" { capabilities = [\"read\"] }\npath \"auth/token/lookup-self\" { capabilities = [\"read\"] }"}' > /dev/null
}
run_playbook() { (cd "$ANSIBLE_DIR" && ansible-playbook "$1" < /dev/null > /dev/null); }
restore_policy() { retry run_playbook configure.yml; }

# --- Escenario 3: Vault se sella ----------------------------------------------------------
inject_seal() { vault_api PUT sys/seal > /dev/null; }
restore_unseal() { retry run_playbook init.yml; }

START=$(date +%s)
scenario "0. Línea base (todo sano)" nothing nothing success
scenario "1. Token expirado" inject_expired_token restore_token failure
scenario "2. Política ACL cambiada" inject_policy_change restore_policy failure
scenario "3. Vault sellado" inject_seal restore_unseal failure
scenario "4. Recuperación (todo restaurado)" nothing nothing success

{
  echo "## Pruebas de resiliencia de Vault ($(date '+%Y-%m-%d %H:%M'))"
  echo
  echo "| Escenario | CD | Causa reportada por el pipeline | Revisión de Helm | Tienda: /cart 200, medido dentro del clúster | Reglas 8200 abiertas | Resultado |"
  echo "|---|---|---|---|---|---|---|"
  printf '%s\n' "${ROWS[@]}"
  echo
  echo "Duración total: $(( ($(date +%s) - START) / 60 )) min"
} | tee "$REPORT"

echo
echo "Reporte guardado en $REPORT"
! printf '%s\n' "${ROWS[@]}" | grep -q '❌'
