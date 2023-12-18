package deliverer

type PublishOption func(*PublishOptions)

type SubscribeOption func(*SubscribeOptions)

type RefreshOptions struct {
	Expired   bool
	ExpireAt  int64
	Info      []byte
	clientID  string
	sessionID string
}

type RefreshOption func(options *RefreshOptions)

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

func WithChannelInfo(chanInfo []byte) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.ChannelInfo = chanInfo
	}
}

func WithEmitPresence(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EmitPresence = enabled
	}
}

func WithEmitJoinLeave(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.EmitJoinLeave = enabled
	}
}

func WithPushJoinLeave(enabled bool) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.PushJoinLeave = enabled
	}
}

func WithSubscribeClient(clientID string) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.clientID = clientID
	}
}

func WithSubscribeData(data []byte) SubscribeOption {
	return func(opts *SubscribeOptions) {
		opts.Data = data
	}
}

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
