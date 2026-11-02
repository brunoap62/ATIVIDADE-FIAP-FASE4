# 🚀 ToggleMaster — FIAP Postech (Tech Challenge Fase 3)

---

## 📌 1. Visão Geral da Arquitetura

O **ToggleMaster** é uma plataforma distribuída de gerenciamento e avaliação de Feature Flags em tempo real, composta por **5 microsserviços especializados**:

```mermaid
flowchart TD
    subgraph AWS_Cloud ["AWS Cloud (us-east-2)"]
        subgraph VPC ["VPC Isolada"]
            subgraph EKS_Cluster ["Amazon EKS Cluster 1.31"]
                IngressNginx["Ingress NGINX Controller (AWS NLB)"]
                ArgoCD["ArgoCD (GitOps Engine)"]
                ESO["External Secrets Operator (ESO)"]
                K8sSecrets["Secrets Nativas do K8s\n(*-service-secret)"]

                AuthSvc["auth-service (Go)"]
                FlagSvc["flag-service (Python)"]
                TargetingSvc["targeting-service (Python)"]
                EvalSvc["evaluation-service (Go)"]
                AnalyticsSvc["analytics-service (Python)"]
            end

            subgraph Private_Data ["Camada de Dados & Cache"]
                AuthRDS[("RDS: auth_db\n(PostgreSQL)")]
                FlagRDS[("RDS: flag_db\n(PostgreSQL)")]
                TargetingRDS[("RDS: targeting_db\n(PostgreSQL)")]
                RedisCluster[("ElastiCache Redis 7.0")]
                DynamoDBTable[("DynamoDB Table\n(Serverless)")]
            end
        end

        SQSQueue[("AWS SQS Queue\n(evaluation-queue)")]
        SSM[("AWS SSM Parameter Store\n(SecureString)")]
        ECR["AWS ECR Repositories"]
    end

    IngressNginx --> AuthSvc
    IngressNginx --> FlagSvc
    IngressNginx --> TargetingSvc
    IngressNginx --> EvalSvc
    IngressNginx --> AnalyticsSvc

    AuthSvc --> AuthRDS
    FlagSvc --> FlagRDS
    TargetingSvc --> TargetingRDS
    EvalSvc --> RedisCluster
    EvalSvc --> SQSQueue
    AnalyticsSvc --> SQSQueue
    AnalyticsSvc --> DynamoDBTable

    ESO -.->|1. Consulta via IRSA| SSM
    ESO -->|2. Cria/Sincroniza| K8sSecrets
    K8sSecrets -->|3. Injeta envFrom| AuthSvc
    K8sSecrets -->|3. Injeta envFrom| FlagSvc
    K8sSecrets -->|3. Injeta envFrom| TargetingSvc
    K8sSecrets -->|3. Injeta envFrom| EvalSvc
    K8sSecrets -->|3. Injeta envFrom| AnalyticsSvc

    ArgoCD -.->|Aplica Manifestos Kustomize| EKS_Cluster
```

---

## 🧩 2. Os 5 Microsserviços do Projeto

| Microsserviço            | Linguagem      | Banco / Integração              | Responsabilidade                                                                     |
| :----------------------- | :------------- | :------------------------------ | :----------------------------------------------------------------------------------- |
| **`auth-service`**       | Go (Gin)       | RDS PostgreSQL (`auth_db`)      | Gerenciamento de chaves de API (`api_keys`) e validação de tokens.                   |
| **`flag-service`**       | Python (Flask) | RDS PostgreSQL (`flag_db`)      | CRUD de Feature Flags e status de ativação/desativação.                              |
| **`targeting-service`**  | Python (Flask) | RDS PostgreSQL (`targeting_db`) | Regras de segmentação de público e targeting por atributos.                          |
| **`evaluation-service`** | Go             | Redis + SQS + Flag/Targeting    | Motor de avaliação de regras de alta performance com cache L1 e eventos assíncronos. |
| **`analytics-service`**  | Python         | AWS SQS + DynamoDB              | Consumidor assíncrono que persiste métricas e eventos de avaliação no NoSQL.         |

---

## 🔐 3. Gestão Segura de Segredos (AWS SSM + ESO + IRSA)

Adotamos a política de **Segurança por Design e Automação Total**. Nenhuma credencial trafega em texto plano no Git e nenhuma senha é criada manualmente no console da AWS.

### 🔄 Diagrama de Sequência do Fluxo de Segredos:

