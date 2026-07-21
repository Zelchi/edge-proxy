package server

import (
	"encoding/json"
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
		default:
			http.NotFound(w, r)
		}
	})
}

func IsDashboardHost(configuredHost, requestHost string) bool {
	if host, _, err := net.SplitHostPort(requestHost); err == nil {
		requestHost = host
	}
	return strings.EqualFold(strings.TrimRight(configuredHost, "."), strings.TrimRight(requestHost, "."))
}

const dashboardHTML = `<!doctype html><html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Edge Proxy · Uso</title><style>body{margin:0;background:#101722;color:#eaf0ff;font:16px system-ui,sans-serif}.wrap{max-width:1000px;margin:48px auto;padding:0 20px}h1{margin:0;font-size:28px}.sub{color:#9baac4}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:16px;margin-top:28px}.card{background:#182231;border:1px solid #27364d;border-radius:14px;padding:20px}.label{color:#9baac4;font-size:13px}.value{font-size:30px;font-weight:700;margin-top:9px}.ok{color:#65d69a}footer{color:#72819b;margin-top:24px;font-size:13px}</style></head><body><main class="wrap"><h1>Edge Proxy</h1><p class="sub">Uso de rede e transferência em tempo real</p><section class="grid"><article class="card"><div class="label">Requisições</div><div class="value" id="requests">—</div></article><article class="card"><div class="label">Erros 5xx</div><div class="value" id="errors">—</div></article><article class="card"><div class="label">Entrada</div><div class="value" id="in">—</div></article><article class="card"><div class="label">Saída</div><div class="value" id="out">—</div></article></section><footer><span class="ok">●</span> Atualiza a cada 3 segundos</footer></main><script>const f=n=>{const u=['B','KB','MB','GB','TB'];let i=0;while(n>=1024&&i<u.length-1){n/=1024;i++}return n.toFixed(i?2:0)+' '+u[i]};async function load(){try{const m=await fetch('/api/metrics',{cache:'no-store'}).then(r=>r.json());requests.textContent=m.requests.toLocaleString('pt-BR');errors.textContent=m.errors.toLocaleString('pt-BR');in.textContent=f(m.in_bytes);out.textContent=f(m.out_bytes)}catch{}}load();setInterval(load,3000)</script></body></html>`
