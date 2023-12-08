package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	kplugin "github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/lib/pq"
)

type Input struct {
	Sql        string        `json:"sql"`
	Params     []interface{} `json:"params"`
	DeliverID  int64         `json:"deliver_id"`
	WorkflowID int64         `json:"workflow_id"`
	Error      error         `json:"error"`
}

type Sql struct {
}

var db *sql.DB
var err error
var id int

func NewDatabase(driverName, dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open(driverName, dataSourceName)
	if err != nil {
		return nil, err
	}

	err = db.Ping()
	if err != nil {
		return nil, err
	}

	return db, nil
}

func (s *Sql) Execute(args *proto.ExecuteRequest, cb kplugin.StatusHelper) (*proto.ExecuteResponse, error) {
	out, err := s.ExecuteImpl(args, cb)
	resp := &proto.ExecuteResponse{Output: out}
	if err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

func initDB(args *proto.ExecuteRequest) (*sql.DB, error) {
	id++
	fmt.Println("ID ", id)
	if db != nil && args.Config["instance"] == "one" {
		return db, nil
	}
	fmt.Println("CONNECT")
	driver := args.Config["driver"]
	host := args.Config["host"]
	port := args.Config["port"]
	user := args.Config["user"]
	password := args.Config["password"]
	dbname := args.Config["dbname"]
	otherCofig := args.Config["otherConfig"]

	switch driver {
	case "mysql":
		db, err = sql.Open("mysql", fmt.Sprintf("%s:%s@tcp(%s:%s)/%s", user, password, host, port, dbname))
	case "postgres":
		db, err = NewDatabase(driver, fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s %s",
			host,
			port,
			user,
			password,
			dbname,
			otherCofig,
		))
	}
	return db, err
}

func (s *Sql) ExecuteImpl(args *proto.ExecuteRequest, cb kplugin.StatusHelper) ([]byte, error) {
	instance := args.Config["instance"] == "one"

	fmt.Println(" INSTART", instance)
	db, err = initDB(args)
	if err != nil {
		return nil, err
	}

	if !instance {
		defer db.Close()
	}

	sqlCh := make(chan *Input)

	go func() {
		for {
			var input Input
			if instance {
				data := cb.Input()
				err := json.Unmarshal([]byte(data), &input)
				if err != nil {
					input.Error = err
				}
				sqlCh <- &input
			} else {
				// if !instance {

				// } else {
				// 	b := cb.Input()
				// 	fmt.Println("GET INPUT HERE", string(b))
				// 	json.Unmarshal(b, &input)
				// }
				if args.Config["deliver_id"] != "" {
					did, err := strconv.ParseInt(args.Config["deliver_id"], 10, 64)
					if err != nil {
						input.Error = err
						fmt.Println("Error parse deliver_id ")
					}
					input.DeliverID = did
				}

				if args.Config["worklfow_id"] != "" {
					wid, err := strconv.ParseInt(args.Config["worklfow_id"], 10, 64)
					if err != nil {
						input.Error = err
						fmt.Println("Error parse deliver_id ")
					}
					input.WorkflowID = wid
				}
				input.Sql = args.Config["sql"]
				if args.Config["params"] != "" {
					err := json.Unmarshal([]byte(args.Config["params"]), &input.Params)
					if err != nil {
						input.Error = err
					}
				}

				if args.Config["input"] != "" {
					err := json.Unmarshal([]byte(args.Config["input"]), &input)
					if err != nil {
						input.Error = err
					}
				}
				sqlCh <- &input
				return
			}

		}
	}()

	for {
		select {
		case input := <-sqlCh:
			fmt.Printf("RUN HERE---- %+v\n", input)

			if input.Error != nil {
				if instance {
					cb.Update([]byte(err.Error()), false)
				}
				return nil, input.Error
			}

			_, err := initDB(args)
			if err != nil {
				if instance {
					cb.Update([]byte(err.Error()), false)
				}
				return nil, err
			}

			fmt.Printf("PARAMS %+v\n", input.Params)
			rows, err := db.Query(input.Sql, input.Params...)
			if err != nil {
				fmt.Println("ERROR 1")
				if instance {
					cb.Update([]byte(err.Error()), false)
				}
				return nil, err
			}
			columns, err := rows.Columns()
			if err != nil {
				fmt.Println("ERROR 2")
				return nil, err
			}
			if err != nil {
				cb.Update([]byte(err.Error()), false)
			}

			values := make([]interface{}, len(columns))

			for i := range columns {
				var value interface{}
				values[i] = &value
			}
			collections := make([]map[string]interface{}, 0)

			for rows.Next() {
				err := rows.Scan(values...)
				if err != sql.ErrNoRows && err != nil {
					cb.Update([]byte(err.Error()), false)
				}

				results := make(map[string]interface{}, len(columns))

				for i, v := range values {
					val := reflect.Indirect(reflect.ValueOf(v))
					t, ok := val.Interface().([]byte)
					if ok {
						results[columns[i]] = string(t)
					} else {
						t, ok := val.Interface().(string)
						if ok {
							results[columns[i]] = t
						}
					}
				}

				for _, v := range results {
					fmt.Println(v)
				}
				collections = append(collections, results)
			}

			str, err := json.Marshal(collections)
			if err != nil {
				return nil, err
			}
			cb.Update(str, true)

			if instance {
				cb.Update(nil, true)
			} else {
				return nil, nil
			}
			// return nil, nil
		}
	}
}
