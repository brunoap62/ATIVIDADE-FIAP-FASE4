#!/usr/bin/env bash
# ==============================================================================
# Script de Simulação "A Prova Real" (Fase 4 — Incidente + PagerDuty + Discord + Self-Healing)
# ==============================================================================
# Uso:
#   ./scripts/test-chaos-self-healing.sh [auth-service|evaluation-service|flag-service|targeting-service]
# ==============================================================================

set -euo pipefail

TARGET_SERVICE="${1:-auth-service}"
NAMESPACE="toggle-master"
OBS_NAMESPACE="observability"

echo "=============================================================================="
echo " 🧪 INICIANDO A PROVA REAL: CAOS + ALERTA + PAGERDUTY + DISCORD + SELF-HEALING"
echo " 🎯 Microsserviço Alvo: ${TARGET_SERVICE}"
echo "=============================================================================="

INGRESS_HOST=$(kubectl get svc ingress-nginx-controller -n ingress-nginx -o jsonpath='{.status.loadBalancer.ingress[0].hostname}' 2>/dev/null || true)

if [[ -z "${INGRESS_HOST}" ]]; then
  echo "❌ Não foi possível obter o hostname do Ingress NGINX. Verifique a conexão com o cluster EKS."
  exit 1
fi

case "${TARGET_SERVICE}" in
  auth-service)
    CHAOS_URL="http://${INGRESS_HOST}/auth/chaos?enable=true"
    TRAFFIC_URL="http://${INGRESS_HOST}/auth/validate"
    HEALTH_URL="http://${INGRESS_HOST}/auth/health"
    ;;
  evaluation-service)
    CHAOS_URL="http://${INGRESS_HOST}/evaluate-api/chaos?enable=true"
    TRAFFIC_URL="http://${INGRESS_HOST}/evaluate-api/evaluate?user_id=user-chaos&flag_name=new-checkout"
    HEALTH_URL="http://${INGRESS_HOST}/evaluate-api/health"
    ;;
  flag-service)
    CHAOS_URL="http://${INGRESS_HOST}/flags-api/chaos?enable=true"
    TRAFFIC_URL="http://${INGRESS_HOST}/flags-api/flags"
    HEALTH_URL="http://${INGRESS_HOST}/flags-api/health"
    ;;
  targeting-service)
    CHAOS_URL="http://${INGRESS_HOST}/targeting-api/chaos?enable=true"
    TRAFFIC_URL="http://${INGRESS_HOST}/targeting-api/rules/new-checkout"
    HEALTH_URL="http://${INGRESS_HOST}/targeting-api/health"
    ;;
  *)
    echo "❌ Serviço inválido: ${TARGET_SERVICE}"
    exit 1
    ;;
esac

echo ""
echo "📌 1. Estado atual dos Pods antes da falha:"
kubectl get pods -n "${NAMESPACE}" -l "app=${TARGET_SERVICE}"

echo ""
echo "🔥 2. Ativando Modo Chaos (Falha Silenciosa de Negócio) em ${TARGET_SERVICE}:"
curl -s "${CHAOS_URL}" | sed 's/^/   /'
echo ""

echo "🩺 3. Confirmando que o /health continua 200 OK (LivenessProbe não mata o Pod prematuramente):"
HTTP_HEALTH=$(curl -s -o /dev/null -w "%{http_code}" "${HEALTH_URL}")
echo "   GET ${HEALTH_URL} -> HTTP ${HTTP_HEALTH}"

echo ""
echo "💥 4. Gerando tráfego por 40 segundos na rota de negócio para disparar o Alerta (5xx > 5% por 30s)..."
for i in $(seq 1 40); do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${TRAFFIC_URL}" || true)
  printf "\r   [Requisição %02d/40] GET %s -> HTTP %s" "${i}" "${TRAFFIC_URL}" "${STATUS}"
  sleep 1
done
echo ""

echo ""
echo "🚨 5. Verificando Logs do Controlador de Self-Healing (Runbook RB-001):"
kubectl logs -n "${OBS_NAMESPACE}" deploy/self-healing-webhook --tail=20 || true

echo ""
echo "🔄 6. Acompanhando os Pods do ${TARGET_SERVICE} sendo recriados pelo Self-Healing:"
kubectl get pods -n "${NAMESPACE}" -l "app=${TARGET_SERVICE}"

echo ""
echo "⏳ Aguardando 15 segundos para o novo Pod estabilizar e enviando tráfego 200 OK para Auto-Resolve..."
sleep 15
for i in $(seq 1 15); do
  STATUS=$(curl -s -o /dev/null -w "%{http_code}" "${TRAFFIC_URL}" || true)
  printf "\r   [Verificação Pós-Cura %02d/15] GET %s -> HTTP %s" "${i}" "${TRAFFIC_URL}" "${STATUS}"
  sleep 1
done
echo ""
echo "=============================================================================="
echo " ✅ Simulação concluída! Verifique:"
echo "    - Grafana Dashboard (Linha 2: Pico de erro 5xx zerado + Linha 1: Restart contabilizado)"
echo "    - PagerDuty (Incidente Triggered -> Auto-Resolved)"
echo "    - Discord (3 Cards: 🚨 FIRING -> 🛠️ SELF-HEALING EXECUTADO -> 🟢 RESOLVED)"
echo "=============================================================================="
