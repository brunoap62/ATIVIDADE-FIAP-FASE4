# ==============================================================================
# Estágio 1: Builder - Instalar dependências em ambiente virtual Alpine
# ==============================================================================
FROM python:3.12-alpine AS builder

WORKDIR /app

# Instalar dependências de compilação mínimas
RUN apk add --no-cache gcc musl-dev libffi-dev

# Copiar requirements e instalar no ambiente virtual
COPY requirements.txt .
RUN python -m venv /opt/venv && \
    /opt/venv/bin/pip install --no-cache-dir --upgrade pip && \
    /opt/venv/bin/pip install --no-cache-dir -r requirements.txt


# ==============================================================================
# Estágio 2: Imagem final Alpine ultra-segura e minimalista (Zero CVEs de SO)
# ==============================================================================
FROM python:3.12-alpine

# Atualiza pacotes do sistema operacional no Alpine para mitigar CVEs de imagem base
RUN apk upgrade --no-cache

# Definir variáveis de ambiente recomendadas
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH="/opt/venv/bin:$PATH"

WORKDIR /app

# Criar grupo e usuário não-root no Alpine
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

# Copiar apenas o virtualenv limpo do builder
COPY --from=builder /opt/venv /opt/venv

# Copiar o código da aplicação e módulo de telemetria OpenTelemetry
COPY app.py telemetry.py ./

# Ajustar propriedade dos arquivos
RUN chown -R appuser:appgroup /app

# Usar usuário não-root
USER appuser

# Expor a porta que a aplicação escuta por padrão
EXPOSE 8005

# Rodar a aplicação utilizando Gunicorn
CMD ["gunicorn", "--bind", "0.0.0.0:8005", "--workers", "2", "--threads", "2", "--access-logfile", "-", "--error-logfile", "-", "app:app"]
