package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
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

	unknownWebsocketMessage struct {
		Kind string
		Data json.RawMessage
	}

	RegisterHostMessage struct {
		Host string
	}
)

type pendingConnection struct {
	conn       net.Conn
	requestBuf *bytes.Buffer
}

var (
	// token -> pending connection
	requests = map[string]*pendingConnection{}
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
		DataHost     string
		PublicHost   string
	}

	flag.StringVar(&config.InternalHost, "internal-host", "0.0.0.0:8081", "host for local clients to connect to (control)")
	flag.StringVar(&config.DataHost, "data-host", "0.0.0.0:8082", "host for local clients to connect to (data)")
	flag.StringVar(&config.PublicHost, "public-host", "0.0.0.0:8080", "host for remote clients to connect to")

	flag.Parse()

	eg := errgroup.Group{}
	eg.Go(func() error {
		return listenForLocalClients(config.InternalHost)
	})
	eg.Go(func() error {
		return listenForDataConnections(config.DataHost)
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
		log.Println("acme challenge for", host)
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
		log.Println("connected")
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
			log.Println("disconnected:", connectionID, "host:", host)
		}
	})

	m.HandleMessage(func(s *melody.Session, msg []byte) {
		fmt.Println("message", string(msg))

		var unknownMsg unknownWebsocketMessage
		err := json.Unmarshal(msg, &unknownMsg)
		if err != nil {
			s.Write([]byte("Invalid message"))
			err = s.Close()
			if err != nil {
				log.Println("error closing session:", err)
			}
			return
		}

		switch unknownMsg.Kind {
		case "registerHost":
			var registerHostMsg RegisterHostMessage
			err := json.Unmarshal(unknownMsg.Data, &registerHostMsg)
			if err != nil {
				s.Write([]byte("Invalid message"))
				err = s.Close()
				if err != nil {
					log.Println("error closing session:", err)
				}
				return
			}

			connectionID := fmt.Sprintf("%p", s)
			mapMu.Lock()
			routes.Add(registerHostMsg.Host, routeConfig{
				Host:         registerHostMsg.Host,
				ConnectionID: connectionID,
				Session:      s,
			})
			connectionHost[connectionID] = registerHostMsg.Host
			mapMu.Unlock()

			log.Println("registered host:", registerHostMsg.Host, "for connection:", connectionID)
		}

	})

	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	defer ln.Close()

	return httpServer.Serve(ln)
}

func listenForDataConnections(host string) error {
	ln, err := net.Listen("tcp", host)
	if err != nil {
		return err
	}
	defer ln.Close()

	log.Println("Data connection listener started on", host)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("Failed to accept data connection:", err)
			continue
		}

		go handleDataConnection(conn)
	}
}

func handleDataConnection(localConn net.Conn) {
	defer localConn.Close()

	// Read token (first line, newline-terminated)
	reader := bufio.NewReader(localConn)
	token, err := reader.ReadString('\n')
	if err != nil {
		log.Println("Failed to read token:", err)
		return
	}
	token = strings.TrimSpace(token)

	// Look up the pending remote client connection
	mapMu.Lock()
	pending, ok := requests[token]
	if ok {
		delete(requests, token)
	}
	mapMu.Unlock()

	if !ok {
		log.Println("Invalid token:", token)
		return
	}
	defer pending.conn.Close()

	log.Println("Splicing connections for token:", token)

	// Splice the connections together
	// Prepend the HTTP request buffer to the remote connection reads
	remoteReader := io.MultiReader(pending.requestBuf, pending.conn)

	eg := errgroup.Group{}
	eg.Go(func() error {
		_, err := io.Copy(localConn, remoteReader)
		return err
	})
	eg.Go(func() error {
		// Use reader to preserve any buffered data from token read
		_, err := io.Copy(pending.conn, reader)
		return err
	})

	if err := eg.Wait(); err != nil {
		log.Println("Connection splice error for token:", token, "error:", err)
		return
	}
	log.Println("Connection splice completed for token:", token)
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
		// Write data request to the session with a token
		dataMsg, err := json.Marshal(OpenDataConnectionMessage{
			Token: token,
		})
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		msg, err := json.Marshal(unknownWebsocketMessage{
			Kind: "openDataConnection",
			Data: dataMsg,
		})
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		err = routeConfig.Session.Write(msg)
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

		// Serialize the HTTP request to a buffer (since it was already consumed)
		requestBuf := &bytes.Buffer{}
		err = r.Write(requestBuf)
		if err != nil {
			log.Println("Failed to serialize HTTP request:", err)
			conn.Close()
			return
		}

		// Store the connection and request buffer in the map
		mapMu.Lock()
		requests[token] = &pendingConnection{
			conn:       conn,
			requestBuf: requestBuf,
		}
		mapMu.Unlock()
		// TODO: have some cleanup so we don't leak connections if the local client dies
	}))
}
