package main

import (
	"bufio"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	_ "net/http/pprof"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/pubsub"
)

const exampleTokenHmacSecret = "secret"

type ChatMessage struct {
	Input string `json:"input"`
}

func main() {
	go func() {
		log.Println(http.ListenAndServe(":6060", nil))
	}()

	client := pubsub.NewClient(
		"ws://localhost:8000/connection/websocket",
		pubsub.Config{
			// Sending token makes it work with Centrifugo JWT auth (with `secret` HMAC key).
		},
	)
	defer client.Close()

	client.OnConnecting(func(e pubsub.ConnectingEvent) {
		log.Printf("Connecting - %d (%s)", e.Code, e.Reason)
	})
	client.OnConnected(func(e pubsub.ConnectedEvent) {
		log.Printf("Connected with ID %s", e.ClientID)
	})
	client.OnDisconnected(func(e pubsub.DisconnectedEvent) {
		log.Printf("Disconnected: %d (%s)", e.Code, e.Reason)
	})

	client.OnError(func(e pubsub.ErrorEvent) {
		log.Printf("Error: %s", e.Error.Error())
	})

	client.OnMessage(func(e pubsub.MessageEvent) {
		log.Printf("Message from server: %s", string(e.Data))
	})

	client.OnSubscribed(func(e pubsub.ServerSubscribedEvent) {
		log.Printf("Subscribed to server-side channel %s", e.Channel)
	})
	client.OnSubscribing(func(e pubsub.ServerSubscribingEvent) {
		log.Printf("Subscribing to server-side channel %s", e.Channel)
	})
	client.OnUnsubscribed(func(e pubsub.ServerUnsubscribedEvent) {
		log.Printf("Unsubscribed from server-side channel %s", e.Channel)
	})

	client.OnPublication(func(e pubsub.ServerPublicationEvent) {
		log.Printf("Publication from server-side channel %s: %s", e.Channel, e.Data)
	})

	err := client.Connect()
	if err != nil {
		log.Fatalln(err)
	}

	sub, err := client.NewSubscription("chat:index", pubsub.SubscriptionConfig{})
	if err != nil {
		log.Fatalln(err)
	}

	sub.OnSubscribing(func(e pubsub.SubscribingEvent) {
		log.Printf("Subscribing on channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
	})
	sub.OnSubscribed(func(e pubsub.SubscribedEvent) {
		log.Printf("Subscribed on channel %s", sub.Channel)
	})
	sub.OnUnsubscribed(func(e pubsub.UnsubscribedEvent) {
		log.Printf("Unsubscribed from channel %s - %d (%s)", sub.Channel, e.Code, e.Reason)
	})

	sub.OnError(func(e pubsub.SubscriptionErrorEvent) {
		log.Printf("Subscription error %s: %s", sub.Channel, e.Error)
	})

	sub.OnPublication(func(e pubsub.PublicationEvent) {
		var chatMessage *ChatMessage
		err := json.Unmarshal(e.Data, &chatMessage)
		if err != nil {
			return
		}
		log.Printf("Someone says via channel %s: %s", sub.Channel, chatMessage.Input)
	})

	err = sub.Subscribe()
	if err != nil {
		log.Fatalln(err)
	}

	pubText := func(text string) error {
		msg := &ChatMessage{
			Input: text,
		}
		data, _ := json.Marshal(msg)
		_, err := sub.Publish(context.Background(), data)
		return err
	}

	err = pubText("hello")
	if err != nil {
		log.Printf("Error publish: %s", err)
	}

	log.Printf("Print something and press ENTER to send\n")

	// Read input from stdin.
	go func(sub *pubsub.Subscription) {
		reader := bufio.NewReader(os.Stdin)
		for {
			text, _ := reader.ReadString('\n')
			text = strings.TrimSpace(text)
			switch text {
			case "#subscribe":
				err := sub.Subscribe()
				if err != nil {
					log.Println(err)
				}
			case "#unsubscribe":
				err := sub.Unsubscribe()
				if err != nil {
					log.Println(err)
				}
			case "#disconnect":
				err := client.Disconnect()
				if err != nil {
					log.Println(err)
				}
			case "#connect":
				err := client.Connect()
				if err != nil {
					log.Println(err)
				}
			case "#close":
				client.Close()
			default:
				err = pubText(text)
				if err != nil {
					log.Printf("Error publish: %s", err)
				}
			}
		}
	}(sub)

	// Run until CTRL+C.
	select {}
}
