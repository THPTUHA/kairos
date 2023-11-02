package storage

import (
	"database/sql"
	"fmt"

	"github.com/THPTUHA/kairos/server/storage/models"
	_ "github.com/lib/pq"
	"github.com/rs/zerolog/log"
)

var db *sql.DB
var dbURI string

func Connect(uri string) error {
	var err error
	dbURI = uri
	db, err = sql.Open("postgres", uri)

	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		return err
	}

	fmt.Println("Connected to postgres database!")
	return nil
}

func Get() *sql.DB {
	if db == nil {
		Connect(dbURI)
	}
	err := db.Ping()
	if err != nil {
		log.Error().Msg(err.Error())
	}
	return db
}

type TaskOptions struct {
	ID         int64
	Time       int64
	WorkflowID int64
	UserID     int64
}

type WorkflowOptions struct {
	ID           int64
	UserID       int64
	CollectionID int64
}

type CollectionOptions struct {
	ID     int64
	UserID int64
	Path   string
}

type Storage interface {
	GetTasks(options []*TaskOptions) ([]*models.Task, error)
	GetWorkflows(options []*WorkflowOptions) ([]*models.Workflow, error)
	GetCollections(options []*CollectionOptions) ([]*models.Collections, error)
	SetCollections(collection *models.Collections) (int64, error)
	UpdateCollection(taskRecord *models.TaskRecord) error
}
