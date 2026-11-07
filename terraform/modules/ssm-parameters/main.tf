# ==============================================================================
# Parametros Criptografados no AWS SSM Parameter Store
# ==============================================================================

# --- Auth Service ---
resource "aws_ssm_parameter" "auth_service_database_url" {
  name        = "/auth-service/database_url"
  description = "Connection string do RDS PostgreSQL para o auth-service com SSL habilitado"
  type        = "SecureString"
  value       = "postgres://${var.auth_db_username}:${var.auth_db_password}@${var.auth_db_endpoint}/${var.auth_db_name}?sslmode=require"

  tags = {
    Environment = var.environment
    Service     = "auth-service"
    ManagedBy   = "Terraform"
  }
}

resource "aws_ssm_parameter" "auth_service_master_key" {
  name        = "/auth-service/master_key"
  description = "Chave mestre de administracao para criacao de API keys no auth-service"
  type        = "SecureString"
  value       = var.master_key

  tags = {
    Environment = var.environment
    Service     = "auth-service"
    ManagedBy   = "Terraform"
  }
}

# --- Flag Service ---
resource "aws_ssm_parameter" "flag_service_database_url" {
  name        = "/flag-service/database_url"
  description = "Connection string do RDS PostgreSQL para o flag-service com SSL habilitado"
  type        = "SecureString"
  value       = "postgres://${var.flag_db_username}:${var.flag_db_password}@${var.flag_db_endpoint}/${var.flag_db_name}?sslmode=require"

  tags = {
    Environment = var.environment
    Service     = "flag-service"
    ManagedBy   = "Terraform"
  }
}

# --- Targeting Service ---
resource "aws_ssm_parameter" "targeting_service_database_url" {
  name        = "/targeting-service/database_url"
  description = "Connection string do RDS PostgreSQL para o targeting-service com SSL habilitado"
  type        = "SecureString"
  value       = "postgres://${var.targeting_db_username}:${var.targeting_db_password}@${var.targeting_db_endpoint}/${var.targeting_db_name}?sslmode=require"

  tags = {
    Environment = var.environment
    Service     = "targeting-service"
    ManagedBy   = "Terraform"
  }
}

# --- Evaluation Service ---
resource "aws_ssm_parameter" "evaluation_service_redis_url" {
  name        = "/evaluation-service/redis_url"
  description = "Connection string do Redis ElastiCache para o evaluation-service"
  type        = "SecureString"
  value       = "redis://${var.redis_endpoint}:${var.redis_port}"

  tags = {
    Environment = var.environment
    Service     = "evaluation-service"
    ManagedBy   = "Terraform"
  }
}

resource "aws_ssm_parameter" "evaluation_service_sqs_url" {
  name        = "/evaluation-service/sqs_url"
  description = "URL da fila SQS para o evaluation-service"
  type        = "SecureString"
  value       = var.sqs_queue_id

  tags = {
    Environment = var.environment
    Service     = "evaluation-service"
    ManagedBy   = "Terraform"
  }
}

resource "aws_ssm_parameter" "evaluation_service_api_key" {
  name        = "/evaluation-service/service_api_key"
  description = "Chave de API para comunicacao interna do evaluation-service com flag e targeting services"
  type        = "SecureString"
  value       = var.evaluation_service_api_key

  tags = {
    Environment = var.environment
    Service     = "evaluation-service"
    ManagedBy   = "Terraform"
  }
}

# --- Analytics Service ---
resource "aws_ssm_parameter" "analytics_service_dynamodb_table" {
  name        = "/analytics-service/dynamodb_table"
  description = "Nome da tabela DynamoDB para o analytics-service"
  type        = "SecureString"
  value       = var.dynamodb_table_name

  tags = {
    Environment = var.environment
    Service     = "analytics-service"
    ManagedBy   = "Terraform"
  }
}

resource "aws_ssm_parameter" "analytics_service_sqs_url" {
  name        = "/analytics-service/sqs_url"
  description = "URL da fila SQS para o analytics-service"
  type        = "SecureString"
  value       = var.sqs_queue_id

  tags = {
    Environment = var.environment
    Service     = "analytics-service"
    ManagedBy   = "Terraform"
  }
}

# --- Observability, Incident Management & ChatOps (Fase 4) ---
resource "aws_ssm_parameter" "observability_pagerduty_routing_key" {
  name        = "/observability/pagerduty_routing_key"
  description = "Integration Key (Routing Key Events API v2) do PagerDuty para abertura automatica de incidentes"
  type        = "SecureString"
  value       = var.pagerduty_routing_key

  tags = {
    Environment = var.environment
    Service     = "observability"
    ManagedBy   = "Terraform"
  }
}

resource "aws_ssm_parameter" "observability_discord_webhook_url" {
  name        = "/observability/discord_webhook_url"
  description = "URL do Webhook do Discord para notificacoes de ChatOps e Self-Healing"
  type        = "SecureString"
  value       = var.discord_webhook_url

  tags = {
    Environment = var.environment
    Service     = "observability"
    ManagedBy   = "Terraform"
  }
}

