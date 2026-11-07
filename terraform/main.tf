# ==============================================================================
# 1. Módulo de Rede (Networking - VPC, Subnets Públicas/Privadas, IGW e NAT GW)
# ==============================================================================
# Cria a topologia de rede isolada onde o cluster EKS, RDS e ElastiCache serão alocados.
module "network" {
  source = "./modules/network-vpc"

  project_name         = var.project_name
  cluster_name         = "${var.project_name}-eks-cluster"
  vpc_cidr             = var.vpc_cidr
  availability_zones   = ["${var.aws_region}a", "${var.aws_region}b"]
  public_subnet_cidrs  = var.public_subnet_cidrs
  private_subnet_cidrs = var.private_subnet_cidrs
}

# ==============================================================================
# 2. Módulo do Cluster Kubernetes (AWS EKS & Managed Node Groups)
# ==============================================================================
# Orquestrador de contêineres provisionado dentro das subnets privadas da VPC.
# A dependência do módulo 'network' é resolvida automaticamente pelo Terraform.
module "eks" {
  source = "./modules/eks"

  project_name        = var.project_name
  cluster_name        = "${var.project_name}-eks-cluster"
  cluster_version     = "1.31"
  vpc_id              = module.network.vpc_id
  subnet_ids          = module.network.private_subnet_ids
  node_instance_types = var.node_instance_types
  desired_size        = var.node_desired_size
  min_size            = var.node_min_size
  max_size            = var.node_max_size
}

# ==============================================================================
# 3. Módulos de Banco de Dados Relacional (AWS RDS PostgreSQL Dedicados)
# ==============================================================================
# Banco de dados relacional dedicado para o auth-service (auth_db / auth-db)
module "auth_rds" {
  source = "./modules/rds"

  project_name          = "${var.project_name}-auth"
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.private_subnet_ids
  eks_security_group_id = module.eks.cluster_security_group_id
  db_instance_class     = var.rds_instance_class
  db_name               = "auth_db"
  db_username           = var.auth_db_username
  db_password           = var.auth_db_password
}

# Banco de dados relacional dedicado para o flag-service (flag_db / flag-db)
module "flag_rds" {
  source = "./modules/rds"

  project_name          = "${var.project_name}-flag"
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.private_subnet_ids
  eks_security_group_id = module.eks.cluster_security_group_id
  db_instance_class     = var.rds_instance_class
  db_name               = "flag_db"
  db_username           = var.flag_db_username
  db_password           = var.flag_db_password
}

# Banco de dados relacional dedicado para o targeting-service (targeting_db / targeting-db)
module "targeting_rds" {
  source = "./modules/rds"

  project_name          = "${var.project_name}-targeting"
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.private_subnet_ids
  eks_security_group_id = module.eks.cluster_security_group_id
  db_instance_class     = var.rds_instance_class
  db_name               = "targeting_db"
  db_username           = var.targeting_db_username
  db_password           = var.targeting_db_password
}

# ==============================================================================
# 4. Módulo de Cache em Memória (AWS ElastiCache Redis)
# ==============================================================================
# Cache em memória e controle de sessão/flags com acesso seguro via SG do EKS.
module "redis" {
  source = "./modules/redis"

  project_name          = var.project_name
  vpc_id                = module.network.vpc_id
  subnet_ids            = module.network.private_subnet_ids
  eks_security_group_id = module.eks.cluster_security_group_id
  node_type             = var.redis_node_type
  engine_version        = "7.0"
}

# ==============================================================================
# 5. Módulo de Banco de Dados NoSQL (AWS DynamoDB)
# ==============================================================================
# Tabela NoSQL gerenciada e serverless (sob demanda) para analytics de eventos.
module "dynamodb" {
  source = "./modules/dynamodb"

  table_name   = "ToggleMasterAnalytics"
  billing_mode = "PAY_PER_REQUEST"
  hash_key     = "event_id"
}

# ==============================================================================
# 6. Módulo de Mensageria Assíncrona (AWS SQS)
# ==============================================================================
# Fila SQS para desacoplamento e comunicação assíncrona entre os microsserviços.
module "sqs" {
  source   = "./modules/sqs"
  sqs_name = "${var.project_name}-evaluation-queue"
}

