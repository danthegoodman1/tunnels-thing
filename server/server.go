package server

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"

	"github.com/danthegoodman1/tunnels-thing/utils"
	"github.com/google/uuid"
	"github.com/olahol/melody"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"golang.org/x/sync/errgroup"
)

type (
	routeConfig struct {
		Host         string
		ConnectionID string
		Session      *melody.Session
	}

	OpenDataConnectionMessage struct {
		Token string
	}
)

var (
	// token -> connection
	requests = map[string]net.Conn{}
	// pointer (connectionID) -> session
	sessions = map[string]*melody.Session{}
	// connectionID -> host (reverse lookup for cleanup)
	connectionHost = map[string]string{}
	mapMu          = &sync.Mutex{}

	// host -> route configs
	routes = utils.NewOneToMany[string, routeConfig]()
)

func StartServer() {
	var config struct {
		InternalHost string
		PublicHost   string
	}

	flag.StringVar(&config.InternalHost, "internal-host", "0.0.0.0:8081", "host for local clients to connect to")
	flag.StringVar(&config.PublicHost, "public-host", "0.0.0.0:8080", "host for remote clients to connect to")

	flag.Parse()

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
	mux.HandleFunc("GET /data", func(w http.ResponseWriter, r *http.Request) {
		// TODO: splice connection from remote client to local service
	})

	mux.HandleFunc("/.well-known/acme-challenge", func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		fmt.Println("acme challenge for", host)
		// TODO: create autocert handler for domain, shared DB for certs
		// https://chatgpt.com/c/68d00969-fddc-8333-965c-cd1bdcb63e9a
		w.WriteHeader(http.StatusNotImplemented)
	})

	m.HandleConnect(func(s *melody.Session) {
		// store the connection ID
		connectionID := fmt.Sprintf("%p", s) // the pointer address is unique since it's a memory address
		mapMu.Lock()
		sessions[connectionID] = s
		mapMu.Unlock()

		// get cert for that domain with HTTP-01 challenge
		fmt.Println("connected")
	})

	m.HandleDisconnect(func(s *melody.Session) {
		// Get the connection ID and remove from sessions
		connectionID := fmt.Sprintf("%p", s)

		// Single mutex hold for all operations
		mapMu.Lock()
		delete(sessions, connectionID)
		host, hasHost := connectionHost[connectionID]
		if hasHost {
			delete(connectionHost, connectionID)
			routes.Remove(host, routeConfig{
				Host:         host,
				ConnectionID: connectionID,
				Session:      s,
			})
		}
		mapMu.Unlock()

		if hasHost {
			fmt.Println("disconnected:", connectionID, "host:", host)
		}
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		// TODO: handle message
		fmt.Println("message", string(msg))
	})

	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	defer ln.Close()

	return httpServer.Serve(ln)
}

func listenForRemoteClients(host string) error {
	// TODO: make a TCP listener that can handle SNI or HTTP(s) like in Gildra
	return http.ListenAndServe(host, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Find the route config for the host
		mapMu.Lock()
		routeConfig, ok := routes.GetRandom(r.Host)
		mapMu.Unlock()
		if !ok {
			http.Error(w, "Not Found", http.StatusNotFound)
			return
		}

		token := uuid.New().String()
		// TODO: write data request to the session with a token
		b, err := json.Marshal(OpenDataConnectionMessage{
			Token: token,
		})
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		err = routeConfig.Session.Write(b)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// hijack the connection, we are now responsible for the response
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		conn, _, err := hj.Hijack()
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Store the request in the map to be handled later
		mapMu.Lock()
		requests[r.Host] = conn
		mapMu.Unlock()
		// TODO: have some cleanup so we don't leak connections if the local client dies
	}))
}
