output "vault_public_ip" {
  description = "IP pública de la instancia de Vault"
  value       = aws_instance.vault.public_ip
}

output "vault_addr" {
  description = "Dirección de la UI/API de Vault (TLS se configura con Ansible en la semana 2)"
  value       = "https://${aws_instance.vault.public_ip}:8200"
}

output "vault_security_group_id" {
  description = "Security Group de Vault (variable VAULT_SG_ID en GitHub)"
  value       = aws_security_group.vault.id
}

output "ssh_command" {
  description = "Comando para entrar a la instancia"
  value       = "ssh ubuntu@${aws_instance.vault.public_ip}"
}

output "ansible_inventory" {
  description = "Inventario para Ansible: terraform output -raw ansible_inventory > ../ansible/inventory.ini"
  value       = <<-EOT
    [vault]
    ${aws_instance.vault.public_ip} ansible_user=ubuntu
  EOT
}
