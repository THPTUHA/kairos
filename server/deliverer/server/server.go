package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/deliverer"
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/go-redis/redis"
	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type Reply struct {
	ID   int64
	Data string
}

type Message struct {
	Cmd     int
	Payload string
}
type DelivererServer struct {
	node   *deliverer.Node
	sub    *natsSub
	redis  *redis.Client
	logger *logrus.Entry
}

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
	router.Handle("/pubsub", deliverer.NewWebsocketHandler(s.node, deliverer.WebsocketConfig{
		CheckOrigin: s.checkSameHost,
	}))

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
	var d DelivererServer
	err := d.createNode()
	if err != nil {
		return nil, err
	}
	d.redis = NewRedisDB(RedisHost, RedisPort, "")
	d.createSub(log)
	d.logger = log
	return &d, nil
}

func (d *DelivererServer) createSub(log *logrus.Entry) {
	sub := NewNatsSub(&natsSubConfig{
		url:           nats.DefaultURL,
		name:          "deliverer",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
		logger:        log,
	})
	d.sub = sub
}

func (s *DelivererServer) startSub(signals chan os.Signal) {
	subList := make(map[string]nats.MsgHandler)
	subList[messaging.DELIVERER_TASK] = func(msg *nats.Msg) {
		var cmd workflow.CmdTask
		err := json.Unmarshal(msg.Data, &cmd)
		if err != nil {
			s.logger.WithField("deliver", "receiver data").Error(err)
			return
		}
		s.redis.Set(fmt.Sprintf("%s-%d", cmd.Channel, cmd.DeliverID), string(msg.Data), 10*time.Minute)
		s.node.Publish(cmd.Channel, msg.Data)
		s.logger.WithField("deliver", "receiver deliver cmd").Debug(fmt.Sprintf("id=%d channel=%s", cmd.DeliverID, cmd.Channel))
	}

	subList[messaging.MONITOR_WORKFLOW] = func(msg *nats.Msg) {
		var cmd workflow.MonitorWorkflow
		err := json.Unmarshal(msg.Data, &cmd)
		if err != nil {
			s.logger.WithField("deliver", "receiver data").Error(err)
			return
		}
		s.node.Publish(fmt.Sprintf("kairosuser-%d", cmd.UserID), msg.Data)
		fmt.Printf("[ DELIVER MONITOR] %+v \n", cmd)
	}
	s.sub.Subscribes(subList, signals)
}

type ChannelRole struct {
	Name string
	Role int32
}

