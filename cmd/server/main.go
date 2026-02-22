package main

import (
	"flag"
	"fmt"
	"suffren/internal/node"
	"suffren/internal/p2p"
)

func main() {
	port := flag.String("port", "8080", "Local listening port")
	flag.Parse()

	network := p2p.NewNetwork(*port)
	n := node.NewNode(*port, network, node.NewLAMessageHandler())
	n.Start()

	fmt.Println("Node started on port", *port)

	n.Stop()
}
