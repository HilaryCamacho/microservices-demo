# HashiCorp Vault (Fase III)

Servidor de Vault en una instancia EC2 **fuera del clúster de EKS**, para centralizar los secretos
que hoy están en duro y que GitHub Actions los consuma con `hashicorp/vault-action`.

| Semana | Pasos | Carpeta | Estado |
|---|---|---|---|
| 1. Diseño e infraestructura | 1. EC2 · 2. Security Group 8200 · 3. Flujo de secretos · 4. Topología | `terraform/` | Terraform listo, sin aplicar |
| 2. Aprovisionamiento automatizado | 4. Roles de Ansible · 5. Instalar Vault (`ui = true`) · 6. Prueba de fuego | `ansible/` | Pendiente |
| 3. Configuración y conexión CI/CD | 7. Init, unseal, KV · 8. Política ACL de solo lectura · 9. `vault-action` | `ansible/` + `.github/workflows/` | Pendiente |

## Topología

```
 Administradoras (IP /32)                 GitHub Actions
        │  22 (SSH, Ansible)                    │  8200 (semana 3)
        │  8200 (UI/API)                        │
        ▼                                       ▼
┌──────────────── VPC online-boutique-vault-vpc · 10.50.0.0/16 ───────────────┐
│  Subred pública 10.50.0.0/24 (1 AZ, sin NAT)                                │
│   ┌───────────────────────────────┐                                         │
│   │ EC2 Ubuntu 24.04 · t3.small    │  SG: entrada 22 y 8200 solo admin_cidrs │
│   │ Vault (UI + API en :8200)      │      salida 80/443 para paquetes        │
│   │ Disco gp3 20 GB cifrado, IMDSv2│                                         │
│   └───────────────────────────────┘                                         │
└─────────────────────────────────────────────────────────────────────────────┘

┌──────────────── VPC de EKS (terraform/) ─────────────────┐
│  Clúster online-boutique · 11 microservicios              │   Separadas: Vault se puede
└───────────────────────────────────────────────────────────┘   destruir sin tocar el clúster
```

Decisiones:

- **VPC propia y estado de Terraform propio:** la prueba de fuego (paso 6) destruye y recrea Vault
  sin afectar a EKS. La VPC no lleva NAT Gateway, así que no genera ese costo.
- **Security Group cerrado:** la entrada solo permite 8200 y 22 desde las IPs de las administradoras.
  `admin_cidrs` rechaza `0.0.0.0/0` y no se guarda en Git. El puerto 22 es necesario para Ansible.
- **Salida limitada a 80/443**, lo justo para instalar paquetes.
- **Disco cifrado e IMDSv2 obligatorio.**
- **Acceso de GitHub Actions (semana 3):** los runners de GitHub no tienen IP fija. Por definir:
  abrir 8200 temporalmente a la IP del runner durante el job, o usar un runner propio.
- **TLS (semana 2):** Vault se configurará con certificado (autofirmado) para que los tokens
  no viajen en texto plano.

## Uso

Requisitos: sesión de AWS activa (`aws login --region us-east-2`) y una llave SSH en `~/.ssh/id_ed25519.pub`
(otra ruta con `-var ssh_public_key_path=...`).

```bash
cd vault/terraform
terraform init

MY_IP="$(curl -s https://checkip.amazonaws.com)/32"
terraform plan  -var="admin_cidrs=[\"$MY_IP\"]" -out=tfplan
terraform apply tfplan

terraform output                     # IP, URL de Vault, comando SSH
terraform output -raw ansible_inventory > ../ansible/inventory.ini
```

Para dar acceso a varias personas: `-var='admin_cidrs=["<ip-1>/32","<ip-2>/32"]'`.

Destruir al terminar (usa las mismas variables):

```bash
terraform destroy -var="admin_cidrs=[\"$MY_IP\"]"
```

Costo aproximado: USD 0.03 por hora (t3.small, IP pública y 20 GB gp3).

Si `t3.small` no es elegible en el plan gratuito, consulta los tipos permitidos y usa `-var instance_type=...`:

```bash
aws ec2 describe-instance-types --region us-east-2 \
  --filters Name=free-tier-eligible,Values=true \
  --query 'InstanceTypes[].InstanceType' --output text
```
