locals {
  services = [
    "adservice",
    "cartservice",
    "checkoutservice",
    "currencyservice",
    "emailservice",
    "frontend",
    "loadgenerator",
    "paymentservice",
    "productcatalogservice",
    "recommendationservice",
    "shippingservice",
  ]
}

# Un repositorio ECR por microservicio: <name>/<servicio>
resource "aws_ecr_repository" "services" {
  for_each = toset(local.services)

  name = "${var.name}/${each.key}"

  # Los tags (SHA del commit) no se pueden sobrescribir: cada imagen es trazable a un commit
  image_tag_mutability = "IMMUTABLE"

  # Permite que terraform destroy borre el repositorio aunque tenga imágenes
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }
}

# Conserva solo las últimas 10 imágenes de cada servicio para controlar el costo de almacenamiento
resource "aws_ecr_lifecycle_policy" "services" {
  for_each = aws_ecr_repository.services

  repository = each.value.name

  policy = jsonencode({
    rules = [{
      rulePriority = 1
      description  = "Conservar las ultimas 10 imagenes"
      selection = {
        tagStatus   = "any"
        countType   = "imageCountMoreThan"
        countNumber = 10
      }
      action = {
        type = "expire"
      }
    }]
  })
}
