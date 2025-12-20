# Edge Proxy (Go)

Edge Proxy é um reverse proxy / edge server escrito em Go, com foco em simplicidade, baixo consumo de recursos e suporte nativo a HTTPS automático via Let’s Encrypt (ACME).

- - -
### Principais funcionalidades
- Reverse proxy baseado em host (Host header)
- HTTPS automático (Let’s Encrypt / ACME)
- Redirecionamento HTTP → HTTPS
- Rate limiting básico
- Suporte a múltiplos domínios
- Certificados persistentes via volume
- - -

### Crie uma network docker para compartilhar a rede entre eles
```bash
docker network create proxy_network
```