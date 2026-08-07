# Edge Proxy

Reverse proxy em Go para expor múltiplos serviços por domínio, com HTTPS automático, HTTP/3, redirecionamento HTTP → HTTPS, rate limit e dashboard simples.

## Requisitos

- Docker e Docker Compose
- DNS dos domínios apontando para o IP público do servidor
- Portas TCP `80` e `443`, e UDP `443`, liberadas

## Configuração rápida

Edite o [config.yml](config.yml):

```yaml
http:
  address: ":80"
  redirect_to_https: true

https:
  address: ":443"

rate_limit:
  requests_per_second: 100
  burst: 200

tls:
  # Cache dos certificados emitidos automaticamente pelo Let's Encrypt.
  certs_dir: /app/autocert
  domains:
    - app.exemplo.com
    - painel.exemplo.com
    - api.exemplo.com

  # Opcional: diretório com fullchain.pem e privkey.pem.
  # Necessário para aceitar e redirecionar subdomínios HTTPS desconhecidos.
  certs_fallback: /app/certs

dashboard:
  host: painel.exemplo.com

fallback:
  host: app.exemplo.com
  status_code: 302

routes:
  - host: app.exemplo.com
    upstream: http://app:8080
  - host: api.exemplo.com
    upstream: http://api:3000
    preserve_host: false
```

Regras importantes:

- Todo `routes[].host` HTTPS precisa constar em `tls.domains`.
- `dashboard.host` também precisa constar em `tls.domains`.
- Hosts são tratados em minúsculas e sem ponto final.
- Os upstreams aceitam somente `http://` ou `https://`.
- `preserve_host: true` envia o host público original ao upstream. Por padrão, o upstream recebe o próprio host interno.

### Onde está o upstream?

- Outro serviço no mesmo `compose.yml`: use o nome do serviço, por exemplo `http://api:3000`.
- Serviço executando no host do servidor, ou porta publicada por outro container: use `http://host.docker.internal:3000`.

O Compose mapeia `host.docker.internal` para o gateway do host no Linux. O
Podman também disponibiliza esse nome e o alias `host.containers.internal`.
Evite `172.17.0.1`: esse IP depende da configuração da bridge e pode mudar.

## Iniciar com Docker Compose

Suba o proxy:

```bash
docker compose up -d --build
```

Com Podman, use o equivalente:

```bash
podman compose up -d --build
```

Verifique os logs:

```bash
docker compose logs -f edge-proxy
```

O Compose usa automaticamente a rede bridge padrão do projeto.

O Compose expõe:

- HTTP em TCP `80`
- HTTPS/HTTP2 em TCP `443`
- HTTP/3 em UDP `443`

## Certificados

### Domínios configurados

Para os domínios em `tls.domains`, o proxy obtém e guarda certificados Let's Encrypt automaticamente. O desafio HTTP-01 usa a porta 80; os domínios precisam resolver para o servidor.

### Certificado curinga para fallback HTTPS

Um host HTTPS desconhecido só pode receber redirect se o proxy tiver um certificado válido para ele. Para isso, emita um certificado curinga, por exemplo para `exemplo.com` e `*.exemplo.com`, usando DNS-01:

```bash
sudo certbot certonly --manual --preferred-challenges dns \
  -d exemplo.com -d '*.exemplo.com'
```

O Certbot exibirá valores TXT para `_acme-challenge.exemplo.com`. No painel do
provedor DNS, o campo **Host** normalmente deve ser apenas `_acme-challenge`.

Após emitir ou renovar, copie os PEMs para a pasta `certs` na raiz do projeto:

```bash
sudo install -m 644 /etc/letsencrypt/live/exemplo.com/fullchain.pem certs/fullchain.pem
sudo install -m 600 /etc/letsencrypt/live/exemplo.com/privkey.pem certs/privkey.pem
```

A pasta `certs` é montada no container como `/app/certs`, não é incluída na imagem e é ignorada pelo Git. O proxy detecta alterações nesses dois arquivos e usa o certificado novo em novas conexões TLS, sem rebuild ou reinício.

Certificados emitidos com `certbot --manual` não renovam sozinhos. Repita o processo antes da data de expiração e copie novamente os dois arquivos.

## Fallback

Quando um host não possui rota, o proxy redireciona para `fallback.host`, preservando caminho e query string:

```text
http://desconhecido.exemplo.com/docs?lang=pt
→ https://app.exemplo.com/docs?lang=pt
```

Em HTTPS, isso funciona para hosts cobertos pelo certificado curinga configurado em `certs_fallback`. Sem ele, o navegador encerra a conexão durante o handshake TLS antes de qualquer redirect HTTP ser possível.

O próprio `fallback.host` não é redirecionado novamente, evitando loop. Ele precisa ter uma rota ou ser atendido pelo dashboard.

## Health e dashboard

- `GET /healthz` retorna `200 ok` por HTTP ou HTTPS.
- O host definido em `dashboard.host` exibe o dashboard.
- `GET /api/metrics` retorna métricas em JSON no host do dashboard.

## Reload de configuração

Alterações em `routes`, `fallback` e `http.redirect_to_https` são recarregadas automaticamente.

Alterações em endereços de escuta, rate limit ou configuração TLS exigem reiniciar o container:

```bash
docker compose up -d --build
```

## Testes

```bash
go test ./...
go test -race ./...
```
