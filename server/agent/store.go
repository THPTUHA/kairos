package agent

import (
	"sync"

	"github.com/tidwall/buntdb"
)

type Store struct {
	db   *buntdb.DB
	lock *sync.Mutex // for

}

func NewStore() (*Store, error) {
	db, err := buntdb.Open(":memory:")
	if err != nil {
		return nil, err
	}

	store := &Store{
		db:   db,
		lock: &sync.Mutex{},
	}

	return store, nil
}
