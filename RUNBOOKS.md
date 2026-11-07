# 📖 Catálogo Oficial de Runbooks SRE — ToggleMaster (Fase 4)

Este documento contém os procedimentos operacionais padronizados (**Runbooks**) vinculados aos alertas inteligentes do **Prometheus / Alertmanager** e executados automaticamente pelo motor de **Self-Healing (`self-healing-webhook`)** no cluster Amazon EKS.

---

## 🚨 RB-001: Mitigação Automática de Alta Taxa de Erros HTTP 5xx (`ToggleMasterHighErrorRate5xx`)

### 1. Identificação do Alerta
- **Nome da Regra:** `ToggleMasterHighErrorRate5xx`
- **Expressão PromQL:**
  ```promql
  (
    sum by (service_name) (rate(traces_spanmetrics_calls_total{http_status_code=~"5.."}[1m]))
    /
    clamp_min(sum by (service_name) (rate(traces_spanmetrics_calls_total[1m])), 0.0001)
  ) * 100 > 5
  ```
- **Janela de Sustentação (`for`):** `30s` (Prevenção de *Alert Fatigue* contra falhas transitórias de rede).
- **Severidade:** `critical`
- **Automação de Self-Healing:** `Habilitada (self_healing="true")`

### 2. Diagnóstico Rápido (Golden Signals + LGTM)
1. **No Dashboard Grafana (`ToggleMaster — SRE Golden Signals & Live Troubleshooting`):**
   - Selecione o microsserviço afetado no filtro `$service` no topo.
   - Na **Linha 2 (Esquerda)**, verifique o pico no gráfico de **Taxa de Erros HTTP 5xx (%)**.
   - Na **Linha 2 (Direita, ao lado)**, inspecione o painel de **Logs de Erro em Tempo Real (Loki)** para ler a mensagem de erro e copiar o `trace_id`.
   - Na **Linha 3 (Direita)**, abra o `trace_id` no **Grafana Tempo** para visualizar o Span exato que falhou no **Service Map**.

### 3. Fluxo de Execução Automatizada (Runbook Automation / Self-Healing)
Quando o alerta entra em estado **`FIRING`**, o **Alertmanager** aciona simultaneamente o **PagerDuty** (`Events API v2`) e o controlador interno **`self-healing-webhook`**, que executa os 4 passos abaixo sem intervenção humana:

1. **Passo 1 — Validação de Escopo:** Confirma que o serviço pertence à lista autorizada (`auth-service`, `flag-service`, `targeting-service`, `evaluation-service`, `analytics-service`) no namespace `toggle-master`.
2. **Passo 2 — Guardrail de Cooldown (60s):** Verifica se o deployment já sofreu restart nos últimos 60 segundos para evitar loop de reinicialização durante o *RollingUpdate*.
3. **Passo 3 — Mitigação via Kubernetes API (`Rollout Restart`):**
   - Executa `PATCH` autenticado via RBAC (`ServiceAccount: self-healing-sa`) no recurso `Deployment`:
     ```bash
     kubectl rollout restart deployment/<service_name> -n toggle-master
     ```
   - O Kubernetes cria um novo Pod limpo, aguarda o `readinessProbe` (`GET /health -> 200 OK`) e encerra o Pod degradado sem downtime.
4. **Passo 4 — Auditoria em ChatOps (Discord) e PagerDuty:**
   - Envia Rich Embed Azul no canal do **Discord** registrando o comando executado e o horário do restart.
   - Assim que a taxa de erros 5xx retorna a `0%`, o alerta entra em **`RESOLVED`**, fechando automaticamente o incidente no **PagerDuty** e enviando o Rich Embed Verde no **Discord**.

---

## ⚠️ RB-002: Diagnóstico e Mitigação de Latência p95 Elevada (`ToggleMasterHighLatencyP95`)

### 1. Identificação do Alerta
- **Nome da Regra:** `ToggleMasterHighLatencyP95`
- **Expressão PromQL:**
  ```promql
  histogram_quantile(0.95, sum by (le, service_name) (rate(traces_spanmetrics_latency_bucket[2m]))) > 1000
  ```
- **Janela de Sustentação (`for`):** `1m`
- **Severidade:** `warning` (Inibido automaticamente pelo Alertmanager caso `ToggleMasterHighErrorRate5xx` esteja ativo para o mesmo serviço).

### 2. Procedimento de Verificação
1. Verifique na **Linha 3 do Dashboard Grafana** qual dependência apresenta maior duração nos Spans do **Grafana Tempo** (`RDS PostgreSQL`, `ElastiCache Redis`, `AWS SQS` ou `DynamoDB`).
2. Verifique na **Linha 4 (Saturação & HPA)** se o **HPA** já escalou as réplicas do serviço ou se há *throttling* de CPU nos Pods.
