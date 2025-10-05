package client

import "flag"

func StartClient() {
	var config struct {
		Host string
		Port int
	}

	flag.StringVar(&config.Host, "service-host", "localhost", "host to connect to for the local service")
	flag.IntVar(&config.Port, "service-port", 8080, "port to connect to for the local service")

	flag.StringVar(&config.Host, "tunnel-host", "localhost", "host to connect to for the tunnel")
	flag.IntVar(&config.Port, "tunnel-port", 8080, "port to connect to the tunnel")

	flag.Parse()

	// TODO: Create control plane connection to tunnel server
	// TODO: On control plane signal, create a data plane connection to the tunnel server
	// TODO: splice the data plane UPGRADE request and a TCP connection to the local service together
}
