# Edge Proxy

Proxy de borda multiprotocolo em Go para expor serviços HTTP, HTTPS, TCP e UDP
por um único ponto de entrada. Inclui HTTP/3, certificados automáticos,
redirecionamento HTTP para HTTPS, rate limit, métricas e dashboard.

## Configuração

Edite o `config.yml`:

```yaml
rate_limit:
  requests_per_second: 1000
  burst: 3000

http:
  address: ":80"
  redirect_to_https: true
  dashboard: painel.exemplo.com
  fallback: app.exemplo.com
  routes:
    - host: app.exemplo.com
      upstream: http://application:3000

https:
  address: ":443"
  tls:
    certs_dir: /app/autocert
    certs_fallback: /app/certs
    domains:
      - painel.exemplo.com
      - app.exemplo.com

tcp:
  address: ":25000"
  routes:
    - host: service-a.example.com
      upstream: backend-a:25565
    - host: service-b.example.com
      upstream: backend-b:25565

udp:
  address: ":19000"
  routes:
    - host: service.example.com
      upstream: backend:19132
```

`http.dashboard`, `http.fallback` e os hosts de `http.routes` são strings
simples. O fallback sempre responde com `302 Found` e não possui status
configurável.

Rotas HTTP aceitam somente upstreams com `http://` ou `https://`. Por padrão,
o upstream recebe o próprio host interno; `preserve_host: true` mantém o host
público.

## TCP

O TCP é transparente depois da seleção da rota. Quando `routes` é usado, o
roteador padrão lê o preâmbulo inicial para obter o hostname e encaminha os
bytes originais. Um hostname desconhecido é rejeitado, a menos que exista
`default_upstream`:

```yaml
tcp:
  address: ":25000"
  routes:
    - host: service-a.example.com
      upstream: backend-a:25565
  default_upstream: backend-default:25565
```

Para encaminhar todo o tráfego para um único destino, use o roteador estático:

```yaml
tcp:
  address: ":27015"
  router: static
  upstream: game-server:27015
```

Upstreams TCP usam apenas `host:porta`, sem prefixo de esquema.

## UDP

O UDP cria uma sessão por endereço de cliente, encaminhando datagramas para o
upstream. As sessões expiram por inatividade e podem ser limitadas com
`max_sessions`:

```yaml
udp:
  address: ":19000"
  upstream: game-server:19132
  max_sessions: 4096
```

Também é possível declarar uma única rota UDP. O protocolo UDP não carrega o
hostname usado no DNS, portanto várias rotas por domínio na mesma porta não
podem ser distinguidas.

A porta UDP `443` é reservada ao HTTP/3.

## Docker e Podman

Publique no host as portas configuradas:

```yaml
ports:
  - "80:80"
  - "443:443/tcp"
  - "443:443/udp"
  - "25000:25000/tcp"
  - "19000:19000/udp"
```

Para serviços em outros containers, use o nome do serviço na mesma rede. Para
serviços no host, use `host.docker.internal` no Docker ou
`host.containers.internal` no Podman. `localhost` dentro do proxy é o próprio
container, não o host nem outro container.

Inicie com:

```bash
docker compose up -d --build
```

Ou:

```bash
podman compose up -d --build
```

## Certificados

Os domínios em `https.tls.domains` usam o cache automático do Let's Encrypt.
`https.tls.certs_fallback` deve conter `fullchain.pem` e `privkey.pem` quando
for necessário atender domínios cobertos por um certificado alternativo.

A pasta `certs/` na raiz é montada em `/app/certs` e não é incluída na imagem.

## Dashboard e métricas

- `GET /healthz` retorna `200 ok`;
- `GET /api/metrics` retorna métricas em JSON;
- `/api/metrics/stream` envia atualizações por SSE a cada cinco segundos;
- o dashboard exibe requisições HTTP, tráfego, erros e estatísticas TCP/UDP.

Endereços de clientes não são usados como labels de métricas.

## Reload

Alterações em rotas HTTP, fallback e `http.redirect_to_https` são recarregadas
automaticamente. Alterações em endereços, rate limit, TLS ou roteamento
TCP/UDP exigem reinício:

```bash
docker compose restart edge-proxy
```
