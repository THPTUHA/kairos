package runner

import (
	"encoding/json"
	"fmt"

	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/nats-io/nats.go"
	"github.com/sirupsen/logrus"
)

type UserResource struct {
	UserID   int64
	Channels map[int64][]int64
	natConn  *nats.Conn
	log      *logrus.Entry
}

func NewUserResource(userID int64) (*UserResource, error) {
	natConn, err := nats.Connect(nats.DefaultURL, optNatsWorker(defaultNatsOptions())...)
	if err != nil {
		return nil, err
	}

	return &UserResource{
		UserID:   userID,
		Channels: make(map[int64][]int64),
		natConn:  natConn,
		log:      logger.InitLogger("debug", "user_resource"),
	}, nil
}

func (ur *UserResource) Run() error {
	_, err := ur.natConn.Subscribe(fmt.Sprintf("user-%d", ur.UserID), func(msg *nats.Msg) {
		var reply workflow.CmdReplyTask
		err := json.Unmarshal(msg.Data, &reply)
		if err != nil {
			ur.log.Error(err)
			return
		}

	})

	return err
}
