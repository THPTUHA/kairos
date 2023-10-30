package pubsub

import (
	"fmt"
	"log"
	"net/http"

	"github.com/THPTUHA/kairos/server/deliverer"
	"github.com/gorilla/mux"
)

func Start() {
	node, err := createPubSub()
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/server/pubsub", authMiddleware(deliverer.NewWebsocketHandler(node, deliverer.WebsocketConfig{})))
	fmt.Println("Start pubsub")
	if err := http.ListenAndServe(":8002", router); err != nil {
		log.Fatalln(err)
	}
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	})
}

func createPubSub() (*deliverer.Node, error) {
	node, err := deliverer.New(deliverer.Config{})
	if err != nil {
		return nil, err
	}

	node.OnConnect(func(client *deliverer.Client) {
		log.Printf("client %s connected via %s", client.UserID(), client.Transport().Name())

		client.OnSubscribe(func(e deliverer.SubscribeEvent, cb deliverer.SubscribeCallback) {
			log.Printf("client %s subscribes on channel %s", client.UserID(), e.Channel)
			cb(deliverer.SubscribeReply{}, nil)
		})

		client.OnPublish(func(e deliverer.PublishEvent, cb deliverer.PublishCallback) {
			log.Printf("client %s publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))
			cb(deliverer.PublishReply{}, nil)
		})

		client.OnDisconnect(func(e deliverer.DisconnectEvent) {
			log.Printf("client %s disconnected", client.UserID())
		})
	})

	if err := node.Run(); err != nil {
		return nil, err
	}

	return node, nil
}
