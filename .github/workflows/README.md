# GitHub Actions — CI/CD

Los pipelines se autentican en AWS mediante **OIDC**: GitHub emite un token temporal y AWS lo intercambia por
credenciales de corta duración del rol `online-boutique-github-actions`. No hay access keys guardadas en el repositorio.

## Flujo

```
feature/*  ──PR──▶  develop  ──PR──▶  main
               │              │          │
               ▼              ▼          ▼
          CI (build)     CI (build)   CI (build + push a ECR)
                                          │
                                          ▼
                                   CD (helm upgrade en EKS)
```

## [ci.yaml](ci.yaml) — Integración continua

Se ejecuta en cada pull request y push a `develop` y `main`.

1. **Validar**: `terraform fmt`, `terraform validate`, `helm lint` y `helm template`.
2. **Pruebas**: `go build` y `go test` de `frontend` y `productcatalogservice` (servicios del perfil de mascota).
3. **Build**: construye en paralelo las imágenes Docker de los 11 microservicios.
   - En pull requests solo se construyen, para verificar que compilan.
   - En push a `develop`/`main` se suben a Amazon ECR con el tag `<SHA del commit>`.

## [cd.yaml](cd.yaml) — Despliegue continuo

Se ejecuta cuando el CI termina con éxito después de un push a `main`, es decir, cuando se mergea un pull request.

1. Se conecta al clúster EKS.
2. Ejecuta `helm upgrade --install` con el chart de `helm-chart/`, usando las imágenes del commit que construyó el CI.
3. Si el despliegue falla, Helm hace rollback automático a la versión anterior (`--rollback-on-failure`).
4. Publica la URL pública del frontend en el resumen del workflow y en el environment `production`.

## Configuración requerida en GitHub

En **Settings → Secrets and variables → Actions → Variables**:

| Variable | Valor |
|---|---|
| `AWS_ROLE_ARN` | `terraform output -raw github_actions_role_arn` |
| `AWS_REGION` | `us-east-2` (opcional, es el valor por defecto) |
| `EKS_CLUSTER_NAME` | `online-boutique` (opcional, es el valor por defecto) |

> `workflow_run` solo se dispara si `cd.yaml` existe en la rama por defecto (`main`).
