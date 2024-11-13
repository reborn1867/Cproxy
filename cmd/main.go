package main

import (
	"bytes"
	"crypto/tls"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"

	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
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
	// if err := server.ListenAndServeTLS(filepath.Join(p.certDir, "tls.crt"), filepath.Join(p.certDir, "tls.key")); err != nil {
	// 	fmt.Printf("Error starting server: %v\n", err)
	// }

	if err := server.ListenAndServe(); err != nil {
		fmt.Printf("Error starting server: %v\n", err)
	}
}

// use httputil.ReverseProxy to proxy the request to the remote server
func (p *ReverseProxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// print the request URL

	reqDump, err := httputil.DumpRequestOut(r, true)
	if err != nil {
		fmt.Printf("err: %s\n", err)
	}

	fmt.Printf("Original Request URL: %v\n", r.URL.String())
	fmt.Printf("Original path: %s\n", r.URL.Path)
	fmt.Printf("Original raw path: %s\n", r.URL.RawFragment)
	fmt.Printf("Original header: %s\n", r.Header)
	fmt.Printf("Original details: %s\n", reqDump)

	config := ctrl.GetConfigOrDie()

	remoteAddr := fmt.Sprintf("%s:%d", p.remoteHost, p.remotePort)
	target := &url.URL{
		Scheme: "https",
		Host:   remoteAddr,
		Path:   "/",
	}

	transport, err := rest.TransportFor(config)
	if err != nil {
		log.Fatalf("Error creating transport: %v", err)
	}

	// rewrite := func(req *httputil.ProxyRequest) {
	// 	req.SetURL(target)

	// 	reqDump, err := httputil.DumpRequestOut(req.Out, true)
	// 	if err != nil {
	// 		log.Fatal(err)
	// 	}

	// }

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

		// print the rewritten request URL
		fmt.Printf("Rewritten request URL: %v\n", req.URL.String())
		fmt.Printf("Rewritten path: %s\n", req.URL.Path)
		fmt.Printf("Rewritten raw path: %s\n", req.URL.RawFragment)
		fmt.Printf("Rewritten header: %s\n", req.Header)
		fmt.Printf("Rewritten details: %s\n", reqDump)
	}

	proxy := &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
		ModifyResponse: func(response *http.Response) error {
			// print the response status code and message
			fmt.Printf("Response status code: %v\n", response.Status)
			h := response.Header.Clone()
			fmt.Printf("Response Header: %+v\n", h)
			// print response body
			body, err := io.ReadAll(response.Body)
			if err != nil {
				fmt.Printf("Error reading response body: %v\n", err)
			}

			respDump, err := httputil.DumpResponse(response, true)
			if err != nil {
				fmt.Printf("Error dump response body: %v\n", err)
			}

			// Restore the io.ReadCloser to its original state
			response.Body = io.NopCloser(bytes.NewBuffer(body))

			fmt.Printf("Response body: %s\n", respDump)

			return nil
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			// print the error
			fmt.Printf("Error: %v\n", err)
		},
	}

	proxy.ServeHTTP(w, r)
}

func dialTLS(network, addr string) (net.Conn, error) {
	conn, err := net.Dial(network, addr)
	if err != nil {
		return nil, err
	}

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{ServerName: host}

	tlsConn := tls.Client(conn, cfg)
	if err := tlsConn.Handshake(); err != nil {
		conn.Close()
		return nil, err
	}

	cs := tlsConn.ConnectionState()
	cert := cs.PeerCertificates[0]

	// Verify here
	cert.VerifyHostname(host)
	log.Println(cert.Subject)

	return tlsConn, nil
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
