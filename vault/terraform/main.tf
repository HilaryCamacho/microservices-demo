data "aws_availability_zones" "available" {
  state = "available"
}

# AMI oficial de Ubuntu 24.04 publicada por Canonical
data "aws_ssm_parameter" "ubuntu_ami" {
  name = "/aws/service/canonical/ubuntu/server/24.04/stable/current/amd64/hvm/ebs-gp3/ami-id"
}

# Red propia de Vault, fuera del clúster de EKS: una subred pública y sin NAT Gateway
module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 6.0"

  name = "${var.name}-vpc"
  cidr = var.vpc_cidr

  azs            = [data.aws_availability_zones.available.names[0]]
  public_subnets = [cidrsubnet(var.vpc_cidr, 8, 0)]

  map_public_ip_on_launch = true
  enable_nat_gateway      = false
  enable_dns_hostnames    = true
}

# Entrada: solo 8200 (UI/API de Vault) y 22 (SSH para Ansible), y solo desde admin_cidrs
resource "aws_security_group" "vault" {
  name        = "${var.name}-sg"
  description = "Vault: UI/API en 8200 y SSH para Ansible, solo desde IPs permitidas"
  vpc_id      = module.vpc.vpc_id

  tags = { Name = "${var.name}-sg" }
}

resource "aws_vpc_security_group_ingress_rule" "vault_api" {
  for_each = toset(var.admin_cidrs)

  security_group_id = aws_security_group.vault.id
  description       = "Vault UI/API"
  cidr_ipv4         = each.value
  ip_protocol       = "tcp"
  from_port         = 8200
  to_port           = 8200
}

resource "aws_vpc_security_group_ingress_rule" "ssh" {
  for_each = toset(var.admin_cidrs)

  security_group_id = aws_security_group.vault.id
  description       = "SSH para Ansible"
  cidr_ipv4         = each.value
  ip_protocol       = "tcp"
  from_port         = 22
  to_port           = 22
}

# Salida: solo HTTP/HTTPS para instalar paquetes (apt y repositorio de HashiCorp).
# DNS y NTP de AWS no pasan por el Security Group.
resource "aws_vpc_security_group_egress_rule" "web" {
  for_each = toset(["80", "443"])

  security_group_id = aws_security_group.vault.id
  description       = "Salida TCP ${each.value} para paquetes"
  cidr_ipv4         = "0.0.0.0/0"
  ip_protocol       = "tcp"
  from_port         = tonumber(each.value)
  to_port           = tonumber(each.value)
}

resource "aws_key_pair" "vault" {
  key_name   = "${var.name}-key"
  public_key = file(pathexpand(var.ssh_public_key_path))
}

resource "aws_instance" "vault" {
  ami                         = data.aws_ssm_parameter.ubuntu_ami.insecure_value
  instance_type               = var.instance_type
  subnet_id                   = module.vpc.public_subnets[0]
  vpc_security_group_ids      = [aws_security_group.vault.id]
  key_name                    = aws_key_pair.vault.key_name
  associate_public_ip_address = true

  # IMDSv2 obligatorio
  metadata_options {
    http_endpoint = "enabled"
    http_tokens   = "required"
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.root_volume_size
    encrypted   = true
  }

  tags = { Name = var.name }

  # Una nueva versión de la AMI no debe reemplazar la instancia en cada plan
  lifecycle {
    ignore_changes = [ami]
  }
}
