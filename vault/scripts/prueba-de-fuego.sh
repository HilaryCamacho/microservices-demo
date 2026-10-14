#!/usr/bin/env bash
# Paso 6 (prueba de fuego): destruye el entorno de Vault y lo recrea desde cero sin pasos manuales.
# Después corre site.yml dos veces: la segunda debe terminar sin cambios (idempotencia).
#
# Uso: vault/scripts/prueba-de-fuego.sh
#   MY_IP=<ip>/32          IP con acceso (por defecto, la IP pública actual)
#   CI_ROLE_NAME=<rol>     rol IAM de GitHub Actions (opcional)
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
TF_DIR="$ROOT/terraform"
ANSIBLE_DIR="$ROOT/ansible"

MY_IP="${MY_IP:-$(curl -fsS https://checkip.amazonaws.com)/32}"
TF_VARS=(-var "admin_cidrs=[\"$MY_IP\"]")
if [ -n "${CI_ROLE_NAME:-}" ]; then
  TF_VARS+=(-var "ci_role_name=$CI_ROLE_NAME")
fi

START=$(date +%s)

echo "==> 1/5 Destruyendo el entorno de Vault"
terraform -chdir="$TF_DIR" destroy -auto-approve "${TF_VARS[@]}"

# Las llaves, el token y el host SSH del entorno anterior ya no sirven
if [ -d "$ANSIBLE_DIR/.secrets" ]; then
  mv "$ANSIBLE_DIR/.secrets" "$ANSIBLE_DIR/.secrets.$(date +%Y%m%d-%H%M%S).bak"
fi
rm -f "$ANSIBLE_DIR/.ssh_known_hosts" "$ANSIBLE_DIR/inventory.ini"

echo "==> 2/5 Recreando la infraestructura"
terraform -chdir="$TF_DIR" apply -auto-approve "${TF_VARS[@]}"
terraform -chdir="$TF_DIR" output -raw ansible_inventory > "$ANSIBLE_DIR/inventory.ini"

cd "$ANSIBLE_DIR"

echo "==> 3/5 Esperando a que la instancia acepte SSH"
ansible vault -m ansible.builtin.wait_for_connection -a "timeout=300"

echo "==> 4/5 Aprovisionando Vault"
ansible-playbook site.yml

echo "==> 5/5 Segunda corrida: no debe haber cambios"
LOG="$(mktemp)"
ANSIBLE_FORCE_COLOR=0 ansible-playbook site.yml | tee "$LOG"
if grep -Eq 'changed=[1-9]' "$LOG"; then
  echo "FALLÓ: la segunda corrida hizo cambios, el playbook no es idempotente" >&2
  exit 1
fi

echo "OK: entorno recreado desde cero en $(( ($(date +%s) - START) / 60 )) min y playbook idempotente (changed=0)"
