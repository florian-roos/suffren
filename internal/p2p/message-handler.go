package p2p

import (
	"fmt"
	"log"

	"flatstate/internal/message"
)

type MessageConnection interface {
	Send(message.Message) error
	Receive() (message.Message, error)
	Close() error
}

// Receive a message, print it and send an ack
func HandleIncomingMessages(conn MessageConnection, port string) {
	defer conn.Close()

	for {
		msg, err := conn.Receive()
		if err != nil {
			log.Printf("[ERROR] Failed to receive message: %v\n", err)
			return
		}

		fmt.Printf("RECEIVED: %s from: %s\n", msg.Payload, msg.Sender)

		err = conn.Send(message.NewMessage(port, "ACK"))
		if err != nil {
			log.Printf("[ERROR] Failed to send Ack")
			return
		}
	}
}

// Send a message.Message and wait for its ack before terminating
func SendMessage(conn MessageConnection, msg message.Message) error {
	err := conn.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send message: %v\n", err)
		return err
	}

	ackReceived, err := conn.Receive()

	if err != nil {
		log.Printf("[ERROR] Failed to receive ACK: %v\n", err)
		return err
	}

	if ackReceived.Payload != "ACK" || ackReceived.Id == msg.Id {
		log.Printf("[ERROR] Invalid ACK received: %v, %v\n", ackReceived, msg)
		return fmt.Errorf("invalid ACK")
	}

	log.Printf("ACK received for message ID: %s\n", msg.Id)
	return nil
}
