package pubsub

import "github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"

// ClientInfo contains information about client connection.
type ClientInfo struct {
	// Client is a client unique id.
	Client string
	// User is an ID of authenticated user. Zero value means anonymous user.
	User string
	// ConnInfo is additional information about connection.
	ConnInfo []byte
	// ChanInfo is additional information about connection in context of
	// channel subscription.
	ChanInfo []byte
}

// Publication is a data sent to channel.
type Publication struct {
	// Offset is an incremental position number inside history stream.
	// Zero value means that channel does not maintain Publication stream.
	Offset uint64
	// Data published to channel.
	Data []byte
	// Info is optional information about client connection published
	// this data to channel.
	Info *ClientInfo
	// Tags contain custom key-value pairs attached to Publication.
	Tags map[string]string
}

func newCommandEncoder(enc deliverprotocol.Type) deliverprotocol.CommandEncoder {
	if enc == deliverprotocol.TypeJSON {
		return deliverprotocol.NewJSONCommandEncoder()
	}
	return deliverprotocol.NewProtobufCommandEncoder()
}

func newReplyDecoder(enc deliverprotocol.Type, data []byte) deliverprotocol.ReplyDecoder {
	if enc == deliverprotocol.TypeJSON {
		return deliverprotocol.NewJSONReplyDecoder(data)
	}
	return deliverprotocol.NewProtobufReplyDecoder(data)
}

func infoFromProto(v *deliverprotocol.ClientInfo) ClientInfo {
	info := ClientInfo{
		Client: v.GetClient(),
		User:   v.GetUser(),
	}
	if len(v.ConnInfo) > 0 {
		info.ConnInfo = v.ConnInfo
	}
	if len(v.ChanInfo) > 0 {
		info.ChanInfo = v.ChanInfo
	}
	return info
}

func pubFromProto(pub *deliverprotocol.Publication) Publication {
	p := Publication{
		Offset: pub.GetOffset(),
		Data:   pub.Data,
		Tags:   pub.GetTags(),
	}
	if pub.GetInfo() != nil {
		info := infoFromProto(pub.GetInfo())
		p.Info = &info
	}
	return p
}

func errorFromProto(err *deliverprotocol.Error) *Error {
	return &Error{Code: err.Code, Message: err.Message, Temporary: err.Temporary}
}
