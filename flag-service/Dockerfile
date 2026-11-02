# ==============================================================================
# Estágio 1: Builder - Instalar dependências de forma limpa
# ==============================================================================
FROM python:3.12-slim-bookworm AS builder

WORKDIR /app

# Atualizar o pip e instalar ferramentas de compilação se necessário
RUN pip install --no-cache-dir --upgrade pip

# Copiar apenas os requisitos para aproveitar o cache de camadas do Docker
COPY requirements.txt .

# Instalar dependências no diretório do usuário root local
# O uso de --user nos permite facilmente copiar todas as libs instaladas no segundo estágio
RUN pip install --no-cache-dir --user -r requirements.txt


# ==============================================================================
# Estágio 2: Imagem final leve e segura para execução (Debian 12 Bookworm LTS)
# ==============================================================================
FROM python:3.12-slim-bookworm

# Atualiza pacotes do sistema operacional para garantir zero vulnerabilidades na imagem base
RUN apt-get update && apt-get upgrade -y && rm -rf /var/lib/apt/lists/*

# Definir variáveis de ambiente recomendadas para o Python em containers:
# - PYTHONDONTWRITEBYTECODE: Impede a escrita de arquivos .pyc otimizando o espaço em disco
# - PYTHONUNBUFFERED: Garante o envio imediato de logs para o console sem buffering
ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PATH=/home/appuser/.local/bin:$PATH

# Criar um grupo e usuário de sistema não-root por questões de segurança (Princípio de Privilégio Mínimo)
RUN groupadd -g 10001 appgroup && \
    useradd -u 10001 -g appgroup -m -d /home/appuser -s /sbin/nologin appuser

WORKDIR /app

# Copiar as dependências instaladas no builder para a pasta do novo usuário
COPY --from=builder /root/.local /home/appuser/.local

# Copiar o código fonte do serviço
COPY app.py .

# Ajustar as permissões de propriedade para o usuário não-root
RUN chown -R appuser:appgroup /app /home/appuser

# Usar o usuário não-root criado
USER appuser

# Expor a porta que a aplicação escuta por padrão
EXPOSE 8002

# Rodar a aplicação utilizando o servidor de produção Gunicorn
CMD ["gunicorn", "--bind", "0.0.0.0:8002", "--workers", "4", "--threads", "2", "--access-logfile", "-", "--error-logfile", "-", "app:app"]