```mermaid
sequenceDiagram
    autonumber
    actor Dev as Dev / CI
    participant TF as Terraform (IaC)
    participant RDS as AWS RDS PostgreSQL
    participant SSM as AWS SSM Parameter Store
    participant ESO as External Secrets Operator
    participant K8s as Kubernetes Secret
    participant App as Pod (Microsserviço)

    %% Fase 1: Provisionamento
    Dev->>TF: Executa terraform apply
    TF->>RDS: Provisiona instancia PostgreSQL (db.t3.micro)
    RDS-->>TF: Retorna Endpoint real da instancia
    TF->>SSM: Cria parametro SecureString (/auth-service/database_url)
    Note over TF,SSM: Connection string gerada automaticamente sem expor senhas no Git

    %% Fase 2: Sincronização
    Note over ESO: ESO le o ClusterSecretStore e o ExternalSecret
    ESO->>SSM: Consulta parametro via IRSA (Token OIDC temporario)
    SSM-->>ESO: Retorna connection string descriptografada
    ESO->>K8s: Cria ou atualiza o recurso Secret (auth-service-secret)
    Note over ESO,K8s: Secret nativa criada automaticamente no namespace toggle-master

    %% Fase 3: Consumo
    App->>K8s: Carrega variaveis de ambiente (envFrom secretRef)
    K8s-->>App: Injeta DATABASE_URL no container
    App->>RDS: Conecta ao banco e executa auto-migration
```

### 🧱 Os Componentes Envolvidos:

1. **AWS SSM Parameter Store:** Armazena parâmetros confidenciais criptografados (`SecureString`).
2. **IRSA (IAM Roles for Service Accounts):** Concede permissões granulares aos Pods do Kubernetes via tokens JWT OIDC temporários, eliminando chaves estáticas (`AWS_ACCESS_KEY_ID`).
3. **`ClusterSecretStore` ([`cluster-secret-store.yml`](gitops/infrastructure/base/cluster-secret-store.yml)):** Configura o conector global do Kubernetes com o Parameter Store da AWS.
4. **`ExternalSecret` ([`externalsecret.yml`](gitops/apps/auth-service/base/externalsecret.yml)):** Mapeia os parâmetros da AWS para Secrets nativas do Kubernetes.
5. **`Secret` Nativa:** Injetada nos Pods via `envFrom: secretRef` de forma totalmente transparente.

---

## 🏗️ 4. Infraestrutura como Código (Terraform Modular)

Toda a infraestrutura é provisionada de forma 100% declarativa, reproduzível e modular:

```text
terraform/
├── main.tf                     # Camada de composição limpa dos módulos
├── variables.tf / outputs.tf   # Declaração de entradas e saídas
├── backend.tf                  # Estado remoto no AWS S3 com Lock no DynamoDB
└── modules/
    ├── network-vpc/            # VPC, Subnets Públicas/Privadas, NAT Gateway e IGW
    ├── eks/                    # Cluster Amazon EKS 1.31 com Managed Node Groups
    ├── rds/                    # 3x Instâncias dedicadas RDS PostgreSQL
    ├── redis/                  # Cluster AWS ElastiCache Redis 7.0
    ├── dynamodb/               # Tabela NoSQL Serverless (PAY_PER_REQUEST)
    ├── sqs/                    # Fila AWS SQS para eventos
    ├── ssm-parameters/         # Parâmetros e connection strings criptografadas
    ├── iam-irsa/               # IAM Roles com OIDC para pods e operadores
    ├── ingress-nginx/          # Ingress Controller integrado a AWS NLB
    ├── external-secrets/       # Helm Release do ESO
    └── argocd/                 # Helm Release do ArgoCD
```

---

## 💰 5. Estimativa Oficial de Custos na AWS (FinOps)

O dimensionamento da infraestrutura foi planejado seguindo os pilares de **Excelência Operacional e Otimização de Custos (FinOps)** do *AWS Well-Architected Framework*.

