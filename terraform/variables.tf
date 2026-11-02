variable "project_name" {
  description = "Nome do projeto"
  type        = string
  sensitive   = false
}

variable "aws_region" {
  description = "Região AWS padrão"
  type        = string
  sensitive   = true
}

# --- Configurações de Rede (VPC e Subnets) ---

variable "vpc_cidr" {
  description = "Bloco CIDR principal da VPC"
  type        = string
  default     = "10.0.0.0/16"
}

variable "public_subnet_cidrs" {
  description = "Lista de blocos CIDR para as subnets públicas"
  type        = list(string)
  default     = ["10.0.1.0/24", "10.0.2.0/24"]
}

variable "private_subnet_cidrs" {
  description = "Lista de blocos CIDR para as subnets privadas"
  type        = list(string)
  default     = ["10.0.10.0/24", "10.0.20.0/24"]
}

variable "auth_db_username" {
  description = "Usuário master do banco RDS PostgreSQL do auth-service"
  type        = string
  sensitive   = true
}

variable "auth_db_password" {
  description = "Senha master do banco RDS PostgreSQL do auth-service"
  type        = string
  sensitive   = true
}

variable "master_key" {
  description = "Chave mestre de administracao para o auth-service"
  type        = string
  sensitive   = true
}

variable "evaluation_service_api_key" {
  description = "Chave de API interna para o evaluation-service consultar flag-service e targeting-service"
  type        = string
  sensitive   = true
}

variable "flag_db_username" {
  description = "Usuário master do banco RDS PostgreSQL do flag-service"
  type        = string
  sensitive   = true
}

variable "flag_db_password" {
  description = "Senha master do banco RDS PostgreSQL do flag-service"
  type        = string
  sensitive   = true
}

variable "targeting_db_username" {
  description = "Usuário master do banco RDS PostgreSQL do targeting-service"
  type        = string
  sensitive   = true
}

variable "targeting_db_password" {
  description = "Senha master do banco RDS PostgreSQL do targeting-service"
  type        = string
  sensitive   = true
}

# --- Configurações de Recursos e Tipos de Instância ---

variable "node_instance_types" {
  description = "Lista de tipos de instâncias EC2 para os nós do EKS"
  type        = list(string)
  default     = ["t3.medium"]
}

variable "node_desired_size" {
  description = "Quantidade desejada de nós no cluster EKS"
  type        = number
  default     = 2
}

variable "node_min_size" {
  description = "Quantidade mínima de nós no cluster EKS"
  type        = number
  default     = 2
}

variable "node_max_size" {
  description = "Quantidade máxima de nós no cluster EKS"
  type        = number
  default     = 4
}

variable "redis_node_type" {
  description = "Tipo de nó para o cluster ElastiCache Redis (ex: cache.t3.micro)"
  type        = string
  default     = "cache.t3.micro"
}

variable "rds_instance_class" {
  description = "Classe de instância para os bancos RDS PostgreSQL (ex: db.t3.micro)"
  type        = string
  default     = "db.t3.micro"
}