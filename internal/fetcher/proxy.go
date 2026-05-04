package fetcher

import (
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
)

// egressProxy is an HTTP CONNECT proxy that forwards connections only to whitelisted IPs.
type egressProxy struct {
	server     *http.Server
	listener   net.Listener
	allowedIPs map[string]struct{}
}

func startProxy(allowedIPs []string) (*egressProxy, error) {
	set := make(map[string]struct{}, len(allowedIPs))
	for _, ip := range allowedIPs {
		set[ip] = struct{}{}
	}

	ln, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		return nil, fmt.Errorf("proxy listen: %w", err)
	}

	p := &egressProxy{listener: ln, allowedIPs: set}
	p.server = &http.Server{Handler: p}
	go p.server.Serve(ln) //nolint:errcheck
	return p, nil
}

func (p *egressProxy) port() int {
	return p.listener.Addr().(*net.TCPAddr).Port
}

func (p *egressProxy) stop() {
	p.server.Close()
}

func (p *egressProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	host := r.URL.Hostname()
	if host == "" {
		host, _, _ = net.SplitHostPort(r.Host)
	}

	if !p.hostAllowed(host) {
		log.Printf("egress proxy: blocked %s %s", r.Method, r.Host)
		http.Error(w, "Forbidden: destination not whitelisted", http.StatusForbidden)
		return
	}

	if r.Method == http.MethodConnect {
		p.handleTunnel(w, r)
	} else {
		p.handleHTTP(w, r)
	}
}

func (p *egressProxy) hostAllowed(host string) bool {
	if net.ParseIP(host) != nil {
		_, ok := p.allowedIPs[host]
		return ok
	}
	ips, err := net.LookupHost(host)
	if err != nil {
		return false
	}
	for _, ip := range ips {
		if _, ok := p.allowedIPs[ip]; ok {
			return true
		}
	}
	return false
}

func (p *egressProxy) handleTunnel(w http.ResponseWriter, r *http.Request) {
	target, err := net.Dial("tcp", r.Host)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer target.Close()

	w.WriteHeader(http.StatusOK)

	hj, ok := w.(http.Hijacker)
	if !ok {
		return
	}
	conn, _, err := hj.Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	go io.Copy(target, conn) //nolint:errcheck
	io.Copy(conn, target)    //nolint:errcheck
}

func (p *egressProxy) handleHTTP(w http.ResponseWriter, r *http.Request) {
	r.RequestURI = ""
	for _, h := range []string{"Proxy-Connection", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailers", "Transfer-Encoding", "Upgrade"} {
		r.Header.Del(h)
	}
	resp, err := http.DefaultTransport.RoundTrip(r)
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body) //nolint:errcheck
}
