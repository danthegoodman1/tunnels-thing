package client

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/url"
	"strconv"

	"github.com/gorilla/websocket"
	"golang.org/x/sync/errgroup"
)

type (
	unknownWebsocketMessage struct {
		Kind string
		Data json.RawMessage
	}

	OpenDataConnectionMessage struct {
		Token string
	}

	RegisterHostMessage struct {
		Host string
	}

	config struct {
		ServiceHost    string
		ServicePort    int
		TunnelHost     string
		TunnelDataPort int
		RegisterHost   string
	}
)

func StartClient() {
	var cfg config

	flag.StringVar(&cfg.ServiceHost, "service-host", "localhost", "host to connect to for the local service")
	flag.IntVar(&cfg.ServicePort, "service-port", 8080, "port to connect to for the local service")

	flag.StringVar(&cfg.TunnelHost, "tunnel-host", "localhost", "host to connect to for the tunnel")
	flag.IntVar(&cfg.TunnelDataPort, "tunnel-data-port", 8082, "port to connect to the tunnel for data connections")

	flag.StringVar(&cfg.RegisterHost, "register-host", "", "host to register with the tunnel server")

	flag.Parse()

	if cfg.RegisterHost == "" {
		log.Fatal("--register-host is required")
	}

	// Create control plane connection to tunnel server (control is on port 8081)
	wsURL := url.URL{
		Scheme: "ws",
		Host:   net.JoinHostPort(cfg.TunnelHost, "8081"),
		Path:   "/control",
	}

	log.Println("Connecting to tunnel server at", wsURL.String())
	conn, _, err := websocket.DefaultDialer.Dial(wsURL.String(), nil)
	if err != nil {
		log.Fatal("Failed to connect to tunnel server:", err)
	}
	defer conn.Close()

	// Register the host
	registerMsg, err := json.Marshal(RegisterHostMessage{
		Host: cfg.RegisterHost,
	})
	if err != nil {
		log.Fatal("Failed to marshal register message:", err)
	}

	msg, err := json.Marshal(unknownWebsocketMessage{
		Kind: "registerHost",
		Data: registerMsg,
	})
	if err != nil {
		log.Fatal("Failed to marshal websocket message:", err)
	}

	err = conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		log.Fatal("Failed to send register message:", err)
	}

	log.Println("Registered host:", cfg.RegisterHost)

	// Handle incoming messages
	for {
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Println("Connection closed:", err)
			return
		}

		var unknownMsg unknownWebsocketMessage
		err = json.Unmarshal(message, &unknownMsg)
		if err != nil {
			log.Println("Failed to unmarshal message:", err)
			continue
		}

		switch unknownMsg.Kind {
		case "openDataConnection":
			var openDataMsg OpenDataConnectionMessage
			err = json.Unmarshal(unknownMsg.Data, &openDataMsg)
			if err != nil {
				log.Println("Failed to unmarshal openDataConnection message:", err)
				continue
			}

			log.Println("Opening data connection for token:", openDataMsg.Token)

			// Handle data connection in a goroutine
			go handleDataConnection(cfg, openDataMsg.Token)
		default:
			log.Println("Unknown message kind:", unknownMsg.Kind)
		}
	}
}

func handleDataConnection(cfg config, token string) {
	// Dial the tunnel server data port
	tunnelAddr := net.JoinHostPort(cfg.TunnelHost, strconv.Itoa(cfg.TunnelDataPort))
	tunnelConn, err := net.Dial("tcp", tunnelAddr)
	if err != nil {
		log.Println("Failed to connect to tunnel data endpoint:", err)
		return
	}
	defer tunnelConn.Close()

	// Send token as first line
	_, err = fmt.Fprintf(tunnelConn, "%s\n", token)
	if err != nil {
		log.Println("Failed to send token to tunnel:", err)
		return
	}

	// Connect to local service
	serviceAddr := net.JoinHostPort(cfg.ServiceHost, strconv.Itoa(cfg.ServicePort))
	serviceConn, err := net.Dial("tcp", serviceAddr)
	if err != nil {
		log.Println("Failed to connect to local service:", err)
		return
	}
	defer serviceConn.Close()

	log.Println("Splicing connections for token:", token)

	// Splice the connections together
	eg := errgroup.Group{}
	eg.Go(func() error {
		_, err := io.Copy(serviceConn, tunnelConn)
		return err
	})
	eg.Go(func() error {
		_, err := io.Copy(tunnelConn, serviceConn)
		return err
	})

	if err := eg.Wait(); err != nil {
		log.Println("Connection splice error for token:", token, "error:", err)
		return
	}
	log.Println("Connection splice completed for token:", token)
}
