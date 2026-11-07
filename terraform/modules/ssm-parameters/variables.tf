variable "auth_db_username" {
  description = "Usuario do banco RDS PostgreSQL do auth-service"
  type        = string
}

variable "auth_db_password" {
  description = "Senha do banco RDS PostgreSQL do auth-service"
  type        = string
  sensitive   = true
}

variable "auth_db_endpoint" {
  description = "Endpoint do banco RDS PostgreSQL do auth-service"
  type        = string
}

variable "auth_db_name" {
  description = "Nome do banco de dados do auth-service"
  type        = string
}

variable "master_key" {
  description = "Chave mestre de administracao para criacao de API keys no auth-service"
  type        = string
  sensitive   = true
}

variable "flag_db_username" {
  description = "Usuario do banco RDS PostgreSQL do flag-service"
  type        = string
}

variable "flag_db_password" {
  description = "Senha do banco RDS PostgreSQL do flag-service"
  type        = string
  sensitive   = true
}

variable "flag_db_endpoint" {
  description = "Endpoint do banco RDS PostgreSQL do flag-service"
  type        = string
}

variable "flag_db_name" {
  description = "Nome do banco de dados do flag-service"
  type        = string
}

variable "targeting_db_username" {
  description = "Usuario do banco RDS PostgreSQL do targeting-service"
  type        = string
}

variable "targeting_db_password" {
  description = "Senha do banco RDS PostgreSQL do targeting-service"
  type        = string
  sensitive   = true
}

variable "targeting_db_endpoint" {
  description = "Endpoint do banco RDS PostgreSQL do targeting-service"
  type        = string
}

variable "targeting_db_name" {
  description = "Nome do banco de dados do targeting-service"
  type        = string
}

variable "redis_endpoint" {
  description = "Endpoint do cluster ElastiCache Redis"
  type        = string
}

variable "redis_port" {
  description = "Porta do cluster ElastiCache Redis"
  type        = number
}

variable "sqs_queue_id" {
  description = "URL/ID da fila SQS criada"
  type        = string
}

variable "evaluation_service_api_key" {
  description = "Chave de API do evaluation-service para comunicacao interna"
  type        = string
  sensitive   = true
}

variable "dynamodb_table_name" {
  description = "Nome da tabela DynamoDB do analytics-service"
  type        = string
}

variable "environment" {
  description = "Ambiente de deploy"
  type        = string
  default     = "prod"
}

variable "pagerduty_routing_key" {
  description = "Integration Key (Routing Key Events API v2) do PagerDuty"
  type        = string
  sensitive   = true
}

variable "discord_webhook_url" {
  description = "URL do Webhook do Discord para ChatOps e Self-Healing"
  type        = string
  sensitive   = true
}

