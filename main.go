package main

import (
	"fmt"
	"os"

	"github.com/danthegoodman1/tunnels-thing/client"
	"github.com/danthegoodman1/tunnels-thing/server"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("must use either 'server' or 'client'")
		os.Exit(1)
	}
	role := os.Args[1]

	// Remove the role argument so flags parse correctly
	os.Args = append([]string{os.Args[0]}, os.Args[2:]...)

	switch role {
	case "server":
		server.StartServer()
	case "client":
		client.StartClient()
	default:
		fmt.Println("must use either 'server' or 'client'")
		os.Exit(1)
	}
}
