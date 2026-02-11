package main

import (
	"flag"
	"time"

	"flatstate/internal/p2p"
)

func main() {
	port := flag.String("port", "8080", "Port d'écoute local")
	target := flag.String("target", "", "Adresse cible (ex: 127.0.0.1:8081)")
	delay := flag.Int("delay", 1000, "Délai entre les messages (ms)")
	sentence := flag.String("sentence", "Hello World", "La phrase à envoyer")

	flag.Parse()

	// Initialisation du noeud
	node := &p2p.Node{
		Port:       *port,
		Sentence:   *sentence,
		Delay:      time.Duration(*delay),
		TargetAddr: *target,
	}

	// Lancement
	// On passe la target pour initier la connexion sortante
	node.Start()
}
