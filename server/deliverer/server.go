package deliverer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	log "github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/httpserver/auth"
	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/THPTUHA/kairos/server/storage/models"
	"github.com/gorilla/mux"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type DelivererServer struct {
	node   *Node
	sub    *natsSub
	config *ServerConfig
	logger *logrus.Entry
}

type ObjectStatusCmd struct {
	Cmd      int    `json:"cmd"`
	ObjectID string `json:"object_id"`
	Type     int    `json:"type"`
	Status   int    `json:"status"`
}

func NewDelivererServer(file string) (*DelivererServer, error) {
	log := log.InitLogger(logrus.DebugLevel.String(), "deliverer")
	var d DelivererServer
	err := d.createNode()
	if err != nil {
		return nil, err
	}

	config, err := SetConfig(file)
	if err != nil {
		return nil, err
	}

	d.config = config
	// d.redis = NewRedisDB(RedisHost, RedisPort, "")
	err = d.createSub(log)
	if err != nil {
		return nil, err
	}
	d.logger = log
	return &d, nil
}

type ChannelRole struct {
	Name string
	Role int32
}

func (d *DelivererServer) createSub(log *logrus.Entry) error {
	sub, err := NewNatsSub(&natsSubConfig{
		url:           d.config.Nats.URL,
		name:          "deliverer",
		reconnectWait: 2 * time.Second,
		maxReconnects: 10,
		logger:        log,
	})
	if err != nil {
		d.logger.Error(err)
		return err
	}
	d.sub = sub
	return nil
}

func (d *DelivererServer) createNode() error {
	node, err := NewNode(Config{
		LogLevel:   LogLevelDebug,
		LogHandler: d.handleLog,
	})

	if err != nil {
		return err
	}
	node.OnConnecting(func(ctx context.Context, e ConnectEvent) (ConnectReply, error) {
		user, err := auth.ExtractAccessStr(e.Token)
		if err != nil {
			return ConnectReply{}, err
		}

		subs := make(map[string]SubscribeOptions)
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
			go func() {
				us := ObjectStatusCmd{
					Cmd:      workflow.ObjectStatusWorkflow,
					ObjectID: user.ClientID,
					Type:     models.ClientUser,
					Status:   1,
				}
				js, _ := json.Marshal(us)
				d.node.Publish(fmt.Sprintf("kairosuser-%s", user.UserID), js)
			}()
		case models.ChannelUser:
			id = fmt.Sprintf("kairoschannel-%s", user.ClientID)
			certID, _ := strconv.ParseInt(user.ClientID, 10, 64)
			req := proto.ChannelPermitRequest{
				CertID: certID,
			}
			data, err := json.Marshal(&req)
			if err != nil {
				d.logger.Error(err)
				return ConnectReply{}, err
			}

			reply, err := d.sub.Con.Request(fmt.Sprintf("%s.%d", messaging.INFOMATION, proto.Command_ChannelPermitRequest), data, 5*time.Second)
			if err != nil {
				d.logger.Error(err)
				return ConnectReply{}, err
			}
			var res proto.ChannelPermitReply
			err = json.Unmarshal(reply.Data, &res)
			if err != nil {
				d.logger.Error(err)
				return ConnectReply{}, err
			}

			fmt.Printf("Reply %+v\n", res.ChannelPermits)
			for _, cp := range res.ChannelPermits {
				channels = append(channels, &ChannelRole{
					Name: fmt.Sprintf("%s-%d", cp.ChannelName, cp.ChannelID),
					Role: cp.Role,
				})
			}
		}

		if len(channels) == 0 {
			return ConnectReply{}, errors.New("Not identified")
		}

		credentials := &Credentials{
			UserID:   fmt.Sprintf("%s@%s", id, user.UserID),
			ExpireAt: time.Now().Unix() + 30000000,
		}

		for _, c := range channels {
			subs[c.Name] = SubscribeOptions{
				Role: c.Role,
			}
		}

		return ConnectReply{
			ClientSideRefresh: true,
			Credentials:       credentials,
			Subscriptions:     subs,
		}, nil
	})

	node.OnConnect(func(client *Client) {
		fmt.Printf("client %s connected via %s", client.UserID(), client.Transport().Name())

		client.OnSubscribe(func(e SubscribeEvent, cb SubscribeCallback) {
			fmt.Printf("client %s subscribes on channel %s", client.UserID(), e.Channel)
			cb(SubscribeReply{
				Options: SubscribeOptions{
					Data: []byte(`{"user_count_id": "` + client.UserID() + `"}`),
				},
			}, nil)
		})

		client.OnPublish(func(e PublishEvent, cb PublishCallback) {
			cid := client.UserID()
			uid := strings.Split(cid, "@")[1]
			fmt.Printf("client %s publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))
			var cmd workflow.CmdReplyTask
			err := json.Unmarshal(e.Data, &cmd)
			if err != nil {
				d.logger.WithField("onpublish", "receiver data").Error(err)
				return
			}
			go func() {
				if cmd.WorkflowID != 0 {
					d.logger.WithField("cmd", cmd.Cmd).Infof("to workflow id = %d data=%+v", cmd.WorkflowID, string(e.Data))
					d.reply(fmt.Sprintf("w-%d", cmd.WorkflowID), string(e.Data))
				} else {
					d.logger.WithField("cmd", cmd.Cmd).Infof("to user %s", string(e.Data))
					d.reply(uid, string(e.Data))
				}
			}()
			// workflowID, cmd, payload, cb
			cb(PublishReply{}, nil)
		})

		client.OnDisconnect(func(e DisconnectEvent) {
			id := client.UserID()
			ids := strings.Split(id, "@")
			uid := ids[1]
			cid := strings.Split(ids[0], "-")[1]
			go func() {
				us := ObjectStatusCmd{
					Cmd:      workflow.ObjectStatusWorkflow,
					ObjectID: cid,
					Type:     models.ClientUser,
					Status:   0,
				}
				js, _ := json.Marshal(us)
				d.node.Publish(fmt.Sprintf("kairosuser-%s", uid), js)
			}()
			fmt.Printf("client %s type disconnected", client.UserID())
		})

	})

	if err := node.Run(); err != nil {
		return err
	}
	d.node = node

	return nil
}

