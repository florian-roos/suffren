package main

import (
	"flag"
	"time"

	"flatstate/internal/p2p"
)

func main() {
	port := flag.String("port", "8080", "Local listening port")
	target := flag.String("target", "", "Target Addres (ex: 127.0.0.1:8081)")
	delay := flag.Int("delay", 1000, "Delay between messages (ms)")
	sentence := flag.String("sentence", "Hello World", "Sentence to send")

	flag.Parse()

	node := &p2p.Node{
		Port:       *port,
		Sentence:   *sentence,
		Delay:      time.Duration(*delay),
		TargetAddr: *target,
	}

	node.Start()
}