func (d *DelivererServer) createNode() error {
	node, err := deliverer.New(deliverer.Config{
		LogLevel:   deliverer.LogLevelDebug,
		LogHandler: handleLog,
	})

	if err != nil {
		return err
	}

	node.OnConnecting(func(ctx context.Context, e deliverer.ConnectEvent) (deliverer.ConnectReply, error) {
		user, err := auth.ExtractAccessStr(e.Token)
		if err != nil {
			return deliverer.ConnectReply{}, err
		}

		subs := make(map[string]deliverer.SubscribeOptions)
		channels := make([]*ChannelRole, 0)
		id := ""
		switch user.UserType {
		case models.KairosUser:
			id = fmt.Sprintf("kairosuser-%s", user.UserID)
			channels = append(channels, &ChannelRole{
				Name: id,
				Role: models.ReadWriteRole,
			})
		case models.ClientUser:
			id = fmt.Sprintf("kairosdeamon-%s", user.ClientID)
			channels = append(channels, &ChannelRole{
				Name: id,
				Role: models.ReadWriteRole,
			})
		case models.ChannelUser:
			// TODO add premission
			certID, _ := strconv.ParseInt(user.ClientID, 10, 64)
			req := proto.ChannelPermitRequest{
				CertID: certID,
			}
			data, err := json.Marshal(&req)
			if err != nil {
				fmt.Println("Errr 01", err)
				return deliverer.ConnectReply{}, err
			}

			reply, err := d.sub.Con.Request(fmt.Sprintf("%s.%d", messaging.INFOMATION, proto.Command_ChannelPermitRequest), data, 5*time.Second)
			if err != nil {
				fmt.Println("Errr 02", err)
				return deliverer.ConnectReply{}, err
			}
			var res proto.ChannelPermitReply
			err = json.Unmarshal(reply.Data, &res)
			if err != nil {
				fmt.Println("Errr 03", err)
				return deliverer.ConnectReply{}, err
			}

			fmt.Printf("Reply %+v\n", res.ChannelPermits)
			// TODO change role here
			for _, cp := range res.ChannelPermits {
				channels = append(channels, &ChannelRole{
					Name: fmt.Sprintf("%s-%d", cp.ChannelName, cp.ChannelID),
					Role: cp.Role,
				})
			}
		}

		if len(channels) == 0 {
			return deliverer.ConnectReply{}, errors.New("Not identified")
		}

		credentials := &deliverer.Credentials{
			UserID:   fmt.Sprintf("%s@%s", id, user.UserID),
			ExpireAt: time.Now().Unix() + 300000,
		}

		fmt.Println("Connect user", id)
		for _, c := range channels {
			subs[c.Name] = deliverer.SubscribeOptions{
				Role: c.Role,
			}
		}

		return deliverer.ConnectReply{
			ClientSideRefresh: true,
			Credentials:       credentials,
			Subscriptions:     subs,
		}, nil
	})

	node.OnConnect(func(client *deliverer.Client) {
		log.Printf("client %s connected via %s", client.UserID(), client.Transport().Name())
		msgs := d.GetCacheKey(client.UserID())
		go func() {
			for _, ms := range msgs {
				client.Send([]byte(ms))
			}
		}()

		client.OnSubscribe(func(e deliverer.SubscribeEvent, cb deliverer.SubscribeCallback) {
			log.Printf("client %s subscribes on channel %s", client.UserID(), e.Channel)
			cb(deliverer.SubscribeReply{
				Options: deliverer.SubscribeOptions{
					Data: []byte(`{"user_count_id": "` + client.UserID() + `"}`),
				},
			}, nil)
		})

		client.OnPublish(func(e deliverer.PublishEvent, cb deliverer.PublishCallback) {
			cid := client.UserID()
			uid := strings.Split(cid, "@")[1]
			log.Printf("client %s publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))
			var cmd workflow.CmdReplyTask
			err := json.Unmarshal(e.Data, &cmd)
			d.redis.Del(fmt.Sprintf("%s-%d", cmd.Channel, cmd.DeliverID))
			if err != nil {
				d.logger.WithField("onpublish", "receiver data").Error(err)
				return
			}
			go func() {
				d.reply(uid, string(e.Data))
			}()
			// workflowID, cmd, payload, cb
			cb(deliverer.PublishReply{}, nil)
		})

		client.OnDisconnect(func(e deliverer.DisconnectEvent) {
			log.Printf("client %s type disconnected", client.UserID())
		})
	})

	if err := node.Run(); err != nil {
		return err
	}
	d.node = node

	return nil
}

func (d *DelivererServer) reply(natChan string, data string) {
	fmt.Printf("[DELIVER REPLY NATS] Channel = %s, Data=%+v \n", natChan, data)
	d.sub.Con.Publish(natChan, []byte(data))
}

func (s *DelivererServer) checkSameHost(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		s.logger.Error(fmt.Errorf("failed to parse Origin header %q: %w", origin, err))
		return false
	}
	if strings.Contains(r.Host, "localhost") && strings.Contains(u.Host, "localhost") {
		return true
	}
	s.logger.Error(fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host))
	return false
}

func NewRedisDB(host, port, password string) *redis.Client {
	redisClient := redis.NewClient(&redis.Options{
		Addr:     fmt.Sprintf("%s:%s", host, port),
		Password: password,
		DB:       0,
	})
	return redisClient
}

func (d *DelivererServer) GetCacheKey(pattern string) []string {
	var matchingKeys []string
	iter := d.redis.Scan(0, pattern, 0).Iterator()
	for iter.Next() {
		key := iter.Val()
		matchingKeys = append(matchingKeys, key)
	}
	if err := iter.Err(); err != nil {
		d.logger.Error(err)
	}
	return matchingKeys
}
