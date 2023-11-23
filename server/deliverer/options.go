package deliverer

// PublishOption is a type to represent various Publish options.
type PublishOption func(*PublishOptions)

// SubscribeOption is a type to represent various Subscribe options.
type SubscribeOption func(*SubscribeOptions)

type RefreshOptions struct {
	// Expired can close connection with expired reason.
	Expired bool
	// ExpireAt defines time in future when subscription should expire,
	// zero value means no expiration.
	ExpireAt int64
	// Info defines custom channel information, zero value means no channel information.
	Info []byte
	// clientID to refresh.
	clientID string
	// sessionID to refresh.
	sessionID string
}

// RefreshOption is a type to represent various Refresh options.
type RefreshOption func(options *RefreshOptions)

// SubscribeOptions define per-subscription options.
type SubscribeOptions struct {
	// ExpireAt defines time in future when subscription should expire,
	// zero value means no expiration.
	ExpireAt    int64
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
	// Data to send to a client with Subscribe Push.
	Data []byte
	// RecoverSince will try to subscribe a client and recover from a certain StreamPosition.
	RecoverSince *StreamPosition
	// clientID to subscribe.
	clientID string
	// Source is a way to mark the source of Subscription - i.e. where it comes from. May be useful
	// for inspection of a connection during its lifetime.
	Source uint8
	Role   int32
}

type UnsubscribeOptions struct {
	// clientID to unsubscribe.
	clientID string
	// sessionID to unsubscribe.
	sessionID string
	// custom unsubscribe object.
	unsubscribe *Unsubscribe
}

// UnsubscribeOption is a type to represent various Unsubscribe options.
type UnsubscribeOption func(options *UnsubscribeOptions)

type DisconnectOptions struct {
	// Disconnect represents custom disconnect to use.
	// By default, DisconnectForceNoReconnect will be used.
	Disconnect *Disconnect
	// ClientWhitelist contains client IDs to keep.
	ClientWhitelist []string
	// clientID to disconnect.
	clientID string
	// sessionID to disconnect.
	sessionID string
}

// DisconnectOption is a type to represent various Disconnect options.
type DisconnectOption func(options *DisconnectOptions)

// NoLimit defines that limit should not be applied.
const NoLimit = -1

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

// WithSubscribeClient allows setting client ID that should be subscribed.
// This option not used when Client.Subscribe called.
func WithSubscribeClient(clientID string) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.clientID = clientID
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

// WithRefreshExpired to set expired flag - connection will be closed with DisconnectExpired.
func WithRefreshExpired(expired bool) RefreshOption {
	return func(opts *RefreshOptions) {
		opts.Expired = expired
	}
}

func WithRefreshExpireAt(expireAt int64) RefreshOption {
	return func(opts *RefreshOptions) {
		opts.ExpireAt = expireAt
	}
}

// WithRefreshInfo to override connection info.
func WithRefreshInfo(info []byte) RefreshOption {
	return func(opts *RefreshOptions) {
		opts.Info = info
	}
}
