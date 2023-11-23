package main

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
		fmt.Println(" NATS CONNECT ", msg.Subject, cmd, proto.Command_ChannelPermitRequest.Number(),
			fmt.Sprint(proto.Command_ChannelPermitRequest.Number()) == cmd,
		)
		switch cmd {
		case fmt.Sprint(proto.Command_ChannelPermitRequest.Number()):
			fmt.Println("Run here 1")
			var req proto.ChannelPermitRequest
			err := json.Unmarshal(msg.Data, &req)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			fmt.Println("Run here 2")
			cp, err := storage.GetChannelInfoByCertID(req.CertID)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			fmt.Println("Run here 3")
			var res proto.ChannelPermitReply
			res.ChannelPermits = make([]*proto.ChannelPermit, 0)
			for _, e := range cp {
				res.ChannelPermits = append(res.ChannelPermits, &proto.ChannelPermit{
					ChannelID:   e.ID,
					ChannelName: e.Name,
					Role:        e.Role,
				})

			}
			fmt.Println("Run here 4", len(res.ChannelPermits))
			dataRes, err := json.Marshal(&res)
			if err != nil {
				fmt.Println("err", err)
				return
			}
			fmt.Println("Run here 5")
			err = msg.Respond(dataRes)
			if err != nil {
				fmt.Println("err", err)
				return
			}
		}
	})
}
