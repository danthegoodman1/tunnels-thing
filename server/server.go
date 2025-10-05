package server

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"

	"github.com/olahol/melody"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
)

func StartServer() {
	var config struct {
		InternalHost string
		PublicHost   string
	}

	flag.StringVar(&config.InternalHost, "internal-host", "0.0.0.0:8081", "host for local clients to connect to")
	flag.StringVar(&config.PublicHost, "public-host", "0.0.0.0:8080", "host for remote clients to connect to")

	flag.Parse()

	// TODO: TCP listening for both local service clients and remote clients
	eg := errgroup.Group{}
	eg.Go(func() error {
		return listenForLocalClients(config.InternalHost)
	})
	eg.Go(func() error {
		return listenForRemoteClients(config.PublicHost)
	})
	err := eg.Wait()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
}

func listenForLocalClients(host string) error {
	mux := http.NewServeMux()
	httpServer := &http.Server{
		Addr:    host,
		Handler: h2c.NewHandler(mux, &http2.Server{}),
	}
	m := melody.New()

	mux.HandleFunc("GET /control", func(w http.ResponseWriter, r *http.Request) {
		m.HandleRequest(w, r)
	})

	mux.HandleFunc("/.well-known/acme-challenge", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		fmt.Println("acme challenge for", host)
		// TODO: create autocert handler for domain, shared DB for certs
		// https://chatgpt.com/c/68d00969-fddc-8333-965c-cd1bdcb63e9a
		w.WriteHeader(http.StatusNotImplemented)
	})

	m.HandleConnect(func(s *melody.Session) {
		// TODO: register connection for a given domain
		// get cert for that domain with HTTP-01 challenge
		fmt.Println("connected")
	})

	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	defer ln.Close()

	return httpServer.Serve(ln)
}

func listenForRemoteClients(host string) error {
	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	defer ln.Close()
	return nil
}
