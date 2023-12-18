package pubsub

import "github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"

type ClientInfo struct {
	Client   string
	User     string
	ConnInfo []byte
	ChanInfo []byte
}

type Publication struct {
	Data []byte
	Info *ClientInfo
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
		Data: pub.Data,
		Tags: pub.GetTags(),
	}
	if pub.GetInfo() != nil {
		info := infoFromProto(pub.GetInfo())
		p.Info = &info
	}
	return p
}

func errorFromProto(err *deliverprotocol.Error) *Error {
	return &Error{Code: err.Code, Message: err.Message}
}
