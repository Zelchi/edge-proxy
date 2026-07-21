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

Configure pelo `config.yml`. Consulte a seção de reload para saber quais
alterações exigem reinício do container.

### Configuração

```yaml
http:
  address: ":80"
  redirect_to_https: true

https:
  address: ":443"

tls:
  certs_dir: /app/certs
  domains:
    - app.exemplo.com

# Opcional: hosts sem rota são redirecionados para este domínio.
fallback:
  host: app.exemplo.com
  status_code: 302

routes:
  - host: app.exemplo.com
    upstream: http://app:8080
    # Opcional. Por padrão o upstream recebe seu host interno.
    preserve_host: false
```

Todos os hosts são normalizados para minúsculas, sem ponto final. Cada host de
rota HTTPS precisa estar em `tls.domains`; upstreams aceitam somente `http` ou
`https` e precisam ter host. Um `fallback.host` deve ser uma rota ou domínio TLS
conhecido.

O fallback preserva path e query string. Em HTTPS, um host desconhecido precisa
ter certificado válido antes do redirecionamento ser possível; por isso, para
subdomínios variáveis, use domínios explícitos ou uma solução de certificado
curinga com DNS-01.

### Uso rápido

1. Ajuste `config.yml` com os domínios, upstreams e, se desejar, o fallback.
2. Garanta que os DNS dos domínios apontem para este servidor e que TCP/80,
   TCP/443 e UDP/443 estejam liberados.
3. Inicie com Compose:

   ```bash
   docker network create proxy_network
   docker compose up -d --build
   ```

4. Verifique a disponibilidade:

   ```bash
   curl -i http://localhost/healthz
   ```

Para executar sem Compose, crie a imagem e monte a configuração e um diretório
persistente para os certificados:

```bash
docker build -t edge-proxy .
docker run -d --name edge-proxy \
  -p 80:80 -p 443:443 -p 443:443/udp \
  -v "$(pwd)/config.yml:/app/config.yml:ro" \
  -v edge-proxy-certs:/app/certs \
  edge-proxy
```

### Reload de configuração

O proxy recarrega alterações de `routes`, `fallback` e
`http.redirect_to_https` sem interromper requisições em andamento. Alterações em
`http.address`, `https.address`, `tls.certs_dir` ou `tls.domains` exigem reinício
do container; o reload será rejeitado e a configuração anterior continuará ativa.

### Protocolos HTTP

O proxy disponibiliza HTTP/1.1 e HTTP/2 em TCP/443, além de HTTP/3 em UDP/443.
O navegador negocia o melhor protocolo disponível: HTTP/3 é anunciado por
`Alt-Svc`, mas clientes sem QUIC/UDP usam HTTP/2 ou HTTP/1.1 automaticamente.

### Caminho do upstream

Um caminho em `upstream` é usado como prefixo. Por exemplo,
`upstream: http://api:8080/base` encaminha uma requisição para `/users` ao
backend como `/base/users`. Query string e fragmento não são aceitos no
upstream; a query da requisição do cliente é preservada.

### Health check

`GET /healthz` responde `200 ok` em HTTP ou HTTPS sem expor configuração ou
estado interno. Use-o para liveness/readiness no Docker ou no balanceador.

### Dashboard

O host definido em `dashboard.host` serve uma página somente leitura com
requisições, erros 5xx e transferência de entrada/saída. Acesse o host
configurado em `dashboard.host`; a API de métricas fica em `GET /api/metrics` e
não aceita métodos de alteração.

### Imagem Docker

A imagem já inclui o `config.yml` presente no build e pode ser executada sem
Compose. Monte outro arquivo em `/app/config.yml` para substituir a configuração
em runtime; o Compose já faz isso automaticamente.
