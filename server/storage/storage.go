package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"github.com/THPTUHA/kairos/pkg/workflow"
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

func CreateWorkflow(userID string, w *workflow.WorkflowFile) (int64, error) {
	ctx := context.Background()
	tx, err := Get().BeginTx(ctx, nil)
	if err != nil {
		return -1, err
	}
	rawData, err := json.Marshal(w)
	if err != nil {
		return -1, err
	}
	defer tx.Rollback()

	exist, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM workflows WHERE user_id = %s AND namespace = '%s' AND name = '%s' 
	`, userID, w.Namespace, w.Name))
	var wid int64
	for exist.Next() {
		err = exist.Scan(&wid)
		if err != nil && err != sql.ErrNoRows {
			return -1, err
		}

		if err == nil {
			return -1, fmt.Errorf("Workflow %s namespace = %s exist", w.Name, w.Namespace)
		}
	}

	q := fmt.Sprintf(`INSERT INTO 
			workflows(user_id,namespace,name,status,version,raw_data,created_at,updated_at)  
			VALUES (%s,'%s','%s',%d,'%s','%s',%d,%d)`,
		userID,
		w.Namespace,
		w.Name,
		models.WorkflowPending,
		w.Version.String(),
		rawData,
		helper.GetTimeNow(),
		helper.GetTimeNow())
	_, err = tx.ExecContext(ctx, q)

	if err != nil {
		fmt.Println("Fuck")
		return -1, err
	}

	result, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM workflows WHERE user_id = %s AND namespace = '%s' AND name = '%s'
	`, userID, w.Namespace, w.Name))

	if err != nil {
		return -1, err
	}

	for result.Next() {
		err = result.Scan(&wid)
		if err != nil {
			return -1, err
		}
	}

	if w.Vars != nil && w.Vars.Len() > 0 {
		sqlVar := `INSERT INTO vars(
			key,
			value,
			workflow_id
		) VALUES `
		values := make([]string, 0)
		w.Vars.Range(func(name string, value *workflow.Var) error {
			values = append(values, fmt.Sprintf("('%s','%s',%d)", name, value.Value, wid))
			return nil
		})

		_, err = tx.ExecContext(ctx, sqlVar+strings.Join(values, ","))
		if err != nil {
			fmt.Println("Err 3")
			return -1, err
		}
	}

	if w.Tasks.Len() > 0 {
		sqlTask := `INSERT INTO tasks(
			name,
			deps,
			schedule,
			timezone,
			clients,
			retries,
			executor,
			duration,
			workflow_id,
			status,
			payload,
			expires_at
		) VALUES `
		values := make([]string, 0)
		w.Tasks.Range(func(key string, value *workflow.Task) error {
			deps, err := json.Marshal(value.Deps)
			if err != nil {
				return err
			}
			clients, err := json.Marshal(value.Clients)
			if err != nil {
				return err
			}
			values = append(values, fmt.Sprintf("('%s','%s','%s','%s','%s',%d,'%s','%s',%d,%d,'%s','%s')",
				key,
				string(deps),
				value.Schedule,
				value.Timezone,
				string(clients),
				value.Retries,
				value.Executor,
				value.Duration,
				wid,
				models.TaksPending,
				value.Payload,
				value.ExpiresAt,
			))
			return nil
		})

		_, err = tx.ExecContext(ctx, sqlTask+strings.Join(values, ","))
		if err != nil {
			fmt.Println("Err 4")
			return -1, err
		}
	}

	if w.Brokers.Len() > 0 {
		sqlBroker := `INSERT INTO brokers(
			name,
			listens,
			flows,
			workflow_id,
			status
		) VALUES`
		values := make([]string, 0)
		w.Brokers.Range(func(key string, value *workflow.Broker) error {
			listens, err := json.Marshal(value.Listens)
			if err != nil {
				return err
			}

			flows, err := json.Marshal(value.Flows)
			if err != nil {
				fmt.Println("Err 5")
				return err
			}

			values = append(values, fmt.Sprintf("('%s','%s','%s',%d,%d)",
				key,
				listens,
				flows,
				wid,
				models.BrokerPending,
			))

			return nil
		})
		_, err = tx.ExecContext(ctx, sqlBroker+strings.Join(values, ","))
		if err != nil {
			return -1, err
		}
	}

	err = tx.Commit()
	if err != nil {
		return -1, err
	}
	return wid, nil
}

