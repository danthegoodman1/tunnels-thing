package main

import "flag"

func startServer() {
	var config struct {
		Host string
		Port int
	}

	flag.StringVar(&config.Host, "host", "0.0.0.0", "host to connect to")
	flag.IntVar(&config.Port, "port", 8080, "port to connect to")

	flag.Parse()

	// TODO: TCP listening for both local service clients and remote clients
}
