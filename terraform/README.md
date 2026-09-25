# Infraestructura en AWS con Terraform

Provisiona la infraestructura de Online Boutique en AWS (región `us-east-2`, Ohio).

| Archivo | Recursos |
|---|---|
| `main.tf` | VPC (2 AZs, subredes públicas/privadas, 1 NAT Gateway) y clúster **EKS** con un node group administrado |
| `ecr.tf` | Un repositorio **ECR** por microservicio (`online-boutique/<servicio>`), escaneo de imágenes y política de retención |
| `github_oidc.tf` | Proveedor OIDC de GitHub y rol IAM que usan los pipelines de CI/CD (sin access keys) |
| `output.tf` | Datos que se necesitan para configurar GitHub Actions y `kubectl` |

## Requisitos

- Terraform >= 1.6
- AWS CLI v2 autenticado en la cuenta (`aws configure sso` o `aws configure`)
- Verificar la identidad antes de aplicar: `aws sts get-caller-identity`

## Uso

```bash
cd terraform
terraform init
terraform plan
terraform apply
```

El clúster tarda ~15 minutos en crearse. Al terminar:

```bash
# Conectar kubectl al clúster
$(terraform output -raw configure_kubectl)
kubectl get nodes

# Valores para configurar GitHub Actions
terraform output github_actions_role_arn
```

## Destruir (para no generar costos)

```bash
# Primero eliminar el Load Balancer creado por Kubernetes
helm uninstall online-boutique -n online-boutique
terraform destroy
```

> El Load Balancer del frontend lo crea Kubernetes, no Terraform. Si no se desinstala el release de Helm antes,
> `terraform destroy` puede fallar al borrar la VPC.

## Costo aproximado

| Recurso | USD/hora |
|---|---|
| Control plane EKS | 0.10 |
| 2 × EC2 t3.medium | 0.083 |
| NAT Gateway | 0.045 |
| Classic Load Balancer | 0.025 |
| **Total** | **~0.25 (~6 USD/día)** |

Recomendación: destruir la infraestructura cuando no se use y configurar una alerta en AWS Budgets.

## Notas

- El estado (`terraform.tfstate`) se guarda localmente y está excluido de Git porque puede contener datos sensibles.
  Para trabajo en equipo conviene migrarlo al backend S3 comentado en `providers.tf`.
- Para dar acceso a `kubectl` a otro miembro del equipo, agregar su ARN IAM en `cluster_admin_arns` (`terraform.tfvars`).
- Si la cuenta ya tiene un proveedor OIDC de GitHub, usar `create_github_oidc_provider = false`.