func (d *DelivererServer) handleLog(e LogEntry) {
	d.logger.Debugf(fmt.Sprintf("%s: %v", e.Message, e.Fields))
}

func (d *DelivererServer) reply(natChan string, data string) {
	d.sub.Con.Publish(natChan, []byte(data))
}

func (s *DelivererServer) Start() error {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals,
		os.Interrupt,
	)

	go s.startSub(signals)
	auth.Init(s.config.Auth.HmacSecret, s.config.Auth.HmrfSecret)
	router := mux.NewRouter().StrictSlash(true)
	router.Handle("/pubsub", NewWebsocketHandler(s.node, WebsocketConfig{
		CheckOrigin: s.checkSameHost,
	}))

	fmt.Printf("Deliver running on %d \n", s.config.Deliverer.Port)

	go func() {
		<-signals
		os.Exit(0)
	}()

	if err := http.ListenAndServe(fmt.Sprintf(":%d", s.config.Deliverer.Port), router); err != nil {
		return err
	}
	return nil
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

	s.sub.Con.Subscribe(messaging.CLIENT_STATUS, func(msg *nats.Msg) {
		var userIDs []string
		err := json.Unmarshal(msg.Data, &userIDs)
		if err != nil {
			s.logger.WithField("deliver", "client status").Error(err)
			return
		}
		userInfos := s.node.InfoUsers(userIDs)
		data, _ := json.Marshal(userInfos)
		msg.Respond(data)
	})

	s.sub.Con.Subscribe(messaging.REMOVE_CHANNEL, func(msg *nats.Msg) {
		var ch models.Channel
		err := json.Unmarshal(msg.Data, &ch)
		if err != nil {
			s.logger.WithField("deliver", "remove channel").Error(err)
			return
		}

		err = s.node.Unsubscribe(fmt.Sprint(ch.UserID), ch.Name)
		if err != nil {
			s.logger.WithField("deliver", "remove channel").Error(err)
			return
		}
		msg.Respond([]byte("success"))
	})

	s.sub.Subscribes(subList, signals)

}

func (s *DelivererServer) checkSameHost(r *http.Request) bool {
	return true
	// origin := r.Header.Get("Origin")
	// if origin == "" {
	// 	return true
	// }
	// u, err := url.Parse(origin)
	// if err != nil {
	// 	s.logger.Error(fmt.Errorf("failed to parse Origin header %q: %w", origin, err))
	// 	return false
	// }
	// if strings.Contains(r.Host, "localhost") && strings.Contains(u.Host, "localhost") {
	// 	return true
	// }
	// s.logger.Error(fmt.Errorf("request Origin %q is not authorized for Host %q", origin, r.Host))
	// return false
}

type natsSubConfig struct {
	url           string
	name          string
	reconnectWait time.Duration
	maxReconnects int

	logger *logrus.Entry
}

type natsSub struct {
	config *natsSubConfig
	Con    *nats.Conn
}

func NewNatsSub(config *natsSubConfig) (*natsSub, error) {
	nc, err := nats.Connect(config.url, optNats(config)...)
	if err != nil {
		config.logger.Error(err)
		return nil, err
	}
	return &natsSub{
		config: config,
		Con:    nc,
	}, nil
}

func optNats(o *natsSubConfig) []nats.Option {
	opts := make([]nats.Option, 0)
	opts = append(opts, nats.Name(o.name))
	opts = append(opts, nats.MaxReconnects(o.maxReconnects))
	opts = append(opts, nats.ReconnectWait(o.reconnectWait))
	return opts
}

func (ns *natsSub) Subscribes(subs map[string]nats.MsgHandler, signals chan os.Signal) {
	for subject, cb := range subs {
		_, err := ns.Con.Subscribe(subject, cb)
		if err != nil {
			ns.config.logger.Error(err)
			return
		}
	}
	// ns.con.Flush()
	<-signals
	ns.config.logger.Warn("stop subscribe nat")
}
