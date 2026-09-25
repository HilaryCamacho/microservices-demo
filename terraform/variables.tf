variable "name" {
  type        = string
  description = "Nombre base para el clúster EKS, la VPC y los repositorios ECR"
  default     = "online-boutique"
}

variable "region" {
  type        = string
  description = "Región de AWS donde se crea la infraestructura"
  default     = "us-east-2"
}

variable "kubernetes_version" {
  type        = string
  description = "Versión de Kubernetes del clúster EKS. Usar una versión en soporte estándar (las de soporte extendido cuestan más)"
  default     = "1.35"
}

variable "vpc_cidr" {
  type        = string
  description = "Rango CIDR de la VPC"
  default     = "10.0.0.0/16"
}

variable "node_instance_type" {
  type        = string
  description = "Tipo de instancia EC2 para los nodos del clúster"
  default     = "t3.medium"
}

variable "node_desired_size" {
  type        = number
  description = "Número de nodos deseado en el node group"
  default     = 2
}

variable "github_repository" {
  type        = string
  description = "Repositorio de GitHub (owner/repo) autorizado para desplegar vía OIDC"
  default     = "HilaryCamacho/microservices-demo"
}

variable "create_github_oidc_provider" {
  type        = bool
  description = "Crear el proveedor OIDC de GitHub. Poner en false si ya existe en la cuenta de AWS"
  default     = true
}

variable "cluster_admin_arns" {
  type        = list(string)
  description = "ARNs de usuarios/roles IAM adicionales con acceso de administrador al clúster (p. ej. otros miembros del equipo)"
  default     = []
}
