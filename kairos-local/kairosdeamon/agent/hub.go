package agent

import (
	"encoding/json"
	"fmt"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/pubsub"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/sirupsen/logrus"
)

type hubError struct {
	msg string
	err error
}

type Hub struct {
	ErrorCh chan *hubError
	EventCh chan *events.Event
	Client  *pubsub.Client
	taskCh  chan *TaskEvent

	logger *logrus.Entry
}

func NewHub(eventCh chan *events.Event) *Hub {
	// init get config here
	return &Hub{
		EventCh: eventCh,
		logger:  logger.InitLogger(config.LogLevel, "hub"),
	}
}

func (hub *Hub) Run() {
	for {
		select {
		case e := <-hub.EventCh:
			switch e.Cmd {
			case events.ConnectServerCmd:
				err := hub.handleConnectServer()
				if err != nil {
					hub.ErrorCh <- &hubError{
						msg: "connect server error",
					}
				}
			case events.SubscribeServerCmd:
				err := hub.handleSubscribeServer(e.Payload)
				if err != nil {
					hub.ErrorCh <- &hubError{
						msg: "subscribe server error",
					}
				}
			}
		}
	}
}

func (hub *Hub) AddEventTask(ch chan *TaskEvent) {
	hub.taskCh = ch
}

func (hub *Hub) handleSubscribeServer(channel string) error {
	hub.logger.Debug("channel: " + channel)
	sub, err := hub.Client.NewSubscription(channel, pubsub.SubscriptionConfig{
		Recoverable: true,
		JoinLeave:   true,
	})

	if err != nil {
		hub.logger.WithField("sub", "create sub").Error(err)
		return err
	}

	sub.OnSubscribed(func(e pubsub.SubscribedEvent) {
		hub.logger.WithField("hub", "sub").Debug("onsubscribed")
		var te TaskEvent
		err := json.Unmarshal(e.Data, &te)
		if err != nil {
			hub.ErrorCh <- &hubError{
				msg: "ubmarshal onSubscribed",
				err: err,
			}
			return
		}
		return
		// nguy hiểm
		// hub.taskCh <- &te
	})

	sub.OnPublication(func(e pubsub.PublicationEvent) {
		var task interface{}
		json.Unmarshal(e.Data, &task)
		fmt.Printf(" publication reciver task = %+v", task)
		// TODO xử lý recieve task
	})

	err = sub.Subscribe()
	if err != nil {
		hub.logger.WithField("sub", "cannot sub").Error(err)
		return err
	}
	return nil
}

func (hub *Hub) handleConnectServer() error {
	hub.logger.Debug("start connect server")
	client := pubsub.NewClient(config.PubSubEnpoint, pubsub.Config{
		Token: config.Token,
	})
	hub.Client = client
	err := client.Connect()
	if err != nil {
		return err
	}

	client.OnMessage(func(e pubsub.MessageEvent) {
		var event events.Event
		err := json.Unmarshal(e.Data, &event)
		if err != nil {
			hub.ErrorCh <- &hubError{
				msg: "ubmarshal onMessage",
				err: err,
			}
			return
		}
		hub.logger.WithFields(logrus.Fields{
			"cmd":  event.Cmd,
			"data": event.Payload,
		}).Debug("recivier message from server")
		hub.EventCh <- &event

	})

	client.OnConnected(func(e pubsub.ConnectedEvent) {
		channel := e.ClientID
		hub.logger.WithField("client", "onconnected").Debug(channel)

	})

	return nil
}

func (hub *Hub) handleError() {
	for err := range hub.ErrorCh {
		hub.logger.WithField("message", err.msg).WithField("error", err.err)
	}
}
