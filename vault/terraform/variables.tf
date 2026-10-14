variable "name" {
  type        = string
  description = "Prefijo para nombrar los recursos de Vault"
  default     = "online-boutique-vault"
}

variable "region" {
  type        = string
  description = "Región de AWS"
  default     = "us-east-2"
}

variable "vpc_cidr" {
  type        = string
  description = "Rango CIDR de la VPC de Vault (no debe traslaparse con la VPC de EKS)"
  default     = "10.50.0.0/16"
}

variable "instance_type" {
  type        = string
  description = "Tipo de instancia EC2. Debe ser elegible en el plan gratuito de la cuenta"
  default     = "t3.small"
}

variable "root_volume_size" {
  type        = number
  description = "Tamaño del disco raíz en GB"
  default     = 20
}

variable "ssh_public_key_path" {
  type        = string
  description = "Llave pública SSH que usará Ansible para configurar la instancia"
  default     = "~/.ssh/id_ed25519.pub"
}

variable "ci_role_name" {
  type        = string
  description = "Rol IAM de GitHub Actions (stack terraform/). Si se indica, puede abrir 8200 a la IP del runner durante el CD. Vacío = sin acceso del pipeline"
  default     = ""
}

variable "admin_cidrs" {
  type        = list(string)
  description = "IPs (/32) de las administradoras con acceso a Vault (8200) y SSH (22). Se pasa al ejecutar, no se guarda en Git"

  validation {
    condition     = length(var.admin_cidrs) > 0 && alltrue([for c in var.admin_cidrs : can(cidrnetmask(c)) && !startswith(c, "0.0.0.0/")])
    error_message = "admin_cidrs debe tener al menos un CIDR válido y no puede abrir el acceso a todo internet (0.0.0.0/0)."
  }
}