A estimativa oficial foi modelada através da [AWS Pricing Calculator](https://calculator.aws/) sob o ID de workload **`d56598ab-c232-4a75-b5b6-892ae7bdebe6`**, totalizando **US$ 247,11 / mês**:

![Estimativa Oficial de Custos AWS](Calculadora%20de%20custos.png)

### 📊 Resumo de Custos Mensais por Recurso:

| Serviço AWS | Componente & Dimensionamento no Terraform | Custo Mensal (USD) |
| :--- | :--- | :--- |
| **Amazon EKS** | 1x Cluster Control Plane 1.31 (Standard Support - 730h) | **$73.00** |
| **Amazon EC2 (EKS Nodes)** | 2x Instâncias `t3.medium` On-Demand (1.460h) | **$60.74** |
| **Amazon RDS (PostgreSQL)** | 3x Instâncias `db.t3.micro` Single-AZ (`auth_db`, `flag_db`, `targeting_db` - 2.190h) | **$39.42** |
| **AWS VPC (NAT Gateway)** | 1x NAT Gateway nas subnets públicas (730h) | **$32.85** |
| **Elastic Load Balancing (NLB)** | 1x Network Load Balancer (Ingress NGINX - 730h) + LCUs | **$16.49** |
| **Amazon ElastiCache (Redis)** | 1x Nó `cache.t3.micro` On-Demand (Redis 7.0 - 730h) | **$12.41** |
| **Amazon RDS (Storage gp3)** | 60 GB GP3 Storage (20 GB por banco RDS) | **$6.90** |
| **Amazon EC2 (Storage gp3)** | 40 GB GP3 EBS Storage (20 GB por nó EKS) | **$3.20** |
| **Amazon ECR** | 10 GB de armazenamento de imagens Docker (5 repositórios) | **$1.00** |
| **Amazon DynamoDB** | Modo On-Demand (`PAY_PER_REQUEST` - 1M writes + 500k reads + 5 GB) | **$0.69** |
| **Amazon SQS** | 1x Fila Standard `evaluation-queue` (1.000.000 requisições/mês) | **$0.40** |
| **AWS SSM Parameter Store** | Parâmetros `SecureString` no Standard Tier | **$0.00 (Gratuito)** |
| **IAM IRSA / OIDC** | Autenticação temporária sem chaves estáticas | **$0.00 (Gratuito)** |
| **TOTAL MENSAL ESTIMADO** | **Ambiente de Produção Completo em Operação 24/7** | **US$ 247.11 / mês** |

> 💡 **Destaques de FinOps e Decisões Conscientes de Arquitetura:**
> - **Criptografia em Repouso no RDS (`storage_encrypted = true`):** Todas as 3 instâncias PostgreSQL utilizam volumes `gp3` criptografados via AWS KMS sem custo adicional.
> - **Política de Backups e Retenção no RDS (`backup_retention_period = 0` e `skip_final_snapshot = true`):** Decisão consciente de FinOps e agilidade operacional para laboratório acadêmico efêmero — evita cobranças residuais de snapshots após o `terraform destroy` e acelera o descomissionamento da infraestrutura, dado que os schemas e dados iniciais são 100% reproduzíveis via *auto-migration* no startup dos serviços. *(Em um cenário produtivo crítico, aplica-se `backup_retention_period = 7` a `35` dias e `multi_az = true`)*.
> - **DynamoDB Serverless (`PAY_PER_REQUEST`):** Custo praticamente nulo quando ocioso ($0,69/mês), sem pagar por capacidade provisionada desnecessária.
> - **SSM Parameter Store Standard Tier:** Gestão de segredos e credenciais criptografadas sem custos fixos mensais por segredo.
> - **ArgoCD via `ClusterIP`:** Economia de ~$20/mês eliminando a necessidade de um segundo Load Balancer dedicado.
> - **Storage Moderno GP3:** Garante 3.000 IOPS e 125 MB/s nativos por disco com custo 20% menor que o antigo GP2.

---

## 🐙 6. GitOps com Kustomize & ArgoCD

A entrega contínua segue os princípios de **GitOps**, onde o estado do cluster é uma representação fiel do repositório Git.

### 📂 Estrutura Semântica do GitOps (`gitops/`):

```text
gitops/
├── bootstrap/
│   └── argocd/
│       └── app-prod.yml        # 🚀 Root Application declarativa do ArgoCD
├── infrastructure/             # 🌐 Manifestos globais (Namespace, Ingress, ClusterSecretStore)
│   ├── base/
│   └── overlays/prod/
├── apps/                       # 📦 Manifestos individuais de cada microsserviço
│   ├── auth-service/           # (Deployment, Service, ExternalSecret, ConfigMap)
│   ├── flag-service/
│   ├── targeting-service/
│   ├── evaluation-service/
│   └── analytics-service/
└── overlays/
    └── prod/                   # 🎯 Kustomization raiz de produção
        └── kustomization.yml
```

- **ArgoCD:** Configurado como serviço `ClusterIP` com reconciliação acelerada a cada **30 segundos** (`timeout.reconciliation: "30s"`).
- **Ingress NGINX:** Roteia tráfego externo para os serviços através de um Network Load Balancer (NLB) com `rewrite-target`.

---

## 🛡️ 7. Pipeline Multi-Stage DevSecOps (GitHub Actions)

Cada microsserviço possui uma pipeline automatizada (`.github/workflows/ci-*.yml`) estruturada em **6 estágios com Quality Gates rigorosos**:

```text
[Stage 1: Testes & Linters] ──┐
[Stage 1: SAST (Horusec)]    ──┼─► [Stage 2: Docker Build & Trivy Image Scan] ─► [Stage 3: Push ECR] ─► [Stage 4: GitOps Sync] ─► [Stage 5: ArgoCD Rollout]
[Stage 1: SCA (Trivy FS)]    ──┘
```

1. **Stage 1 (Paralelo):**
   - **Testes Unitários:** `go test` / `pytest` com mocks robustos de banco e AWS.
   - **Linters:** `golangci-lint` / `flake8`.
   - **SAST (Static Application Security Testing):** `Horusec CLI` analisando código estático e bloqueando vulnerabilidades.
   - **SCA (Software Composition Analysis):** `Trivy FS` analisando dependências (`go.mod`, `requirements.txt`) e falhando em severidades `CRITICAL`.
2. **Stage 2 (Fan-In):**
   - Construção da imagem Docker local e varredura com `Trivy Image Scan` contra vulnerabilidades de imagem base.
3. **Stage 3:** Push autenticado da imagem no **AWS ECR** com a tag do commit.
4. **Stage 4:** Atualização automática da imagem no manifesto `kustomization.yml` do GitOps com rebase resiliente.
5. **Stage 5:** O ArgoCD detecta a atualização e executa o rollout sincronizado no EKS.
6. **Stage 6:** Publicação do resumo consolidado do Quality Gate no GitHub Step Summary.

> 💡 **Pipeline Orquestradora:** O workflow [`.github/workflows/deploy-all.yml`](.github/workflows/deploy-all.yml) permite disparar o build e deploy dos 5 microsserviços simultaneamente com apenas 1 clique via `workflow_dispatch`.

---

## 🔑 8. Configuração de Secrets no GitHub Actions (Automação Total)

Para que todo o ecossistema de CI/CD e GitOps funcione de forma **100% automática e autônoma**, é necessário apenas cadastrar **4 Repository Secrets** no repositório do GitHub (`Settings > Secrets and variables > Actions`):

| Secret Name | Exemplo de Valor | Descrição |
| :--- | :--- | :--- |
| **`AWS_ACCESS_KEY_ID`** | `AKIAIOSFODNN7EXAMPLE` | Chave de acesso AWS com permissões para CI/CD (ECR, EKS, Terraform). |
| **`AWS_SECRET_ACCESS_KEY`** | `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` | Chave secreta da AWS correspondente. |
| **`AWS_REGION`** | `us-east-2` | Região da AWS onde toda a infraestrutura está provisionada. |
| **`PROJECT_NAME`** | `toggle-master-fase3` | Nome base do projeto usado dinamicamente para ECR, IAM e tags. |

> 🌟 **Como a mágica acontece:**
> Com apenas essas 4 variáveis cadastradas:
> 1. **Pipelines de CI/CD** rodam os testes, linters, SAST (Horusec), SCA e varredura de contêineres (Trivy).
> 2. As imagens Docker são construídas e enviadas automaticamente para os repositórios **AWS ECR** corretos (`${PROJECT_NAME}/${SERVICE_DIR}`).
> 3. Os manifestos do **GitOps** (`kustomization.yml`) são atualizados com a nova tag de imagem e commitados via workflow.
> 4. O **ArgoCD** detecta a alteração e executa o rollout sincronizado nos pods do **Amazon EKS**.
> 5. As credenciais e connection strings de banco de dados (RDS, Redis, SQS) são resolvidas dinamicamente via **AWS SSM Parameter Store** e **External Secrets Operator** sem qualquer intervenção manual.

---

## ⚡ 9. Guia Rápido de Execução e Comandos Úteis

### 🔹 1. Acesso ao Painel do ArgoCD:

```bash
# Iniciar o port-forward para o painel do ArgoCD:
kubectl kubectl port-forward --address 127.0.0.1 svc/argocd-server -n argocd 8080:443

# Obter a senha inicial do usuário 'admin':
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d; echo
# Acesse em: https://localhost:8080
```

### 🔹 2. Validar Status do Kubernetes & Secrets:

```bash
# Verificar se todos os 5 pods estão 1/1 Running:
kubectl get pods -n toggle-master

# Verificar se os ExternalSecrets sincronizaram com o AWS SSM:
kubectl get externalsecrets -n toggle-master

# Verificar as Secrets nativas criadas:
kubectl get secrets -n toggle-master
```

### 🔹 3. Testar Endpoints via Ingress NLB:

```bash
# Obter o Hostname do Ingress NLB:
export INGRESS_HOST=$(kubectl get svc ingress-nginx-controller -n ingress-nginx -o jsonpath='{.status.loadBalancer.ingress[0].hostname}')

# Testar a saúde de cada microsserviço:
curl -i http://$INGRESS_HOST/auth/health
curl -i http://$INGRESS_HOST/flags/health
curl -i http://$INGRESS_HOST/targeting/health
curl -i http://$INGRESS_HOST/evaluate-api/health
curl -i http://$INGRESS_HOST/analytics-api/health
```
