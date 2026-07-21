package server

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"

	"edge-proxy/internal/proxy"
)

func WithDashboard(host string, metrics *proxy.Metrics, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsDashboardHost(host, r.Host) {
			next.ServeHTTP(w, r)
			return
		}
		switch r.URL.Path {
		case "/", "/index.html":
			if r.Method != http.MethodGet {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(dashboardHTML))
		case "/api/metrics":
			if r.Method != http.MethodGet {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(metrics.Snapshot())
		case "/api/metrics/stream":
			if r.Method != http.MethodGet {
				http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
				return
			}
			serveMetricsStream(w, r, metrics)
		default:
			http.NotFound(w, r)
		}
	})
}

func serveMetricsStream(w http.ResponseWriter, r *http.Request, metrics *proxy.Metrics) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	controller := http.NewResponseController(w)
	subscription, unsubscribe := metrics.Subscribe()
	defer unsubscribe()
	for {
		select {
		case snapshot := <-subscription:
			data, err := json.Marshal(snapshot)
			if err != nil { return }
			if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil { return }
			if err := controller.Flush(); err != nil { return }
		case <-r.Context().Done():
			return
		}
	}
}

func IsDashboardHost(configuredHost, requestHost string) bool {
	if host, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = host
	}
	return strings.EqualFold(strings.TrimRight(configuredHost, "."), strings.TrimRight(requestHost, "."))
}

const dashboardHTML = `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 64 64'%3E%3Crect width='64' height='64' rx='16' fill='%23182231'/%3E%3Cpath d='M16 24h32M16 32h32M16 40h20' stroke='%2365d69a' stroke-width='5' stroke-linecap='round'/%3E%3Ccircle cx='45' cy='40' r='5' fill='%23eaf0ff'/%3E%3C/svg%3E"><title>Edge Proxy</title><style>body{margin:0;min-height:100vh;display:grid;place-items:center;background:#101722;color:#eaf0ff;font:16px system-ui,sans-serif}.grid{display:grid;grid-template-columns:repeat(2,minmax(180px,260px));gap:16px;padding:20px}.card{background:#182231;border:1px solid #27364d;border-radius:14px;padding:22px}.label{color:#9baac4;font-size:13px}.value{font-size:30px;font-weight:700;margin-top:9px}@media(max-width:430px){.grid{grid-template-columns:minmax(180px,260px)}}</style></head><body><section class="grid"><article class="card"><div class="label">Requisições</div><div class="value" id="requests">—</div></article><article class="card"><div class="label">Erros 5xx</div><div class="value" id="errors">—</div></article><article class="card"><div class="label">Entrada</div><div class="value" id="in-bytes">—</div></article><article class="card"><div class="label">Saída</div><div class="value" id="out-bytes">—</div></article></section><script>const $=id=>document.getElementById(id),f=n=>{const u=['B','KB','MB','GB','TB'];let i=0;while(n>=1024&&i<u.length-1){n/=1024;i++}return n.toFixed(i?2:0)+' '+u[i]};const stream=new EventSource('/api/metrics/stream');stream.onmessage=e=>{const m=JSON.parse(e.data);$('requests').textContent=m.requests.toLocaleString('pt-BR');$('errors').textContent=m.errors.toLocaleString('pt-BR');$('in-bytes').textContent=f(m.in_bytes);$('out-bytes').textContent=f(m.out_bytes)}</script></body></html>`
