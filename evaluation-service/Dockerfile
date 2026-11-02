# Estágio 1: Compilar o binário Go
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copiar os arquivos de dependência primeiro para aproveitar o cache de camadas do Docker
COPY go.mod go.sum ./
RUN go mod download

# Copiar o restante do código-fonte da aplicação
COPY . .

# Compilar um binário Go estaticamente ligado
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o evaluation-service .

# Estágio 2: Criar uma imagem de execução mínima
FROM alpine:3.21

# Atualizar pacotes do SO e instalar certificados CA e dados de timezone
RUN apk upgrade --no-cache && apk add --no-cache ca-certificates tzdata

# Criar um usuário de sistema não-root por segurança
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copiar o binário compilado do estágio de build
COPY --from=builder /app/evaluation-service /app/evaluation-service

# Garantir que o usuário não-root tenha as permissões necessárias
RUN chown -R appuser:appgroup /app

# Usar o usuário não-root
USER appuser

# Expor a porta da aplicação
EXPOSE 8004

# Comando para rodar o serviço
CMD ["./evaluation-service"]
