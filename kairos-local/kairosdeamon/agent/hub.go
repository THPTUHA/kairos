package agent

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/config"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/events"
	"github.com/THPTUHA/kairos/kairos-local/kairosdeamon/pubsub"
	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/sirupsen/logrus"
)

type hubError struct {
	msg string
	err error
}

type Hub struct {
	ErrorCh chan *hubError

	Client     *pubsub.Client
	clientName string
	userID     int64
	taskCh     chan *workflow.CmdTask
	channel    string
	eventCh    chan *events.Event
	config     *config.Configs
	logger     *logrus.Entry
}

func NewHub(eventCh chan *events.Event, config *config.Configs) *Hub {
	return &Hub{
		config:  config,
		eventCh: eventCh,
		logger:  logger.InitLogger(config.LogLevel, "hub"),
	}
}

func (hub *Hub) AddEventTask(ch chan *workflow.CmdTask) {
	hub.taskCh = ch
}

func (hub *Hub) Publish(cmd *workflow.CmdReplyTask) {
	cmd.RunOn = hub.RunOn()
	cmd.SendAt = helper.GetTimeNow()
	data, err := json.Marshal(cmd)
	fmt.Printf("[HUB PUBLISH REPLY] %+v\n", cmd)
	if err != nil {
		hub.logger.WithField("hub", "publish").Error(err)
	}
	hub.Client.Publish(hub.channel, data)
}

func (hub *Hub) RunOn() string {
	return fmt.Sprintf("%s%s", hub.clientName, workflow.SubClient)
}

func (hub *Hub) PublishLog(log *workflow.LogDaemon) {
	log.RunOn = hub.RunOn()
	log.SendAt = helper.GetTimeNow()
	data, err := json.Marshal(log)
	fmt.Printf("[HUB PUBLISH LOG REPLY] %+v\n", log)
	if err != nil {
		hub.logger.WithField("hub", "publishlog").Error(err)
	}
	hub.Client.Publish(hub.channel, data)
}

func (hub *Hub) HandleConnectServer(auth *config.Auth) error {
	hub.logger.Debug("start connect server", auth.UserID)
	hub.clientName = auth.ClientName
	hub.userID, _ = strconv.ParseInt(auth.UserID, 10, 64)
	client := pubsub.NewClient(hub.config.PubSubEnpoint, pubsub.Config{
		Token: auth.Token,
	})
	hub.Client = client
	hub.channel = fmt.Sprintf("kairosdeamon-%s", auth.ClientID)
	err := client.Connect()
	if err != nil {
		return err
	}

	client.OnConnected(func(e pubsub.ConnectedEvent) {

	})

	client.OnPublication(func(e pubsub.ServerPublicationEvent) {
		var cmd workflow.CmdTask
		json.Unmarshal(e.Data, &cmd)
		if err != nil {
			hub.logger.WithField("task", "publication receiver task").Error(err)
			return
		}
		go func() {
			fmt.Printf("ON PUBLICATION CMD = %+v \n", cmd)
			hub.taskCh <- &cmd
		}()
	})

	return nil
}

func (hub *Hub) IsConnect() bool {
	return hub.Client != nil && hub.Client.State() == pubsub.StateConnected
}

func (hub *Hub) Disconnect() error {
	return hub.Client.Disconnect()
}

func (hub *Hub) handleError() {
	for err := range hub.ErrorCh {
		hub.logger.WithField("message", err.msg).WithField("error", err.err)
	}
}
