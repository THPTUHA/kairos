package pubsub

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"sync/atomic"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/server/deliverer"
	"github.com/THPTUHA/kairos/server/httpserver/config"
	"github.com/gorilla/mux"
)

var userConnectNum int64

func Start(config *config.Configs, pubsub chan *PubSubPayload) {

	userConnectNum = helper.GetTimeNow()
	node, err := createPubSub(pubsub)
	if err != nil {
		log.Fatalln(err)
	}

	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/server/pubsub", authMiddleware(deliverer.NewWebsocketHandler(node, deliverer.WebsocketConfig{})))

	if err := http.ListenAndServe(fmt.Sprintf(":%d", config.PubSub.Port), router); err != nil {
		log.Fatalln(err)
	}
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clientName := r.URL.Query().Get("name")
		fmt.Println("clientName :" + clientName)
		userConnectID := atomic.AddInt64(&userConnectNum, 1)

		ctx := r.Context()
		ctx = deliverer.SetCredentials(ctx, &deliverer.Credentials{
			UserID: fmt.Sprintf("%s-%d@%s", config.KairosDeamon, userConnectID, clientName),
		})
		r = r.WithContext(ctx)
		h.ServeHTTP(w, r)
	})
}

func handleLog(e deliverer.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

func createPubSub(pubsub chan *PubSubPayload) (*deliverer.Node, error) {
	node, err := deliverer.NewNode(deliverer.Config{
		LogLevel:   deliverer.LogLevelDebug,
		LogHandler: handleLog,
	})

	if err != nil {
		return nil, err
	}

	node.OnConnect(func(client *deliverer.Client) {
		log.Printf("client %s connected via %s", client.UserID(), client.Transport().Name())

		err := client.Send([]byte(`{"user_count_id": "` + client.UserID() + `"}`))
		if err != nil {
			if err == io.EOF {
				log.Printf("error sending message: %s\n", err)
				return
			}
			log.Printf("error sending message: %s\n", err)
		}

		log.Println("send message to client")
		client.OnSubscribe(func(e deliverer.SubscribeEvent, cb deliverer.SubscribeCallback) {
			log.Printf("client %s subscribes on channel %s", client.UserID(), e.Channel)
			cb(deliverer.SubscribeReply{
				Options: deliverer.SubscribeOptions{
					Data: []byte(`{"user_count_id": "` + client.UserID() + `"}`),
				},
			}, nil)
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

	go func(pubsub chan *PubSubPayload) {
		for {
			payload := <-pubsub
			log.Printf("client %s pubsub paload = %s\n", payload.UserCountID, payload.Data)
			if payload.Cmd == AuthCmd {
				_, err := node.Publish(
					payload.UserCountID,
					[]byte(`{
						"access_token": "`+payload.Data+`",
						"user_count_id": "`+payload.UserCountID+`",
						"user_id": "`+fmt.Sprint(payload.UserID)+`",
						"client_id": "`+fmt.Sprint(payload.ClientID)+`"
					}`),
				)

				if err != nil {
					log.Printf("error publishing to channel: %s", err)
				}

				SuccessEvent.Payload = fmt.Sprintf("%s@%d", payload.UserCountID, payload.UserID)
				payload.Fn(SuccessEvent)
			}

		}
	}(pubsub)

	return node, nil
}
