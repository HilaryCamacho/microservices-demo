# HashiCorp Vault (Fase III)

Servidor de Vault en una instancia EC2 **fuera del clúster de EKS**. Centraliza los secretos de la aplicación
y GitHub Actions los consume con `hashicorp/vault-action` al desplegar.

| Semana | Paso | Dónde |
|---|---|---|
| 1. Diseño e infraestructura | 1. EC2 fuera del clúster | `terraform/main.tf` |
| | 2. Security Group: acceso web por 8200 | `terraform/main.tf` |
| | 3. Flujo de secretos | [Flujo de secretos](#flujo-de-secretos) |
| | 4. Topología | [Topología](#topología) |
| 2. Aprovisionamiento automatizado | 4. Repositorio Ansible modular | `ansible/` (roles `common` y `vault`) |
| | 5. Instalar Vault, `vault.hcl` con `ui = true` y listener | `ansible/roles/vault/` |
| | 6. Prueba de fuego (idempotencia) | `scripts/prueba-de-fuego.sh` |
| 3. Configuración y conexión CI/CD | 7. Init, unseal, motor KV y UI | `ansible/init.yml` |
| | 8. Política ACL de solo lectura y token único | `policies/online-boutique-ci.hcl`, `ansible/configure.yml` |
| | 9. GitHub Actions con `vault-action` | `.github/workflows/cd.yaml` |
| 4. Pruebas e inyección en K8s | 10. Prueba de integración completa | `cd.yaml` (verificación) y [Paso 10](#paso-10-prueba-de-integración-completa) |
| | 11. Pruebas de resiliencia | `scripts/pruebas-resiliencia.sh` y [Paso 11](#paso-11-pruebas-de-resiliencia) |

## Topología

```
 Administradoras (IP /32)               GitHub Actions (runner del CD)
     │ 22 SSH (Ansible)                      │ 8200, solo la IP del runner y solo durante el job
     │ 8200 UI/API (HTTPS)                   │ (el CD agrega la regla y la quita al terminar)
     ▼                                       ▼
┌────────────── VPC online-boutique-vault-vpc · 10.50.0.0/16 ──────────────┐
│  Subred pública 10.50.0.0/24 (1 AZ, sin NAT)                             │
│   ┌──────────────────────────────────┐                                   │
│   │ EC2 Ubuntu 24.04 · t3.small       │ SG entrada: 22 y 8200 desde        │
│   │ Vault · UI + API en :8200 (TLS)   │   admin_cidrs                      │
│   │ Storage raft · KV v2 en secret/   │ SG salida: 80/443 (paquetes)       │
│   │ Disco gp3 cifrado · IMDSv2        │                                   │
│   └──────────────────────────────────┘                                   │
└──────────────────────────────────────────────────────────────────────────┘
                     │ el CD lee secret/online-boutique/app
                     ▼
┌────────────── VPC de EKS (terraform/) ─────────────┐
│ helm --set-string cartDatabase.redisPassword=…     │
│ Secret redis-cart-auth → redis (--requirepass)     │
│                        → cartservice (REDIS_ADDR)  │
└────────────────────────────────────────────────────┘
```

Decisiones:

- **VPC y estado de Terraform propios:** la prueba de fuego destruye y recrea Vault sin tocar EKS.
  Sin NAT Gateway, así que no genera ese costo.
- **Security Group cerrado:** entrada solo a 8200 y 22 desde `admin_cidrs`. La variable rechaza `0.0.0.0/0`
  y no se guarda en Git. Salida solo a 80/443.
- **TLS con CA propia:** Ansible genera una CA y un certificado con la IP pública. El pipeline confía
  en esa CA (`VAULT_CACERT`), así que el token no viaja en texto plano.
- **Acceso del pipeline:** los runners de GitHub no tienen IP fija. El CD abre 8200 solo a la IP del runner
  y la cierra al terminar (`if: always()`). El rol de CI solo puede modificar este Security Group.
- **Llaves de unseal y root token:** 3 llaves con umbral de 2. Se guardan solo en la máquina de control
  (`ansible/.secrets/`, en `.gitignore`).

## Flujo de secretos

Análisis de valores en duro en el repositorio (paso 3):

| Hallazgo | Riesgo | Decisión |
|---|---|---|
| Redis del carrito **sin contraseña**; conexión fija `redis-cart:6379` en `helm-chart/values.yaml` | Cualquier pod del clúster puede leer o borrar carritos | **Migra a Vault:** `secret/online-boutique/app` → `redis_password`. La genera Ansible y la inyecta el CD |
| Acceso de GitHub Actions a AWS | — | Ya usa **OIDC**, sin llaves que migrar |
| `AWS_ROLE_ARN`, región y nombre del clúster (GitHub Variables) | Identificadores, no secretos | Se quedan en GitHub Variables |
| Token de Vault del pipeline | "Secret zero" para entrar a Vault | GitHub Secret `VAULT_TOKEN`, con solo lectura y TTL de 30 días. Mejora futura: auth JWT/OIDC de GitHub en Vault, sin token |
| AlloyDB/Spanner (Secret Manager de GCP) en `cartservice` y `shoppingassistantservice` | Código no usado en este despliegue | Fuera de alcance |

Flujo: **Ansible** genera la contraseña → **Vault** (`secret/online-boutique/app`) → **CD** la lee con el token
de solo lectura → **Helm** crea el Secret `redis-cart-auth` → Redis exige contraseña y `cartservice` se conecta con ella.

## Uso

Requisitos: `aws login --region us-east-2`, `brew install ansible`, llave SSH en `~/.ssh/id_ed25519.pub`
y el stack de EKS (`terraform/`) aplicado antes, porque crea el rol de GitHub Actions.

```bash
# 1-2. Infraestructura
cd vault/terraform && terraform init
MY_IP="$(curl -s https://checkip.amazonaws.com)/32"
terraform apply -var="admin_cidrs=[\"$MY_IP\"]" -var="ci_role_name=online-boutique-github-actions"
terraform output -raw ansible_inventory > ../ansible/inventory.ini

# 4-5. Instalar Vault
cd ../ansible
ansible-galaxy collection install -r requirements.yml
ansible-playbook site.yml

# 6. Prueba de fuego: destruye, recrea y verifica idempotencia
CI_ROLE_NAME=online-boutique-github-actions ../scripts/prueba-de-fuego.sh

# 7. Init, unseal, KV (también desprecinta después de un reinicio)
ansible-playbook init.yml

# 8. Política, secreto y token del pipeline
ansible-playbook configure.yml

# 9. Conectar GitHub Actions
gh secret set VAULT_TOKEN < .secrets/ci-token
gh variable set VAULT_ADDR --body "$(terraform -chdir=../terraform output -raw vault_addr)"
gh variable set VAULT_SG_ID --body "$(terraform -chdir=../terraform output -raw vault_security_group_id)"
gh variable set VAULT_CACERT --body "$(base64 -i .artifacts/vault-ca.pem)"
```

Después, el CD lee el secreto en cada despliegue a `main`. Sin `VAULT_ADDR`, despliega como antes, sin Vault.

Destruir al terminar: `terraform destroy` con las mismas variables y `gh variable delete VAULT_ADDR`
para que el CD no intente conectarse a un Vault que ya no existe.

Costo aproximado: USD 0.03 por hora (t3.small, IP pública y 20 GB gp3).

## Paso 10: prueba de integración completa

Un commit a `main` dispara el flujo completo, sin pasos manuales:

```
commit → PR aprobado → CI (build + push a ECR) → CD → OIDC en AWS → abre 8200 al runner
       → diagnóstico de Vault → vault-action lee redis_password → cierra 8200
       → helm upgrade (Secret redis-cart-auth) → pods reiniciados → verificación
```

El CD termina con **"Verificar la credencial inyectada desde Vault"**, que deja en el resumen del workflow:

| Comprobación | Esperado |
|---|---|
| Huella de la contraseña en Vault / Secret `redis-cart-auth` / anotación de los pods | Las tres iguales |
| Redis sin contraseña | `NOAUTH Authentication required` |
| Redis con la contraseña del Secret | `PONG` |

Se comparan huellas (los primeros 16 caracteres del SHA-256), así que la contraseña nunca se imprime.

**Credenciales dinámicas (rotación):** la contraseña no está fija en el despliegue, se lee de Vault en cada ejecución.
Para demostrarlo, se rota antes del commit y el siguiente despliegue usa la nueva:

```bash
cd vault/ansible
ansible-playbook configure.yml -e vault_rotate_redis_password=true   # versión nueva en KV v2
# commit → PR → main: el CD inyecta la nueva contraseña
```

Helm pone la huella de la contraseña como anotación (`checksum/redis-auth`) en los pods de `redis-cart` y `cartservice`.
Si cambia, ambos se reinician juntos con la nueva. Si no, Redis seguiría con la anterior y el carrito dejaría
de conectarse en el siguiente reinicio. KV v2 conserva la versión anterior para volver a ella si hace falta.
Redis usa `emptyDir`, así que al rotar se vacían los carritos en curso.

## Paso 11: pruebas de resiliencia

Principio de diseño: **Vault solo se necesita al desplegar, no para que la tienda funcione.** La contraseña ya vive
en el Secret de Kubernetes. Si Vault falla, el CD se detiene **antes de tocar el clúster** y producción sigue igual.

| Fallo simulado | Cómo se provoca | Qué hace el pipeline | Cómo se recupera |
|---|---|---|---|
| **Token expirado** | Token de 30 s cargado en `VAULT_TOKEN` | `lookup-self` responde 403 → "el token expiró o fue revocado". Avisa si quedan menos de 7 días | `configure.yml` genera uno nuevo; `gh secret set VAULT_TOKEN` |
| **Política ACL cambiada** | La política deja de cubrir `secret/data/online-boutique/*` | El token es válido, pero leer da 403 → "la política ya no permite leer" | `configure.yml` restaura la política versionada en Git (corrige la desviación) |
| **Vault sellado** | `PUT /v1/sys/seal` | `/sys/health` responde 503 → "Vault está sellado" | `init.yml` lo desprecinta |

En todos los casos se valida que:

- el CD falla en **"Diagnóstico de Vault"** con el motivo y la recuperación en el resumen;
- **no hay revisión nueva de Helm**, porque no se desplegó nada;
- la tienda responde `200` en `/cart` durante todo el fallo;
- la regla temporal del puerto 8200 **se cierra igual** (`if: always()`).

`scripts/pruebas-resiliencia.sh` ejecuta los 3 escenarios, más una línea base antes y una recuperación al final.
Lanza el CD con `workflow_dispatch` (solo permitido desde `main`), mide la disponibilidad de la tienda cada 5 s y
genera una tabla en `ansible/.artifacts/resiliencia-<fecha>.md`:

```bash
vault/scripts/pruebas-resiliencia.sh
```

Requisitos: Vault inicializado con `init.yml` y `configure.yml`, variables de GitHub configuradas, `kubectl` conectado
a EKS y la tienda desplegada.
