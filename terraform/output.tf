output "region" {
  description = "Región de AWS"
  value       = var.region
}

output "cluster_name" {
  description = "Nombre del clúster EKS"
  value       = module.eks.cluster_name
}

output "ecr_registry" {
  description = "URL del registro ECR"
  value       = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com"
}

output "ecr_repository_prefix" {
  description = "Prefijo de los repositorios ECR (valor de images.repository en Helm)"
  value       = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.region}.amazonaws.com/${var.name}"
}

output "github_actions_role_arn" {
  description = "ARN del rol IAM para GitHub Actions (variable AWS_ROLE_ARN del repositorio)"
  value       = aws_iam_role.github_actions.arn
}

output "configure_kubectl" {
  description = "Comando para conectar kubectl al clúster"
  value       = "aws eks update-kubeconfig --region ${var.region} --name ${module.eks.cluster_name}"
}
