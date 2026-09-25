# Fase II — CI/CD e infraestructura en AWS

Proyecto Integrador de la Metodología DevOps · Hilary Camacho y Danae Barba

## Decisión: migración de GCP a AWS

En la Fase I se planteó Google Cloud (GKE + Artifact Registry). Para la Fase II el equipo migró a **AWS**:

- Coincide con el entorno de referencia del material del curso (EKS / ECR).
- EKS es Kubernetes estándar, así que el Helm chart y el flujo con `kubectl` se reutilizan sin cambios.
- La configuración anterior de GCP queda en el historial de Git (commits de `feature/perfil-mascota`).

| Componente | GCP (antes) | AWS (ahora) |
|---|---|---|
| Kubernetes | GKE Autopilot | **Amazon EKS** (node group EC2 t3.medium) |
| Container registry | Artifact Registry | **Amazon ECR** (un repositorio por servicio) |
| Red | VPC por defecto | VPC propia: subredes públicas/privadas, NAT Gateway |
| Autenticación CI/CD | — | **OIDC** GitHub → IAM Role (sin access keys) |

## Herramientas

| Requerimiento | Herramienta usada |
|---|---|
| Planning tool | GitHub Projects (tablero Kanban) |
| Kubernetes | Amazon EKS (local: Docker Desktop) |
| Docker | Imágenes de los 11 microservicios |
| Git | GitHub, GitFlow simplificado (`feature/*` → `develop` → `main`) |
| GitHub Actions | `ci.yaml` y `cd.yaml` |
| Terraform | `terraform/` (VPC, EKS, ECR, IAM/OIDC) |
| Helm | `helm-chart/` personalizado para EKS/ECR |

## Arquitectura

```
Desarrollador ──push──▶ GitHub ──PR mergeado a main──▶ GitHub Actions
                                                         │
                          CI: test + docker build ───────┤──push──▶ Amazon ECR
                                                         │             │
                          CD: helm upgrade ──────────────┘             │ pull
                                                                       ▼
                         Usuario ──HTTP──▶ Load Balancer ──▶ EKS (VPC us-east-2)
                                                            frontend + 10 servicios + redis
```

## 1. Infraestructura con Terraform

Ver [terraform/README.md](../../terraform/README.md). Crea:

- **VPC** en 2 zonas de disponibilidad. Los nodos van en subredes privadas y el Load Balancer en las públicas.
- **Clúster EKS** con add-ons administrados (CoreDNS, kube-proxy, VPC CNI, Pod Identity) y 2 nodos t3.medium (escala de 1 a 3).
- **11 repositorios ECR** con tags inmutables, escaneo de vulnerabilidades al subir y retención de las últimas 10 imágenes.
- **Rol IAM para GitHub Actions** vía OIDC, con permisos solo para subir imágenes a esos repositorios y desplegar en el clúster.

## 2. CI — Build de imágenes y push a ECR

[`.github/workflows/ci.yaml`](../../.github/workflows/ci.yaml)

- Valida Terraform y Helm, y corre las pruebas de Go de los servicios modificados.
- Construye las 11 imágenes en paralelo (matrix) con caché de capas.
- En push a `develop`/`main` sube cada imagen a ECR con el **SHA del commit** como tag. Así cada imagen se puede rastrear al código exacto que la generó.

## 3. CD — Despliegue con Helm en EKS

[`.github/workflows/cd.yaml`](../../.github/workflows/cd.yaml)

- Se dispara cuando un pull request se mergea a `main` y el CI termina con éxito.
- Ejecuta `helm upgrade --install` con las imágenes que acaba de subir el CI.
- Hace rollback automático si algún pod no queda listo.
- Registra el despliegue en el environment `production` de GitHub con la URL pública del frontend.

**Personalización del Helm chart** (`helm-chart/`):
- `images.repository` apunta a ECR y `images.tag` al SHA del commit.
- `frontend.platform: aws` hace que la tienda muestre el banner de AWS.
- Metadatos del chart (descripción, mantenedores y versión `0.11.0`).

## Puesta en marcha (una sola vez)

1. **Credenciales AWS locales** (no se guardan en el repositorio):
   ```bash
   aws configure sso        # o: aws configure
   aws sts get-caller-identity
   ```
2. **Crear la infraestructura**:
   ```bash
   cd terraform && terraform init && terraform apply
   ```
3. **Configurar GitHub**: en *Settings → Secrets and variables → Actions → Variables*, crear `AWS_ROLE_ARN` con el valor de `terraform output -raw github_actions_role_arn`.
4. **Primer despliegue**: mergear un PR a `develop` y después de `develop` a `main`. El CI sube las imágenes y el CD despliega.
5. **Verificar**:
   ```bash
   $(terraform output -raw configure_kubectl)
   kubectl get pods -n online-boutique
   kubectl get svc frontend-external -n online-boutique   # URL pública
   ```

## Seguridad

- Sin access keys de AWS en GitHub: se usa OIDC con tokens temporales.
- El rol de CI/CD solo lo pueden asumir las ramas `main`/`develop` y el environment `production` de este repositorio.
- El estado de Terraform está excluido de Git (`.gitignore`).
- ECR escanea las imágenes en busca de vulnerabilidades al subirlas.
- Mejora pendiente (Fase III): limitar el acceso del rol de CI/CD a un namespace en lugar de todo el clúster, y mover el estado de Terraform a S3.

## Costos

~0.25 USD/hora (~6 USD/día) mientras la infraestructura existe. Destruirla al terminar cada sesión de pruebas
(ver [terraform/README.md](../../terraform/README.md#destruir-para-no-generar-costos)).
