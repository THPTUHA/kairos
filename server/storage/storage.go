package storage

import (
	"database/sql"
	"fmt"

	"github.com/rs/zerolog/log"
)

var db *sql.DB
var dbURI string

func Connect(uri string) error {
	var err error
	dbURI = uri
	db, err = sql.Open("mysql", uri)

	if err != nil {
		return err
	}

	err = db.Ping()
	if err != nil {
		return err
	}

	fmt.Println("Connected to MySQL database!")
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
