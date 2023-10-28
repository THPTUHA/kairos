package storage

import (
	"github.com/THPTUHA/kairos/agent"
	"github.com/sirupsen/logrus"
)

var Store agent.DeamonStorage

func Init(log *logrus.Entry) error {
	store, err := agent.NewStore(log, true)
	if err != nil {
		return err
	}
	Store = store
	return nil
}
