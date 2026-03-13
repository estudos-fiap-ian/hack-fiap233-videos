# hack-fiap233-videos

Microsserviço de gerenciamento de vídeos escrito em Go. Responsável por receber uploads de vídeo, persistir metadados no PostgreSQL, armazenar arquivos no S3 e publicar eventos no SNS para processamento assíncrono downstream.

## Sumário

- [Arquitetura](#arquitetura)
- [Endpoints](#endpoints)
- [Upload de Vídeo — Detalhado](#upload-de-vídeo--detalhado)
- [Variáveis de Ambiente](#variáveis-de-ambiente)
- [Rodar Localmente](#rodar-localmente)
- [Testes](#testes)
- [Deploy](#deploy)
- [Kubernetes](#kubernetes)
- [Observabilidade](#observabilidade)

---

## Arquitetura

O serviço segue o padrão **Hexagonal (Ports & Adapters)**, isolando a lógica de negócio de qualquer detalhe de infraestrutura.

```
┌─────────────────────────────────────────────────────────────┐
│                        HTTP Handler                         │
│                  (adapter/http/handler.go)                  │
└────────────────────────────┬────────────────────────────────┘
                             │
┌────────────────────────────▼────────────────────────────────┐
│                       VideoService                          │
│                  (application/service.go)                   │
└──────────┬──────────────────────────────┬───────────────────┘
           │                              │
  ┌────────▼────────┐           ┌─────────▼──────────────────┐
  │   PostgreSQL    │           │       AWS S3 + SNS          │
  │  (repository)   │           │   (storage + event)         │
  └─────────────────┘           └────────────────────────────┘
```

**Camadas:**

| Camada | Pacote | Responsabilidade |
|--------|--------|-----------------|
| Domain | `internal/domain` | Entidades (`Video`, `VideoEvent`) e interfaces (ports) |
| Application | `internal/application` | Orquestração do fluxo de negócio |
| Adapters | `internal/adapter/` | Implementações concretas: HTTP, PostgreSQL, S3, SNS |
| Middleware | `internal/middleware` | Métricas Prometheus cross-cutting |

---

## Endpoints

| Método | Rota | Descrição | Auth |
|--------|------|-----------|------|
| `GET` | `/videos/health` | Health check + conectividade com o banco | — |
| `GET` | `/videos/` | Listar todos os vídeos | — |
| `POST` | `/videos/` | Criar metadados de vídeo (sem arquivo) | — |
| `GET` | `/videos/{id}` | Buscar vídeo por ID | — |
| `GET` | `/videos/me` | Listar vídeos do usuário autenticado | JWT |
| `POST` | `/videos/upload` | **Upload completo de vídeo (multipart)** | JWT (opcional) |

### Health Check

```
GET /videos/health
```

```json
{
  "status": "ok",
  "service": "videos",
  "db": "connected"
}
```

---

## Upload de Vídeo — Detalhado

```
POST /videos/upload
Content-Type: multipart/form-data
Authorization: Bearer <JWT>   (opcional)
```

### Campos da requisição

| Campo | Tipo | Obrigatório | Descrição |
|-------|------|-------------|-----------|
| `title` | `string` (form field) | Sim | Título do vídeo |
| `description` | `string` (form field) | Não | Descrição do vídeo |
| `video` | `file` (binary) | Sim | Arquivo de vídeo (max 32 MB por parse de form) |

### Fluxo interno

```
POST /videos/upload
       │
       ├─ 1. Parse multipart form
       ├─ 2. Extrai title, description, arquivo de vídeo
       ├─ 3. Decodifica JWT → extrai e-mail do usuário (sem verificação de assinatura;
       │      a validação é feita pelo API Gateway antes de chegar aqui)
       │
       └─ VideoService.UploadVideo()
              │
              ├─ 4. Salva registro no PostgreSQL com status = "pending"
              ├─ 5. Faz upload do arquivo para S3
              │       Caminho: videos/{id}/{filename}
              ├─ 6. Atualiza o registro no banco com a S3 key
              └─ 7. Publica evento no SNS (JSON com metadados do vídeo)
```

> Falhas nos passos 6 e 7 são logadas mas **não retornam erro ao cliente** — o upload
> é considerado bem-sucedido assim que o arquivo está no S3.

### Resposta de sucesso

```
HTTP 202 Accepted
```

```json
{
  "job_id": 42,
  "status": "pending",
  "message": "video upload received, processing will start shortly"
}
```

O status `pending` indica que o vídeo foi recebido e está aguardando processamento pelo consumidor do SNS (e.g. serviço de transcodificação).

### Respostas de erro

| Status | Cenário |
|--------|---------|
| `400 Bad Request` | Ausência de `title`, multipart inválido ou arquivo `video` não enviado |
| `405 Method Not Allowed` | Método HTTP diferente de `POST` |
| `500 Internal Server Error` | Falha no banco de dados ou no upload para o S3 |

### Evento SNS publicado

Ao concluir o upload, o seguinte payload é publicado no tópico SNS configurado:

```json
{
  "id": 42,
  "title": "Meu Vídeo",
  "description": "Descrição opcional",
  "s3_key": "videos/42/meu-video.mp4",
  "user_email": "usuario@email.com",
  "status": "pending",
  "created_at": "2026-03-13T12:00:00Z"
}
```

---

## Variáveis de Ambiente

| Variável | Descrição | Exemplo |
|----------|-----------|---------|
| `DB_HOST` | Host do PostgreSQL | `postgres-svc` |
| `DB_PORT` | Porta do PostgreSQL | `5432` |
| `DB_USER` | Usuário do banco | `videos` |
| `DB_PASSWORD` | Senha do banco | `secret` |
| `DB_NAME` | Nome do banco | `videos` |
| `S3_BUCKET` | Nome do bucket S3 | `hack-fiap233-videos` |
| `SNS_TOPIC_ARN` | ARN do tópico SNS | `arn:aws:sns:us-east-1:123456:videos-topic` |
| `AWS_REGION` | Região AWS | `us-east-1` |
| `AWS_ACCESS_KEY_ID` | Access Key AWS | — |
| `AWS_SECRET_ACCESS_KEY` | Secret Key AWS | — |
| `AWS_SESSION_TOKEN` | Session Token AWS (Academy) | — |

---

## Rodar Localmente

### Pré-requisitos

- Go 1.22+
- PostgreSQL rodando localmente (ou via Docker)
- Credenciais AWS com acesso a S3 e SNS

### Subir dependências com Docker Compose

```bash
docker run -d \
  --name postgres \
  -e POSTGRES_USER=videos \
  -e POSTGRES_PASSWORD=secret \
  -e POSTGRES_DB=videos \
  -p 5432:5432 \
  postgres:15-alpine
```

### Executar

```bash
export DB_HOST=localhost DB_PORT=5432 DB_USER=videos DB_PASSWORD=secret DB_NAME=videos
export S3_BUCKET=meu-bucket SNS_TOPIC_ARN=arn:aws:sns:us-east-1:123456:topic
export AWS_REGION=us-east-1

go run main.go
```

O servidor sobe na porta `8080`.

### Testar o upload localmente

```bash
curl -X POST http://localhost:8080/videos/upload \
  -H "Authorization: Bearer <JWT>" \
  -F "title=Meu Vídeo de Teste" \
  -F "description=Descrição do vídeo" \
  -F "video=@/caminho/para/video.mp4"
```

---

## Testes

```bash
# Rodar todos os testes
go test ./...

# Com cobertura (mínimo exigido pelo CI: 80%)
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Os testes cobrem todas as camadas com mocks das interfaces de domínio — nenhum serviço externo é necessário para rodar os testes unitários.

---

## Deploy

O deploy é **automático via GitHub Actions** a cada push na branch `main`.

### Pipeline de deploy (`.github/workflows/deploy.yml`)

```
push main
    │
    ├─ 1. Build da imagem Docker (multi-stage, binário estático)
    ├─ 2. Push para o ECR (tags: latest + git SHA)
    ├─ 3. Atualiza Kubernetes Secrets a partir do AWS Secrets Manager
    ├─ 4. Atualiza ConfigMap com S3_BUCKET e SNS_TOPIC_ARN
    ├─ 5. Aplica os manifestos k8s (deployment + service)
    └─ 6. Rollout da nova imagem com timeout de 5 minutos
```

### Secrets necessárias no GitHub

| Secret | Descrição |
|--------|-----------|
| `AWS_ACCESS_KEY_ID` | Access Key da AWS Academy |
| `AWS_SECRET_ACCESS_KEY` | Secret Key da AWS Academy |
| `AWS_SESSION_TOKEN` | Session Token da AWS Academy |

### Validação de PR (`.github/workflows/pr-validation.yml`)

Todo PR para `main` executa automaticamente:

- Testes + cobertura mínima de 80%
- Build da imagem Docker (sem push)
- Dry-run dos manifestos Kubernetes
- Validação de acesso ao EKS e Secrets Manager

---

## Kubernetes

O serviço roda no EKS com os seguintes recursos:

### Deployment

```yaml
replicas: 2
image: 092361660280.dkr.ecr.us-east-1.amazonaws.com/hack-fiap233-videos:latest
resources:
  requests: { cpu: 100m, memory: 128Mi }
  limits:   { cpu: 250m, memory: 256Mi }
```

**Health probes:**

| Probe | Rota | Inicial | Período |
|-------|------|---------|---------|
| Liveness | `/videos/health` | 10s | 10s |
| Readiness | `/videos/health` | 5s | 5s |

### Service

```
Tipo: NodePort
Porta interna: 8080
NodePort: 30082
```

### ConfigMap e Secrets

| Recurso | Chave | Origem |
|---------|-------|--------|
| Secret `videos-db-credentials` | `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | AWS Secrets Manager |
| ConfigMap `videos-config` | `S3_BUCKET`, `SNS_TOPIC_ARN` | Variável no workflow |

---

## Observabilidade

O serviço expõe métricas no padrão **Prometheus** via `/metrics`, coletadas automaticamente pelo scraper configurado nas annotations do Deployment.

### Métricas disponíveis

| Métrica | Tipo | Descrição |
|---------|------|-----------|
| `http_requests_total` | Counter | Total de requisições por método, rota e status HTTP |
| `http_request_duration_seconds` | Histogram | Latência das requisições HTTP |
