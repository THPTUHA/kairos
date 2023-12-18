package deliverer

import (
	"context"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
)

type ConnectEvent struct {
	ClientID  string
	Token     string
	Data      []byte
	Name      string
	Transport TransportInfo
	Channels  []string
}

type ConnectReply struct {
	Context            context.Context
	Credentials        *Credentials
	Data               []byte
	Subscriptions      map[string]SubscribeOptions
	ClientSideRefresh  bool
	Storage            map[string]any
	MaxMessagesInFrame int
	WriteDelay         time.Duration
	ReplyWithoutQueue  bool
	QueueInitialCap    int
	PingPongConfig     *PingPongConfig
}

type ConnectingHandler func(context.Context, ConnectEvent) (ConnectReply, error)

type ConnectHandler func(*Client)

type RefreshEvent struct {
	ClientSideRefresh bool
	Token             string
}

type RefreshReply struct {
	Expired  bool
	ExpireAt int64
	Info     []byte
}

type RefreshCallback func(RefreshReply, error)
type RefreshHandler func(RefreshEvent, RefreshCallback)

type AliveHandler func()

type UnsubscribeEvent struct {
	Channel    string
	ServerSide bool
	Unsubscribe
	Disconnect *Disconnect
}

type UnsubscribeHandler func(UnsubscribeEvent)

type DisconnectEvent struct {
	Disconnect
}

type DisconnectHandler func(DisconnectEvent)
type SubscribeEvent struct {
	Channel string
	Token   string
	Data    []byte
}

type SubscribeCallback func(SubscribeReply, error)

type SubscribeReply struct {
	Options           SubscribeOptions
	ClientSideRefresh bool
	SubscriptionReady chan struct{}
}

type SubscribeHandler func(SubscribeEvent, SubscribeCallback)

type PublishEvent struct {
	Channel    string
	Data       []byte
	ClientInfo *ClientInfo
}

type PublishReply struct {
	Options PublishOptions
	Result  *PublishResult
}

type PublishCallback func(PublishReply, error)

type PublishHandler func(PublishEvent, PublishCallback)

type SubRefreshEvent struct {
	ClientSideRefresh bool
	Channel           string
	Token             string
}

type SubRefreshReply struct {
	Expired  bool
	ExpireAt int64
	Info     []byte
}

type SubRefreshCallback func(SubRefreshReply, error)
type SubRefreshHandler func(SubRefreshEvent, SubRefreshCallback)

type MessageEvent struct {
	Data []byte
}

type MessageHandler func(MessageEvent)

type PresenceEvent struct {
	Channel string
}

type PresenceReply struct {
	Result *PresenceResult
}

type PresenceCallback func(PresenceReply, error)

type PresenceHandler func(PresenceEvent, PresenceCallback)

type PresenceStatsEvent struct {
	Channel string
}

type PresenceStatsReply struct {
	Result *PresenceStatsResult
}

type PresenceStatsCallback func(PresenceStatsReply, error)

type PresenceStatsHandler func(PresenceStatsEvent, PresenceStatsCallback)

type StateSnapshotHandler func() (any, error)

type NotificationEvent struct {
	FromNodeID string
	Op         string
	Data       []byte
}

type NotificationHandler func(NotificationEvent)

type NodeInfoSendReply struct {
	Data []byte
}

type NodeInfoSendHandler func() NodeInfoSendReply

type TransportWriteEvent struct {
	Data      []byte
	Channel   string
	FrameType deliverprotocol.FrameType
}

type TransportWriteHandler func(*Client, TransportWriteEvent) bool

type CommandReadEvent struct {
	Command     *deliverprotocol.Command
	CommandSize int
}

type CommandReadHandler func(*Client, CommandReadEvent) error
