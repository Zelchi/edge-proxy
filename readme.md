# Edge Proxy (Go)

Edge Proxy é um reverse proxy / edge server escrito em Go, com foco em simplicidade, baixo consumo de recursos e suporte nativo a HTTPS automático via Let’s Encrypt (ACME).

Ele permite hospedar múltiplos SaaS/domínios em uma única VPS, atuando como ponto de entrada HTTP/HTTPS.
- - -
### Principais funcionalidades
- Reverse proxy baseado em host (Host header)
- HTTPS automático (Let’s Encrypt / ACME)
- Redirecionamento HTTP → HTTPS
- Rate limiting básico
- Suporte a múltiplos domínios
- Certificados persistentes via volume
- Imagem Docker pequena e segura (multi-stage, non-root)
- - -

### Lembre-se de criar uma network para compartilhar a conexão entre os containers fora do compose deles.
```bash
docker network create proxy_network
```