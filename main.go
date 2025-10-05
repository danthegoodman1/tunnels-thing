package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Println("must use either 'server' or 'client'")
		os.Exit(1)
	}
	role := os.Args[1]
	switch role {
	case "server":
		startServer()
	case "client":
		startClient()
	default:
		fmt.Println("must use either 'server' or 'client'")
		os.Exit(1)
	}
}
