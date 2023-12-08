package deliverer

type PublishOption func(*PublishOptions)

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
	ExpireAt      int64
	ChannelInfo   []byte
	EmitPresence  bool
	EmitJoinLeave bool
	PushJoinLeave bool
	Data          []byte
	clientID      string
	Source        uint8
	Role          int32
}

type UnsubscribeOptions struct {
	clientID    string
	sessionID   string
	unsubscribe *Unsubscribe
}

type UnsubscribeOption func(options *UnsubscribeOptions)

type DisconnectOptions struct {
	Disconnect      *Disconnect
	ClientWhitelist []string
	clientID        string
	sessionID       string
}

type DisconnectOption func(options *DisconnectOptions)

const NoLimit = -1

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
