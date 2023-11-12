package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/deliverer"
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type Message struct {
	Cmd     int
	Payload string
}

type DelivererServer struct {
	node *deliverer.Node
	sub  *natsSub

	logger *logrus.Entry
}

const (
	ConnectServerCmd = iota
	SubscribeServerCmd
)

func main() {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	server, err := createServer()
	if err != nil {
		log.Fatalln(err)
	}

	if err != nil {
		log.Fatalln(err)
	}

	go server.startSub(signals)
	server.Start(signals)
}

func (s *DelivererServer) Start(signals chan os.Signal) {
	auth.Init("kairosauthac", "kairosauthrf")
	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/pubsub", deliverer.NewWebsocketHandler(s.node, deliverer.WebsocketConfig{}))

	fmt.Printf("Deliver running on %d \n", Port)

	go func() {
		<-signals
		os.Exit(0)
	}()

	if err := http.ListenAndServe(fmt.Sprintf(":%d", Port), router); err != nil {
		log.Fatalln(err)
	}
}

func handleLog(e deliverer.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

func createServer() (*DelivererServer, error) {
	log := logger.InitLogger(logrus.DebugLevel.String(), "deliverer")
	node, err := createNode()
	if err != nil {
		return nil, err
	}

	sub := createSub(log)

	return &DelivererServer{
		node:   node,
		sub:    sub,
		logger: log,
	}, nil
}

func createSub(log *logrus.Entry) *natsSub {
	sub := NewNatsSub(&natsSubConfig{
		url:           nats.DefaultURL,
		name:          "deliverer",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
		logger:        log,
	})
	return sub
}

func (s *DelivererServer) startSub(signals chan os.Signal) {
	subList := make(map[string]nats.MsgHandler)
	subList["deliver"] = func(msg *nats.Msg) {
		var task workflow.Task
		err := json.Unmarshal(msg.Data, &task)
		if err != nil {
			s.logger.WithField("deliver", "reciver data").Error(err)
			return
		}
		data, err := json.Marshal(task)
		if err != nil {
			s.logger.WithField("deliver", "reciver data").Error(err)
			return
		}

		for _, c := range task.Clients {
			ch := task.Domains[c]
			s.logger.WithField("deliver", "send to client").Debug(ch)
			s.node.Publish(ch, data)
		}

		s.logger.WithField("deliver", "reciver data").Debug(fmt.Sprintf("%+v", task))
	}

	s.sub.Subscribes(subList, signals)
}

func createNode() (*deliverer.Node, error) {
	node, err := deliverer.New(deliverer.Config{
		LogLevel:   deliverer.LogLevelDebug,
		LogHandler: handleLog,
	})

	if err != nil {
		return nil, err
	}

	node.OnConnecting(func(ctx context.Context, e deliverer.ConnectEvent) (deliverer.ConnectReply, error) {
		user, err := auth.ExtractAccessStr(e.Token)
		if err != nil {
			return deliverer.ConnectReply{}, err
		}
		fmt.Println("----", user.KairosName)
		credentials := &deliverer.Credentials{
			UserID:   user.KairosName,
			ExpireAt: time.Now().Unix() + 1000,
		}

		return deliverer.ConnectReply{
			ClientSideRefresh: true,
			Credentials:       credentials,
		}, nil
	})

	node.OnConnect(func(client *deliverer.Client) {
		log.Printf("client %s connected via %s", client.UserID(), client.Transport().Name())
		msg, err := json.Marshal(&Message{
			Cmd:     SubscribeServerCmd,
			Payload: client.UserID(),
		})

		if err != nil {
			log.Printf("error connect: %s\n", err)
		}

		err = client.Send([]byte(msg))
		if err != nil {
			if err == io.EOF {
				log.Printf("error sending message: %s\n", err)
				return
			}
			log.Printf("error sending message: %s\n", err)
			return
		}

		client.OnSubscribe(func(e deliverer.SubscribeEvent, cb deliverer.SubscribeCallback) {
			log.Printf("client %s subscribes on channel %s", client.UserID(), e.Channel)
			cb(deliverer.SubscribeReply{
				Options: deliverer.SubscribeOptions{
					EnableRecovery: true,
					Data:           []byte(`{"user_count_id": "` + client.UserID() + `"}`),
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

	return node, nil
}