# ==============================================================================
# 7. Módulo IAM & IRSA (Permissões EKS Node, Evaluation e Analytics)
# ==============================================================================
# Encapsula roles e políticas IAM para integração OIDC (IRSA) e nós EKS.
module "iam_irsa" {
  source = "./modules/iam-irsa"

  project_name       = var.project_name
  namespace          = "toggle-master"
  node_role_name     = module.eks.node_role_name
  sqs_queue_arn      = module.sqs.queue_arn
  dynamodb_table_arn = module.dynamodb.table_arn
  oidc_provider_arn  = module.eks.oidc_provider_arn
  oidc_provider_url  = module.eks.oidc_provider_url

  depends_on = [module.eks]
}

# ==============================================================================
# 8. Módulo de Registro de Contêineres (AWS ECR)
# ==============================================================================
# Repositórios privados de imagens Docker dos microsserviços com scan e política de ciclo de vida.
module "ecr" {
  source = "./modules/ecr"
  repository_names = [
    "${var.project_name}/auth-service",
    "${var.project_name}/flag-service",
    "${var.project_name}/targeting-service",
    "${var.project_name}/evaluation-service",
    "${var.project_name}/analytics-service"
  ]
}

# ==============================================================================
# 9. Módulo GitOps / CD (ArgoCD)
# ==============================================================================
# Instalação do ArgoCD via Helm Chart gerenciado diretamente pelo Terraform no cluster EKS.
module "argocd" {
  source = "./modules/argocd"

  namespace              = "argocd"
  server_service_type    = "ClusterIP"
  timeout_reconciliation = "30s"

  depends_on = [module.eks]
}

# ==============================================================================
# 10. Módulo Ingress Controller (Ingress NGINX + AWS NLB)
# ==============================================================================
# Instalação do Ingress NGINX via Helm Chart para gerenciar o Load Balancer e roteamento HTTP/HTTPS.
module "ingress_nginx" {
  source = "./modules/ingress-nginx"

  namespace     = "ingress-nginx"
  chart_version = "4.12.0"

  depends_on = [module.eks]
}

# ==============================================================================
# 11. Módulo de Parâmetros e Segredos (AWS SSM Parameter Store)
# ==============================================================================
# Armazena connection strings computadas e chaves criptografadas via AWS KMS.
module "ssm_parameters" {
  source = "./modules/ssm-parameters"

  auth_db_username = module.auth_rds.db_username
  auth_db_password = var.auth_db_password
  auth_db_endpoint = module.auth_rds.db_endpoint
  auth_db_name     = module.auth_rds.db_name
  master_key       = var.master_key

  flag_db_username = var.flag_db_username
  flag_db_password = var.flag_db_password
  flag_db_endpoint = module.flag_rds.db_endpoint
  flag_db_name     = module.flag_rds.db_name

  targeting_db_username = var.targeting_db_username
  targeting_db_password = var.targeting_db_password
  targeting_db_endpoint = module.targeting_rds.db_endpoint
  targeting_db_name     = module.targeting_rds.db_name

  redis_endpoint = module.redis.redis_endpoint
  redis_port     = module.redis.redis_port

  sqs_queue_id               = module.sqs.queue_id
  evaluation_service_api_key = var.evaluation_service_api_key
  dynamodb_table_name        = module.dynamodb.table_name
  pagerduty_routing_key      = var.pagerduty_routing_key
  discord_webhook_url        = var.discord_webhook_url

  depends_on = [
    module.auth_rds,
    module.flag_rds,
    module.targeting_rds,
    module.redis,
    module.dynamodb,
    module.sqs
  ]
}

# ==============================================================================
# 12. Módulo External Secrets Operator (ESO via IRSA / OIDC)
# ==============================================================================
# Instala o operador ESO no EKS e configura IAM Role/Policy para sincronização automática.
module "external_secrets" {
  source = "./modules/external-secrets"

  project_name      = var.project_name
  aws_region        = var.aws_region
  oidc_provider_arn = module.eks.oidc_provider_arn
  oidc_provider_url = module.eks.oidc_provider_url

  depends_on = [module.eks]
}