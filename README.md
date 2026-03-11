# hack-fiap233-videos

Microsserviço de vídeos em Go.

## Endpoints

| Método | Rota | Descrição |
|---|---|---|
| GET | `/videos/health` | Health check + status do banco |
| GET | `/videos/` | Listar vídeos |
| POST | `/videos/` | Criar vídeo (`{"title":"...","description":"..."}`) |

## Rodar localmente

```bash
go run main.go
```

## Deploy

O deploy é automático via GitHub Actions. Qualquer push na `main` executa:

1. Build da imagem Docker
2. Push para o ECR
3. Deploy no cluster EKS

### Secrets necessárias no GitHub

| Secret | Descrição |
|---|---|
| `AWS_ACCESS_KEY_ID` | Access Key da AWS Academy |
| `AWS_SECRET_ACCESS_KEY` | Secret Key da AWS Academy |
| `AWS_SESSION_TOKEN` | Session Token da AWS Academy |