func DropWorkflow(userID string, id string) (int64, error) {
	ctx := context.Background()
	tx, err := Get().BeginTx(ctx, nil)
	if err != nil {
		return -1, err
	}
	defer tx.Rollback()

	result, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM workflows WHERE user_id = %s AND id = %s 
	`, userID, id))

	if err != nil {
		return -1, err
	}

	var wid int64
	for result.Next() {
		err = result.Scan(&wid)
		if err != nil {
			return -1, fmt.Errorf("Workflow not exist")
		}
	}

	_, err = tx.QueryContext(ctx, fmt.Sprintf(`
		DELETE FROM workflows WHERE user_id = %s AND id = %s
	`, userID, id))

	if err != nil {
		return -1, err
	}

	err = tx.Commit()
	if err != nil {
		return -1, err
	}

	return wid, nil
}

func ListWorkflow(userID string, query map[string]string) ([]*models.Workflow, error) {
	sqlStr := `SELECT id, namespace, name,status, created_at `
	whereQ := make([]string, 0)
	whereQ = append(whereQ, fmt.Sprintf(" user_id = %s ", userID))
	if len(whereQ) > 0 {
		sqlStr += " WHERE " + strings.Join(whereQ, ",")
	}
	rows, err := Get().Query(sqlStr)
	if err != nil {
		return nil, err
	}
	workflows := make([]*models.Workflow, 0)
	for rows.Next() {
		var w models.Workflow
		rows.Scan(
			&w.ID,
			&w.Namespace,
			&w.Name,
			&w.Status,
			&w.CreatedAt,
		)
		workflows = append(workflows, &w)
	}
	return workflows, nil
}

func SetClient(client *models.Client) error {
	fmt.Println("----- RUn here---")
	var existingName string
	err := Get().QueryRow("SELECT name FROM clients WHERE name = $1", client.Name).Scan(&existingName)

	if err == sql.ErrNoRows {
		_, err = Get().Exec("INSERT INTO clients(name, kairos_name, user_id) VALUES($1, $2, $3)",
			client.Name, client.KairosName, client.UserID)
		return err
	}

	if err != nil {
		return err
	}

	_, err = db.Exec("UPDATE clients SET kairos_name = $1, user_id = $2 WHERE name = $3",
		client.KairosName, client.UserID, client.Name)

	return err
}

type TaskQuery struct {
	workflowID int64
}

func DetailWorkflow(workflowID int64) (*workflow.Workflow, error) {
	query := `
		SELECT id, namespace, name, status, version, created_at, updated_at
		FROM workflows
		WHERE id = $1
	`

	row := db.QueryRow(query, workflowID)
	var w workflow.Workflow

	err := row.Scan(
		&w.ID, &w.Namespace, &w.Name, &w.Status, &w.Version,
		&w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		fmt.Println("err 1")
		return nil, err
	}

	taskM, err := GetTasks(&TaskQuery{workflowID: workflowID})
	if err != nil {
		fmt.Println("err 2")
		return nil, err
	}

	w.Tasks = workflow.Tasks{
		OrderedMap: orderedmap.New[string, *workflow.Task](),
	}

	for _, t := range taskM {
		task := workflow.Task{
			ID:        t.ID,
			Name:      t.Name,
			Schedule:  t.Schedule,
			Timezone:  t.Timezone,
			Retries:   t.Retries,
			Executor:  t.Executor,
			Payload:   t.Payload,
			ExpiresAt: t.ExpiresAt,
		}
		fmt.Println("taskID", t.ID)
		json.Unmarshal([]byte(t.Deps), &task.Deps)
		json.Unmarshal([]byte(t.Clients), &task.Clients)
		json.Unmarshal([]byte(t.Clients), &task.Clients)

		w.Tasks.Set(t.Name, &task)
	}

	brokerM, err := GetBrokers(&BrokerQuery{workflowID: workflowID})
	if err != nil {
		fmt.Println("err 3")
		return nil, err
	}
	w.Brokers = workflow.Brokers{
		OrderedMap: orderedmap.New[string, *workflow.Broker](),
	}
	for _, b := range brokerM {
		broker := workflow.Broker{
			ID:   b.ID,
			Name: b.Name,
		}

		json.Unmarshal([]byte(b.Listens), &broker.Listens)
		json.Unmarshal([]byte(b.Flows), &broker.Flows)
		w.Brokers.Set(broker.Name, &broker)
	}

	varsM, err := GetVars(&VarsQuery{workflowID: workflowID})
	if err != nil {
		fmt.Println("err 5")
		return nil, err
	}
	w.Vars = &workflow.Vars{
		OrderedMap: orderedmap.New[string, *workflow.Var](),
	}
	for _, v := range varsM {
		_v := workflow.Var{
			ID:    v.ID,
			Value: v.Value,
		}

		w.Vars.Set(v.Key, &_v)
	}
	return &w, err
}

func DetailWorkflowModel(workflowID int64) (*models.Workflow, error) {
	query := `
		SELECT id, namespace, name, status, version, created_at, updated_at
		FROM workflows
		WHERE id = $1
	`

	row := db.QueryRow(query, workflowID)
	var w models.Workflow

	err := row.Scan(
		&w.ID, &w.Namespace, &w.Name, &w.Status, &w.Version,
		&w.CreatedAt, &w.UpdatedAt)
	if err != nil {
		fmt.Println("err 1")
		return nil, err
	}

	return &w, err
}

func GetTasks(q *TaskQuery) ([]*models.Task, error) {
	query := `
		SELECT id, name, deps, schedule, timezone, clients, retries, executor, duration, workflow_id, status, payload, expires_at
		FROM tasks
		WHERE workflow_id = $1
	`

	rows, err := Get().Query(query, q.workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task

	for rows.Next() {
		var task models.Task
		err := rows.Scan(
			&task.ID, &task.Name, &task.Deps, &task.Schedule, &task.Timezone, &task.Clients, &task.Retries,
			&task.Executor, &task.Duration, &task.WorkflowID, &task.Status, &task.Payload, &task.ExpiresAt,
		)
		if err != nil {
			return nil, err
		}
		tasks = append(tasks, &task)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tasks, nil
}

type BrokerQuery struct {
	workflowID int64
}

func GetBrokers(q *BrokerQuery) ([]*models.Broker, error) {
	query := `
		SELECT id, name, listens, flows, workflow_id, status
		FROM brokers
		WHERE workflow_id = $1
	`

	rows, err := Get().Query(query, q.workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var brokers []*models.Broker

	for rows.Next() {
		var broker models.Broker
		err := rows.Scan(
			&broker.ID, &broker.Name, &broker.Listens, &broker.Flows, &broker.WorkflowID, &broker.Status,
		)
		if err != nil {
			return nil, err
		}
		brokers = append(brokers, &broker)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return brokers, nil
}

type ClientQuery struct {
	Name   string
	UserID string
}

func GetClient(q *ClientQuery) (*models.Client, error) {
	query := `
		SELECT id, kairos_name, name, user_id
		FROM clients
		WHERE name = $1 AND user_id = $2
	`
	var client models.Client

	err := Get().QueryRow(query, q.Name, q.UserID).Scan(
		&client.ID,
		&client.KairosName,
		&client.Name,
		&client.UserID,
	)

	if err != nil {
		return nil, err
	}

	return &client, err
}

type VarsQuery struct {
	workflowID int64
}

func GetVars(q *VarsQuery) ([]*models.Vars, error) {
	query := `
		SELECT id, key, value, workflow_id
		FROM vars
		WHERE workflow_id = $1
	`

	rows, err := Get().Query(query, q.workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var vars []*models.Vars

	for rows.Next() {
		var v models.Vars
		err := rows.Scan(
			&v.ID, &v.Key, &v.Value, &v.WorkflowID,
		)
		if err != nil {
			return nil, err
		}
		vars = append(vars, &v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return vars, nil
}

type Storage interface {
	UpdateWorkflow(workflow *workflow.WorkflowFile) error
}
