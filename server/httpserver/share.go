package httpserver

import (
	"encoding/json"
	"fmt"

	"github.com/THPTUHA/kairos/server/messaging"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/THPTUHA/kairos/server/storage"
	"github.com/nats-io/nats.go"
)

func (server *HttpServer) serviceNats() {
	server.NatConn.Subscribe(fmt.Sprintf("%s.*", messaging.INFOMATION), func(msg *nats.Msg) {
		cmd := msg.Subject[len(messaging.INFOMATION)+1:]

		switch cmd {
		case fmt.Sprint(proto.Command_ChannelPermitRequest.Number()):
			var req proto.ChannelPermitRequest
			err := json.Unmarshal(msg.Data, &req)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			cp, err := storage.GetChannelInfoByCertID(req.CertID)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			var res proto.ChannelPermitReply
			res.ChannelPermits = make([]*proto.ChannelPermit, 0)
			for _, e := range cp {
				res.ChannelPermits = append(res.ChannelPermits, &proto.ChannelPermit{
					ChannelID:   e.ID,
					ChannelName: e.Name,
					Role:        e.Role,
				})

			}
			dataRes, err := json.Marshal(&res)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			err = msg.Respond(dataRes)
			if err != nil {
				fmt.Println("err", err)
				return
			}
		}
	})
}
