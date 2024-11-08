package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"path/filepath"
	"strings"
)

func main() {
	var host string
	var port int
	var remoteHost string
	var remotePort int
	var certDir string

	// Define the command line arguments
	flag.StringVar(&host, "host", "localhost", "The host to listen on")
	flag.IntVar(&port, "port", 8888, "The port to listen on")
	flag.StringVar(&remoteHost, "remote-host", "localhost", "The remote host to proxy to")
	flag.IntVar(&remotePort, "remote-port", 8081, "The remote port to proxy to")
	flag.StringVar(&certDir, "cert-dir", "/tmp/serving-certs", "The directory containing the certificate files")

	// Parse the command line arguments
	flag.Parse()

	// Start the proxy server
	StartProxy(certDir, host, port, remoteHost, remotePort)
}

func StartProxy(certDir, host string, port int, remoteHost string, remotePort int) {
	// Start the proxy server
	proxy := NewReverseProxy(certDir, host, remoteHost, port, remotePort)
	proxy.Start()
}

type ReverseProxy struct {
	host       string
	port       int
	remoteHost string
	remotePort int
	certDir    string
}

func NewReverseProxy(certDir, host, remoteHost string, port, remotePort int) *ReverseProxy {
	return &ReverseProxy{
		host:       host,
		port:       port,
		remoteHost: remoteHost,
		remotePort: remotePort,
		certDir:    certDir,
	}
}

func (p *ReverseProxy) Start() {
	// Start the proxy server

	addr := fmt.Sprintf("%s:%d", p.host, p.port)
	fmt.Printf("Starting proxy server on %s\n", addr)
	server := &http.Server{
		Addr:    addr,
		Handler: p}
	if err := server.ListenAndServeTLS(filepath.Join(p.certDir, "tls.crt"), filepath.Join(p.certDir, "tls.key")); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

// use httputil.ReverseProxy to proxy the request to the remote server
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	remoteAddr := fmt.Sprintf("%s:%d", p.remoteHost, p.remotePort)
	target := &url.URL{
		Scheme: "https",
		Host:   remoteAddr,
	}
	director := func(req *http.Request) {
		targetQuery := target.RawQuery
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
	}

	proxy := &httputil.ReverseProxy{Director: director}
	proxy.ServeHTTP(w, r)
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	if a.RawPath == "" && b.RawPath == "" {
		return singleJoiningSlash(a.Path, b.Path), ""
	}
	// Same as singleJoiningSlash, but uses EscapedPath to determine
	// whether a slash should be added
	apath := a.EscapedPath()
	bpath := b.EscapedPath()

	aslash := strings.HasSuffix(apath, "/")
	bslash := strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	}
	return a.Path + b.Path, apath + bpath
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}