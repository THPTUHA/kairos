package deliverer

import "time"

// PublishOption is a type to represent various Publish options.
type PublishOption func(*PublishOptions)

// SubscribeOption is a type to represent various Subscribe options.
type SubscribeOption func(*SubscribeOptions)

// WithHistory tells Broker to save message to history stream with provided size and ttl.
func WithHistory(size int, ttl time.Duration, metaTTL ...time.Duration) PublishOption {
	return func(opts *PublishOptions) {
		opts.HistorySize = size
		opts.HistoryTTL = ttl
		if len(metaTTL) > 0 {
			opts.HistoryMetaTTL = metaTTL[0]
		}
	}
}

// SubscribeOptions define per-subscription options.
type SubscribeOptions struct {
	// ExpireAt defines time in future when subscription should expire,
	// zero value means no expiration.
	ExpireAt int64
	// ChannelInfo defines custom channel information, zero value means no channel information.
	ChannelInfo []byte
	// EmitPresence turns on participating in channel presence - i.e. client
	// subscription will emit presence updates to PresenceManager and will be visible
	// in a channel presence result.
	EmitPresence bool
	// EmitJoinLeave turns on emitting Join and Leave events from the subscribing client.
	// See also PushJoinLeave if you want current client to receive join/leave messages.
	EmitJoinLeave bool
	// PushJoinLeave turns on receiving channel Join and Leave events by the client.
	// Subscriptions which emit join/leave events should have EmitJoinLeave on.
	PushJoinLeave bool
	// When position is on client will additionally sync its position inside a stream
	// to prevent publication loss. The loss can happen due to at most once guarantees
	// of PUB/SUB model. Make sure you are enabling EnablePositioning in channels that
	// maintain Publication history stream. When EnablePositioning is on Centrifuge will
	// include StreamPosition information to subscribe response - for a client to be
	// able to manually track its position inside a stream.
	EnablePositioning bool
	// EnableRecovery turns on automatic recovery for a channel. In this case
	// client will try to recover missed messages upon resubscribe to a channel
	// after reconnect to a server. This option also enables client position
	// tracking inside a stream (i.e. enabling EnableRecovery will automatically
	// enable EnablePositioning option) to prevent occasional publication loss.
	// Make sure you are using EnableRecovery in channels that maintain Publication
	// history stream.
	EnableRecovery bool
	// Data to send to a client with Subscribe Push.
	Data []byte
	// RecoverSince will try to subscribe a client and recover from a certain StreamPosition.
	RecoverSince *StreamPosition

	// HistoryMetaTTL allows to override default (set in Config.HistoryMetaTTL) history
	// meta information expiration time.
	HistoryMetaTTL time.Duration

	// clientID to subscribe.
	clientID string
	// sessionID to subscribe.
	sessionID string
	// Source is a way to mark the source of Subscription - i.e. where it comes from. May be useful
	// for inspection of a connection during its lifetime.
	Source uint8
}

// NoLimit defines that limit should not be applied.
const NoLimit = -1

// HistoryOption is a type to represent various History options.
type HistoryOption func(options *HistoryOptions)

func WithHistoryFilter(filter HistoryFilter) HistoryOption {
	return func(opts *HistoryOptions) {
		opts.Filter = filter
	}
}

func WithHistoryMetaTTL(metaTTL time.Duration) HistoryOption {
	return func(opts *HistoryOptions) {
		opts.MetaTTL = metaTTL
	}
}

// WithExpireAt allows setting ExpireAt field.
func WithExpireAt(expireAt int64) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.ExpireAt = expireAt
	}
}

// WithChannelInfo ...
func WithChannelInfo(chanInfo []byte) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.ChannelInfo = chanInfo
	}
}

// WithEmitPresence ...
func WithEmitPresence(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EmitPresence = enabled
	}
}

// WithEmitJoinLeave ...
func WithEmitJoinLeave(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EmitJoinLeave = enabled
	}
}

// WithPushJoinLeave ...
func WithPushJoinLeave(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.PushJoinLeave = enabled
	}
}

// WithPositioning ...
func WithPositioning(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EnablePositioning = enabled
	}
}

// WithRecovery ...
func WithRecovery(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EnableRecovery = enabled
	}
}

// WithSubscribeClient allows setting client ID that should be subscribed.
// This option not used when Client.Subscribe called.
func WithSubscribeClient(clientID string) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.clientID = clientID
	}
}

// WithSubscribeSession allows setting session ID that should be subscribed.
// This option not used when Client.Subscribe called.
func WithSubscribeSession(sessionID string) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.sessionID = sessionID
	}
}

// WithSubscribeData allows setting custom data to send with subscribe push.
func WithSubscribeData(data []byte) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.Data = data
	}
}

// WithRecoverSince allows setting SubscribeOptions.RecoverFrom.
func WithRecoverSince(since *StreamPosition) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.RecoverSince = since
	}
}

// WithSubscribeSource allows setting SubscribeOptions.Source.
func WithSubscribeSource(source uint8) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.Source = source
	}
}

func WithClientInfo(info *ClientInfo) PublishOption {
	return func(opts *PublishOptions) {
		opts.ClientInfo = info
	}
}
