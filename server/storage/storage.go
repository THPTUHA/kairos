package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/THPTUHA/kairos/pkg/helper"
	"github.com/THPTUHA/kairos/pkg/logger"
	"github.com/THPTUHA/kairos/pkg/orderedmap"
	"github.com/THPTUHA/kairos/pkg/workflow"
	"github.com/THPTUHA/kairos/server/storage/models"
	_ "github.com/lib/pq"
	"github.com/sirupsen/logrus"
)

var db *sql.DB
var dbURI string
var log *logrus.Entry

func Connect(uri string) error {
	var err error
	dbURI = uri
	db, err = sql.Open("postgres", uri)

	if err != nil {
		log.Error(err)
		return err
	}
	log = logger.InitLogger("debug", "storage")
	err = db.Ping()
	if err != nil {
		log.Error(err)
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
		log.Error(err)
	}
	return db
}

func GetClients(userID string, name string) ([]*models.Client, error) {
	query := `
		SELECT id, name, user_id
		FROM clients
		WHERE user_id = $1 AND name = $2`

	rows, err := Get().Query(query, userID, name)
	if err != nil {
		log.Error(err)
		return nil, err
	}
	defer rows.Close()

	var clients []*models.Client

	for rows.Next() {
		var client models.Client
		err := rows.Scan(&client.ID, &client.Name, &client.UserID)
		if err != nil {
			log.Error(err)
			return nil, err
		}
		clients = append(clients, &client)
	}

	if err := rows.Err(); err != nil {
		log.Error(err)
		return nil, err
	}

	return clients, nil
}

func CreateWorkflow(userID string, w *workflow.WorkflowFile, rawData string) (int64, error) {
	ctx := context.Background()
	tx, err := Get().BeginTx(ctx, nil)
	if err != nil {
		return -1, err
	}

	if err != nil {
		log.Error(err)
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
			return 0, fmt.Errorf("Workflow %s namespace = %s exist", w.Name, w.Namespace)
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
		log.Error(err)
		return -1, err
	}

	result, err := tx.QueryContext(ctx, fmt.Sprintf(`
		SELECT id FROM workflows WHERE user_id = %s AND namespace = '%s' AND name = '%s'
	`, userID, w.Namespace, w.Name))

	if err != nil {
		log.Error(err)
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
			log.Error(err)
			return -1, err
		}
	}

	if w.Tasks.Len() > 0 {
		sqlTask := `INSERT INTO tasks(
			name,
			schedule,
			timezone,
			clients,
			retries,
			executor,
			workflow_id,
			status,
			payload,
			expires_at,
			wait
		) VALUES `
		values := make([]string, 0)
		w.Tasks.Range(func(key string, value *workflow.Task) error {
			if err != nil {
				log.Error(err)
				return err
			}
			clients, err := json.Marshal(value.Clients)
			if err != nil {
				log.Error(err)
				return err
			}
			values = append(values, fmt.Sprintf("('%s','%s','%s','%s',%d,'%s',%d,%d,'%s','%s','%s')",
				key,
				value.Schedule,
				value.Timezone,
				string(clients),
				value.Retries,
				value.Executor,
				wid,
				models.TaksPending,
				value.Payload,
				value.ExpiresAt,
				value.Wait,
			))
			return nil
		})

		_, err = tx.ExecContext(ctx, sqlTask+strings.Join(values, ","))
		if err != nil {
			log.Error(err)
			return -1, err
		}
	}

	if w.Brokers.Len() > 0 {
		sqlBroker := `INSERT INTO brokers(
			name,
			listens,
			flows,
			workflow_id,
			status,
			standard_name,
			clients
		) VALUES`
		values := make([]string, 0)
		w.Brokers.Range(func(key string, value *workflow.Broker) error {
			listens, err := json.Marshal(value.Listens)
			if err != nil {
				log.Error(err)
				return err
			}

			flows, err := json.Marshal(value.Flows)
			if err != nil {
				log.Error(err)
				return err
			}

			str, err := json.Marshal(value.Clients)
			values = append(values, fmt.Sprintf("('%s','%s','%s',%d,%d,'%s','%s')",
				key,
				listens,
				flows,
				wid,
				models.BrokerPending,
				strings.ToLower(key),
				str,
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
	sqlStr := `SELECT id, namespace, name,status, created_at, raw_data FROM workflows `
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
			&w.RawData,
		)
		workflows = append(workflows, &w)
	}
	return workflows, nil
}

func GetWorkflows() ([]int64, error) {
	sqlStr := `SELECT id FROM workflows `
	rows, err := Get().Query(sqlStr)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		rows.Scan(
			&id,
		)
		ids = append(ids, id)
	}
	return ids, nil
}

func SetClient(client *models.Client) (int64, error) {
	var existingName string
	err := Get().QueryRow("SELECT id, name FROM clients WHERE name = $1", client.Name).Scan(&client.ID, &existingName)
	fmt.Println("SET CLIENT ---")
	if err == sql.ErrNoRows {
		err = Get().QueryRow("INSERT INTO clients(name, user_id, created_at, active_since) VALUES($1, $2, $3,$4) RETURNING id",
			client.Name, client.UserID, client.CreatedAt, client.ActiveSince).Scan(&client.ID)
		return client.ID, err
	}

	if err != nil {
		return -1, err
	}

	_, err = db.Exec("UPDATE clients SET user_id = $1, active_since= $2 WHERE name = $3", client.UserID, client.ActiveSince, client.Name)

	return client.ID, err
}

type TaskQuery struct {
	WID int64
}

func DetailWorkflow(workflowID int64, userID string) (*workflow.Workflow, error) {
	var row *sql.Row
	if userID == "" {
		query := `
		SELECT id, namespace, name, status, version, created_at, updated_at, user_id
		FROM workflows
		WHERE id = $1`

		row = db.QueryRow(query, workflowID)
	} else {
		query := `
		SELECT id, namespace, name, status, version, created_at, updated_at, user_id
		FROM workflows
		WHERE id = $1 AND user_id = $2`

		row = db.QueryRow(query, workflowID, userID)
	}

	var w workflow.Workflow

	err := row.Scan(
		&w.ID, &w.Namespace, &w.Name, &w.Status, &w.Version,
		&w.CreatedAt, &w.UpdatedAt, &w.UserID,
	)

	if err != nil {
		fmt.Println("err 1")
		return nil, err
	}

	listens := make(map[string]bool)

	taskM, err := GetTasks(&TaskQuery{WID: workflowID})
	if err != nil {
		fmt.Println("err 2")
		return nil, err
	}

	w.Tasks = workflow.Tasks{
		OrderedMap: orderedmap.New[string, *workflow.Task](),
	}

	for _, t := range taskM {
		task := workflow.Task{
			ID:         t.ID,
			WorkflowID: workflowID,
			Name:       t.Name,
			Schedule:   t.Schedule,
			Timezone:   t.Timezone,
			Retries:    t.Retries,
			Executor:   t.Executor,
			Payload:    t.Payload,
			ExpiresAt:  t.ExpiresAt,
			Wait:       t.Wait,
			Metadata:   map[string]string{},
		}
		json.Unmarshal([]byte(t.Clients), &task.Clients)
		json.Unmarshal([]byte(t.Clients), &task.Clients)
		for _, c := range task.Clients {
			clients, err := GetClients(fmt.Sprint(w.UserID), c)
			if err != nil {
				return nil, err
			}
			if len(clients) > 1 {
				return nil, fmt.Errorf(" more than one client ")
			}
			task.Metadata[fmt.Sprint(clients[0].Name)] = fmt.Sprint(clients[0].ID)
			listens[c] = true
		}
		w.Tasks.Set(t.Name, &task)
	}

	brokerM, err := GetBrokers(&BrokerQuery{WorkflowID: workflowID})
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
		if b.Clients != "" {
			json.Unmarshal([]byte(b.Clients), &broker.Clients)
		}
		w.Brokers.Set(broker.Name, &broker)
		for _, c := range broker.Clients {
			listens[c] = true
		}
		for _, c := range broker.Listens {
			listens[workflow.GetRawName(c)] = true
		}
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
	names := make([]string, 0)
	for n := range listens {
		names = append(names, fmt.Sprintf("'%s'", n))
	}
	fmt.Printf("NAME --- %+v \n", names)
	channelM, err := GetChannels(&ChannelOptions{
		UserID: fmt.Sprint(w.UserID),
		Names:  names[:0],
	})
	if err != nil {
		fmt.Println("err 7")
		return nil, err
	}
	w.Channels = make([]*workflow.Channel, 0)
	for _, c := range channelM {
		w.Channels = append(w.Channels, &workflow.Channel{
			ID:   c.ID,
			Name: c.Name,
		})
	}

	clientM, err := GetAllClient(fmt.Sprint(w.UserID), names)
	if err != nil {
		fmt.Println("err 6")
		return nil, err
	}

	w.Clients = make([]*workflow.Client, 0)
	for _, c := range clientM {
		w.Clients = append(w.Clients, &workflow.Client{
			ID:   c.ID,
			Name: c.Name,
		})
	}

	fmt.Printf("CLIENTS WORKFLOW %+v \n", w.Clients)
	fmt.Printf("CHANNELS WORKFLOW %+v \n", w.Channels)
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
		SELECT id, name, schedule, timezone, clients, retries, executor, workflow_id, status, payload, expires_at,wait
		FROM tasks
		WHERE workflow_id = $1
	`

	rows, err := Get().Query(query, q.WID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []*models.Task

	for rows.Next() {
		var task models.Task
		err := rows.Scan(
			&task.ID, &task.Name, &task.Schedule, &task.Timezone, &task.Clients, &task.Retries,
			&task.Executor, &task.WorkflowID, &task.Status, &task.Payload, &task.ExpiresAt, &task.Wait,
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
	WorkflowID int64
	ID         int64
}

func GetBrokers(q *BrokerQuery) ([]*models.Broker, error) {
	query := `
		SELECT id, name, listens, flows, workflow_id, status, clients
		FROM brokers
	`
	whereQ := make([]string, 0)
	if q.WorkflowID > 0 {
		whereQ = append(whereQ, fmt.Sprintf(" workflow_id = %d ", q.WorkflowID))
	}
	if q.ID > 0 {
		whereQ = append(whereQ, fmt.Sprintf(" id = %d ", q.ID))
	}

	if len(whereQ) > 0 {
		query += fmt.Sprintf(" WHERE %s ", strings.Join(whereQ, " AND "))
	}
	rows, err := Get().Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var brokers []*models.Broker

	for rows.Next() {
		var broker models.Broker
		err := rows.Scan(
			&broker.ID, &broker.Name, &broker.Listens, &broker.Flows, &broker.WorkflowID, &broker.Status, &broker.Clients,
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
		SELECT id, name, user_id
		FROM clients
		WHERE name = $1 AND user_id = $2
	`
	var client models.Client

	err := Get().QueryRow(query, q.Name, q.UserID).Scan(
		&client.ID,
		&client.Name,
		&client.UserID,
	)

	if err != nil {
		return nil, err
	}

	return &client, err
}

func GetAllClient(userID string, names []string) ([]*models.Client, error) {
	query := `
		SELECT id, name, user_id, active_since, created_at
		FROM clients
		WHERE user_id = $1
	`
	if names != nil && len(names) > 0 {
		query = fmt.Sprintf("%s AND name in (%s) ", query, strings.Join(names, ","))
	}

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clients []*models.Client

	for rows.Next() {
		var client models.Client
		if err := rows.Scan(&client.ID, &client.Name, &client.UserID, &client.ActiveSince, &client.CreatedAt); err != nil {
			return nil, err
		}
		clients = append(clients, &client)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return clients, nil
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

func LogMessageFlow(mf *models.MessageFlow) (int64, error) {
	// if mf.Flow == models.DeliverFlow {
	// 	var stauts int
	// 	err := db.QueryRow("SELECT id,status FROM message_flows WHERE workflow_id = $1 AND deliver_id = $2 AND created_at = $3",
	// 		mf.WorkflowID, mf.DeliverID, mf.CreatedAt).Scan(&mf.ID, &stauts)
	// 	if err == sql.ErrNoRows {
	// 		return InsertMessageFlow(mf)
	// 	}
	// 	if err == nil && stauts == workflow.Delivering {
	// 		if _, err = db.Exec("UPDATE message_flows SET status=$1,elapsed_time=$2,response_size=$3  WHERE id=$4",
	// 			mf.Status, mf.ElapsedTime, mf.ResponseSize, mf.ID); err != nil {
	// 			return mf.ID, err
	// 		}

	// 	}
	// 	return mf.ID, err
	// }
	return InsertMessageFlow(mf)
}

func InsertMessageFlow(mf *models.MessageFlow) (int64, error) {
	stmt, err := db.Prepare(`
		INSERT INTO message_flows (
			status, sender_id, sender_type, sender_name, receiver_id,
			receiver_type, receiver_name, workflow_id, message, attempt,
			created_at, flow, deliver_id, request_size, response_size,
			cmd, start, "group", task_id, send_at, receive_at, task_name,
			part, parent, begin_part, finish_part, tracking, broker_group, start_input
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28,$29
		) RETURNING id
	`)
	if err != nil {
		return -1, err
	}
	defer stmt.Close()

	// Execute the SQL statement and retrieve the generated ID
	err = stmt.QueryRow(
		mf.Status, mf.SenderID, mf.SenderType, mf.SenderName, mf.ReceiverID,
		mf.ReceiverType, mf.ReceiverName, mf.WorkflowID, mf.Message, mf.Attempt,
		mf.CreatedAt, mf.Flow, mf.DeliverID, mf.RequestSize, mf.ResponseSize,
		mf.Cmd, mf.Start, mf.Group, mf.TaskID, mf.SendAt, mf.ReceiveAt, mf.TaskName,
		mf.Part, mf.Parent, mf.BeginPart, mf.FinishPart, mf.Tracking, mf.BrokerGroup, mf.StartInput,
	).Scan(&mf.ID)
	if err != nil {
		return -1, err
	}

	return mf.ID, nil
}

func InsertChannel(channel *models.Channel) error {
	query := `
		INSERT INTO channels (user_id, name)
		VALUES ($1, $2)
		RETURNING id`

	err := Get().QueryRow(
		query,
		channel.UserID, channel.Name,
	).Scan(&channel.ID)

	return err
}

func SetBrokerQueue(brokerQueue *models.BrokerQueue) error {
	_, err := Get().Exec("INSERT INTO broker_queues(key, value, workflow_id, used, created_at) VALUES($1, $2, $3, $4, $5)",
		brokerQueue.Key, brokerQueue.Value, brokerQueue.WorkflowID, brokerQueue.Used, brokerQueue.CreatedAt)
	return err
}

func GetKVQueue(key string, workflowID int64) (*models.BrokerQueue, error) {
	fmt.Printf("Key = %s, workflowID= %d \n", key, workflowID)
	rows, err := db.Query("SELECT id, value FROM broker_queues WHERE key = $1 AND workflow_id = $2 AND used = false ORDER BY created_at DESC LIMIT 1", key, workflowID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bq models.BrokerQueue
	for rows.Next() {
		err := rows.Scan(&bq.ID, &bq.Value)
		if err != nil {
			return nil, err
		}

	}
	if bq.ID == 0 {
		return nil, errors.New("no row")
	}
	bq.Key = key
	return &bq, nil
}

func GetKVQueues(keys map[string]bool, workflowID int64) ([]*models.BrokerQueue, error) {
	bqs := make([]*models.BrokerQueue, 0)
	for k := range keys {
		bq, err := GetKVQueue(k, workflowID)
		if err != nil {
			return nil, err
		}
		bqs = append(bqs, bq)
	}

	return bqs, nil
}

func UsedKVQueue(ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	idsString := strings.Trim(strings.Join(strings.Fields(fmt.Sprint(ids)), ","), "[]")

	_, err := db.Exec("UPDATE broker_queues SET used = true WHERE id IN (" + idsString + ")")
	if err != nil {
		return err
	}
	return nil
}

func buildPlaceholders(count int) string {
	placeholders := make([]byte, count*2-1)
	for i := range placeholders {
		placeholders[i] = '$'
		if i%2 == 1 {
			placeholders[i] = ','
		}
	}
	return string(placeholders)
}

type ChannelOptions struct {
	UserID string
	Name   string
	Names  []string
}

func GetChannels(opt *ChannelOptions) ([]*models.Channel, error) {
	query := "SELECT id, user_id, name, created_at FROM channels "

	var values []interface{}
	var index int
	q := make([]string, 0)
	if opt.UserID != "" {
		index++
		q = append(q, fmt.Sprintf(" user_id = $%d ", index))
		values = append(values, opt.UserID)
	}

	if opt.Name != "" {
		index++
		q = append(q, fmt.Sprintf(" name = $%d ", index))
		values = append(values, opt.Name)
	}

	if len(opt.Names) > 0 {
		index++
		q = append(q, fmt.Sprintf(" name in (%s) ", strings.Join(opt.Names, ",")))
	}

	if len(q) > 0 {
		query += " WHERE " + strings.Join(q, " AND ")
	}

	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	var channels []*models.Channel
	rows, err := stmt.Query(values...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var channel models.Channel
		if err := rows.Scan(&channel.ID, &channel.UserID, &channel.Name, &channel.CreatedAt); err != nil {
			return nil, err
		}
		channels = append(channels, &channel)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return channels, nil
}

func CreateChannel(c *models.Channel) (int64, error) {
	query := "INSERT INTO channels (user_id, name, created_at) VALUES ($1, $2, $3) RETURNING id"
	stmt, err := db.Prepare(query)
	if err != nil {
		return -1, err
	}
	defer stmt.Close()

	err = stmt.QueryRow(c.UserID, c.Name, c.CreatedAt).Scan(&c.ID)
	if err != nil {
		return -1, err
	}

	return c.ID, nil
}

func DeleteChannels(userID, channelName string) error {
	query := "DELETE FROM channels WHERE name = $1 AND user_id = $2"
	stmt, err := db.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(channelName, userID)
	if err != nil {
		return err
	}

	return nil
}

func GetInfoUser(userID string) (*models.User, error) {
	query := "SELECT id, username, email, avatar  FROM users WHERE id = $1"
	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()
	var user models.User
	err = stmt.QueryRow(userID).Scan(&user.ID, &user.Username, &user.Email, &user.Avatar)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

func addChannelPermission(tx *sql.Tx, certID int64, role int, channelID int64) error {
	fmt.Println("CERT---", certID, role, channelID)
	_, err := tx.Exec(`
		INSERT INTO channel_permissions (cert_id, role, channel_id)
		VALUES ($1, $2, $3)
	`, certID, role, channelID)
	return err
}

func createCertificate(tx *sql.Tx, name string, userID int64, apiKey string, expireAt int64, createdAt int64) (int64, error) {
	var id int64
	err := tx.QueryRow(`
		INSERT INTO certificates (user_id, api_key, secret_key, expire_at, created_at,name)
		VALUES ($1, $2, $3, $4, $5,$6) RETURNING id
	`, userID, apiKey, "", expireAt, createdAt, name).Scan(&id)
	if err != nil {
		return -1, err
	}
	return id, nil
}

func CreateCertificate(name string, userID int64, apiKey string, expireAt int64, pers []*models.ChannelPermission, cb func(int64) (string, error)) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return -1, err
	}
	defer func() {
		if p := recover(); p != nil {
			_ = tx.Rollback()
			panic(p)
		} else if err != nil {
			_ = tx.Rollback()
		} else {
			err = tx.Commit()
		}
	}()

	certID, err := createCertificate(tx, name, userID, apiKey, expireAt, helper.GetTimeNow())
	fmt.Println("create Cert", err)
	if err != nil {
		return -1, err
	}

	secretKey, err := cb(certID)
	if err != nil {
		return -1, err
	}

	_, err = tx.Exec(`
		UPDATE certificates
		SET secret_key = $1
		WHERE id = $2
	`, secretKey, certID)

	for _, cp := range pers {
		err = addChannelPermission(tx, certID, cp.Role, cp.ChannelID)
		if err != nil {
			return -1, err
		}
	}

	return certID, nil
}

type ChannelPermit struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Role int32  `json:"role"`
}

func GetChannelInfoByCertID(certID int64) ([]*ChannelPermit, error) {
	rows, err := Get().Query(`
		SELECT c.id, c.name, cp.role
		FROM channels c
		JOIN channel_permissions cp ON c.id = cp.channel_id
		WHERE cp.cert_id = $1
	`, certID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	channelPermits := make([]*ChannelPermit, 0)
	for rows.Next() {
		var permis ChannelPermit
		err := rows.Scan(&permis.ID, &permis.Name, &permis.Role)
		if err != nil {
			return nil, err
		}
		channelPermits = append(channelPermits, &permis)
	}

	return channelPermits, nil
}

func GetCertificatesByUserID(userID int64) ([]*models.Certificate, error) {
	rows, err := db.Query(`
		SELECT id, user_id, api_key, secret_key, expire_at, created_at,name
		FROM certificates
		WHERE user_id = $1
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certificates []*models.Certificate
	for rows.Next() {
		var certificate models.Certificate
		err := rows.Scan(&certificate.ID, &certificate.UserID, &certificate.APIKey, &certificate.SecretKey, &certificate.ExpireAt, &certificate.CreatedAt, &certificate.Name)
		if err != nil {
			return nil, err
		}
		certificates = append(certificates, &certificate)
	}

	return certificates, nil
}

func CreateClient(client *models.Client) (int64, error) {
	err := db.QueryRow("INSERT INTO clients (name, user_id, active_since, created_at) VALUES ($1, $2, $3, $4) RETURNING id",
		client.Name, client.UserID, client.ActiveSince, client.CreatedAt).Scan(&client.ID)

	if err != nil {
		return -1, err
	}

	return client.ID, nil
}

func GetCertificatesID(certID int64) (*models.Certificate, error) {
	rows, err := db.Query(`
		SELECT id, user_id, api_key, secret_key, expire_at, created_at,name
		FROM certificates
		WHERE id = $1
	`, certID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var certificate models.Certificate

	for rows.Next() {
		err := rows.Scan(&certificate.ID, &certificate.UserID, &certificate.APIKey, &certificate.SecretKey, &certificate.ExpireAt, &certificate.CreatedAt, &certificate.Name)
		if err != nil {
			return nil, err
		}
	}

	return &certificate, nil
}

func InsertFunction(function *models.Function) (int64, error) {
	queryS := "SELECT id FROM functions WHERE name = $1 AND user_id = $2 limit 1"
	row := db.QueryRow(queryS, function.Name, function.UserID)
	err := row.Scan(&function.ID)
	if err == nil {
		return function.ID, fmt.Errorf("function %s exist", function.Name)
	}

	if err != sql.ErrNoRows {
		return function.ID, err
	}

	query := `
		INSERT INTO functions (user_id, content, created_at,name)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`

	var id int64
	err = Get().QueryRow(query, function.UserID, function.Content, function.CreatedAt, function.Name).Scan(&id)
	if err != nil {
		return -1, err
	}

	return id, nil
}

func GetAllFunctionsByUserID(userID int64) ([]*models.Function, error) {
	query := `
		SELECT id, user_id, content, created_at, name
		FROM functions
		WHERE user_id = $1
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var functions []*models.Function

	for rows.Next() {
		var function models.Function
		err := rows.Scan(&function.ID, &function.UserID, &function.Content, &function.CreatedAt, &function.Name)
		if err != nil {
			return nil, err
		}
		functions = append(functions, &function)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return functions, nil
}

func FindFunctionsByUserID(userID int64) ([]*models.Function, error) {
	query := `
		SELECT id, user_id, name, content, created_at
		FROM functions
		WHERE user_id = $1 
	`

	rows, err := db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var functions []*models.Function

	for rows.Next() {
		var function models.Function
		err := rows.Scan(&function.ID, &function.UserID, &function.Name, &function.Content, &function.CreatedAt)
		if err != nil {
			return nil, err
		}
		functions = append(functions, &function)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return functions, nil
}

func DeleteFunction(id int64) error {
	query := `
		DELETE FROM functions where id = $1
	`
	_, err := db.Query(query, id)
	if err != nil {
		return err
	}
	return nil
}

type GraphEdge struct {
	SenderID       int64   `json:"sender_id"`
	Count          int64   `json:"count"`
	ReceiverID     int64   `json:"receiver_id"`
	Flow           int     `json:"flow"`
	WorkflowID     int64   `json:"workflow_id"`
	SenderType     int     `json:"sender_type"`
	ReceiverType   int     `json:"receiver_type"`
	ElapsedTimeAVG float64 `json:"elapsed_time_avg"`
	Throughput     int64   `json:"throughput"`
}

func PerformCalculation(wids []int64) ([]*GraphEdge, error) {
	ids := make([]string, 0)
	for _, id := range wids {
		ids = append(ids, fmt.Sprint(id))
	}
	wfIDs := strings.Join(ids, ",")
	edges, err := countSenderReceiverPairs(wfIDs)
	if err != nil {
		return nil, err
	}
	return edges, nil
}

func countSenderReceiverPairs(wfIDs string) ([]*GraphEdge, error) {
	edges := make([]*GraphEdge, 0)
	query := `SELECT COUNT(*) as count, SUM(request_size + response_size) as throughput, sender_id, receiver_id, flow, workflow_id,sender_type,receiver_type FROM message_flows 
			WHERE workflow_id in (%s) GROUP BY sender_id, receiver_id, flow, workflow_id, sender_type, receiver_type `
	rows, err := Get().Query(fmt.Sprintf(query, wfIDs))
	if err != nil {
		return nil, err
	}

	for rows.Next() {
		var ge GraphEdge
		rows.Scan(&ge.Count, &ge.ElapsedTimeAVG, &ge.Throughput, &ge.SenderID, &ge.ReceiverID, &ge.Flow, &ge.WorkflowID, &ge.SenderType, &ge.ReceiverType)
		edges = append(edges, &ge)
	}
	return edges, nil
}

func GetMessageFlowsByUserID(userID int64) ([]*models.MessageFlow, error) {
	rows, err := db.Query(`
		SELECT
			mf.id,  mf.sender_id, mf.sender_type, mf.sender_name,
			mf.receiver_id, mf.receiver_type, mf.receiver_name, mf.workflow_id,
			mf.attempt, mf.created_at, mf.flow, mf.deliver_id, mf.cmd, mf.group, w.name,
			mf.part, mf.parent, mf.begin_part, mf.finish_part,mf.start, mf.message
		FROM
			message_flows mf
		JOIN
			workflows w ON mf.workflow_id = w.id
		WHERE
			w.user_id = $1 AND mf.start = true
		ORDER BY id DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messageFlows []*models.MessageFlow

	for rows.Next() {
		var mf models.MessageFlow
		err := rows.Scan(
			&mf.ID, &mf.SenderID, &mf.SenderType, &mf.SenderName,
			&mf.ReceiverID, &mf.ReceiverType, &mf.ReceiverName, &mf.WorkflowID,
			&mf.Attempt, &mf.CreatedAt, &mf.Flow, &mf.DeliverID,
			&mf.Cmd, &mf.Group, &mf.WorkflowName,
			&mf.Part, &mf.Parent, &mf.BeginPart, &mf.FinishPart, &mf.Start, &mf.Message,
		)
		if err != nil {
			return nil, err
		}
		messageFlows = append(messageFlows, &mf)
	}

	return messageFlows, nil
}

func GetMessageFlowsByGroupID(groupID string) ([]*models.MessageFlow, error) {
	rows, err := db.Query(`
		WITH flow_tops as (
			SELECT
				max(id) as id
			FROM
				message_flows
			WHERE
				"group" = $1
			GROUP BY sender_id, sender_type, receiver_id, receiver_type, task_id, part
		) SELECT mf.id, status, sender_id, sender_type, sender_name, receiver_id,
			receiver_type, receiver_name, workflow_id, message, attempt,
			created_at, flow, deliver_id, request_size, response_size,
			cmd, start, "group", task_id, send_at, receive_at, task_name,
			mf.part, mf.parent, mf.begin_part, mf.finish_part, mf.broker_group
		FROM message_flows mf, flow_tops ft
		WHERE mf.id = ft.id
		ORDER BY mf.id ASC
	`, groupID)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messageFlows []*models.MessageFlow

	for rows.Next() {
		var mf models.MessageFlow
		err := rows.Scan(
			&mf.ID, &mf.Status, &mf.SenderID, &mf.SenderType, &mf.SenderName, &mf.ReceiverID,
			&mf.ReceiverType, &mf.ReceiverName, &mf.WorkflowID, &mf.Message, &mf.Attempt,
			&mf.CreatedAt, &mf.Flow, &mf.DeliverID, &mf.RequestSize, &mf.ResponseSize,
			&mf.Cmd, &mf.Start, &mf.Group, &mf.TaskID, &mf.SendAt, &mf.ReceiveAt, &mf.TaskName,
			&mf.Part, &mf.Parent, &mf.BeginPart, &mf.FinishPart, &mf.BrokerGroup,
		)
		if err != nil {
			return nil, err
		}
		messageFlows = append(messageFlows, &mf)
	}

	return messageFlows, nil
}

func GetMessageFlowsByParent(part string, receiverID int64) ([]*models.MessageFlow, error) {
	query := fmt.Sprintf(`SELECT * FROM message_flows WHERE part  = '%s' AND (receiver_id =%d OR task_id = %d) ORDER BY ID DESC LIMIT 20`, part, receiverID, receiverID)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return exactMessageFlow(rows)
}

func GetMessageFlowsByParts(parts []any) ([]*models.MessageFlow, error) {
	placeholdersp := make([]string, len(parts))

	for i := range parts {
		placeholdersp[i] = fmt.Sprintf("$%d", i+1)
	}
	parentStr := strings.Join(placeholdersp, ",")

	query := fmt.Sprintf(`SELECT * FROM message_flows WHERE part IN (%s) ORDER BY ID DESC LIMIT 20`, parentStr)
	rows, err := db.Query(query, parts...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return exactMessageFlow(rows)
}

func GetGroupList(group string, limit string) ([]*models.MessageFlow, error) {
	query := fmt.Sprintf(`SELECT * FROM message_flows WHERE "group" = '%s' ORDER BY ID ASC LIMIT %s`, group, limit)
	fmt.Println("QQQQQ", query)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	return exactMessageFlow(rows)
}

func GetMessageFlowRoot(part string) ([]*models.MessageFlow, error) {
	query := fmt.Sprintf(`SELECT * FROM message_flows WHERE parent = '%s' AND part ='%s'`, part, part)
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	return exactMessageFlow(rows)
}

func exactMessageFlow(rows *sql.Rows) ([]*models.MessageFlow, error) {
	messageFlows := make([]*models.MessageFlow, 0)
	for rows.Next() {
		var mf models.MessageFlow
		err := rows.Scan(
			&mf.ID,
			&mf.Status,
			&mf.SenderID,
			&mf.SenderType,
			&mf.SenderName,
			&mf.ReceiverID,
			&mf.ReceiverType,
			&mf.ReceiverName,
			&mf.WorkflowID,
			&mf.Message,
			&mf.Attempt,
			&mf.CreatedAt,
			&mf.Flow,
			&mf.DeliverID,
			&mf.RequestSize,
			&mf.ResponseSize,
			&mf.Cmd,
			&mf.Start,
			&mf.Group,
			&mf.TaskID,
			&mf.SendAt,
			&mf.ReceiveAt,
			&mf.TaskName,
			&mf.Part,
			&mf.Parent,
			&mf.BeginPart,
			&mf.FinishPart,
			&mf.Tracking,
			&mf.BrokerGroup,
			&mf.StartInput,
		)
		if err != nil {
			return nil, err
		}
		messageFlows = append(messageFlows, &mf)
	}

	return messageFlows, nil
}

func AddTaskRecord(record *models.TaskRecord) error {
	query := "INSERT INTO task_records (status, output, task_id, started_at, finished_at, client_id, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id"
	err := db.QueryRow(query, record.Status, record.Output, record.TaskID, record.StartedAt, record.FinishedAt, record.ClientID, record.CreatedAt).Scan(&record.ID)
	if err != nil {
		return err
	}
	return nil
}

func UpdateTaskRecord(record *models.TaskRecord) error {
	query := "UPDATE task_records SET status=$1, output=$3, started_at=$4, finished_at=$5, client_id=$6, created_at=$7 WHERE task_id=$8 "
	_, err := db.Exec(query, record.Status, record.Output, record.StartedAt, record.FinishedAt, record.ClientID, record.CreatedAt, record.TaskID)
	if err != nil {
		return err
	}
	return nil
}

func InsertBrokerRecord(record *models.BrokerRecord) error {
	query := "INSERT INTO broker_records (status, input, output, broker_id, created_at) VALUES ($1, $2, $3, $4, $5) RETURNING id"
	err := db.QueryRow(query, record.Status, record.Input, record.Output, record.BrokerID, record.CreatedAt).Scan(&record.ID)
	if err != nil {
		return err
	}
	return nil
}

func GetTaskRecords(taskID, clientID int64, limit int) ([]*models.TaskRecord, error) {
	var query string
	var args []interface{}

	if taskID != 0 && clientID != 0 {
		query = "SELECT id, status, output, task_id, started_at, finished_at, client_id, created_at FROM task_records WHERE task_id = $1 AND client_id = $2 order by id desc limit $3"
		args = []interface{}{taskID, clientID, limit}
	} else if taskID != 0 {
		query = "SELECT id, status, output, task_id, started_at, finished_at, client_id, created_at FROM task_records WHERE task_id = $1 order by id desc limit $2"
		args = []interface{}{taskID, limit}
	} else if clientID != 0 {
		query = "SELECT id, status, output, task_id, started_at, finished_at, client_id, created_at FROM task_records WHERE client_id = $1 order by id desc limit $2"
		args = []interface{}{clientID, limit}
	} else {
		return nil, fmt.Errorf("Cần cung cấp ít nhất task_id hoặc client_id")
	}

	rows, err := Get().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.TaskRecord
	for rows.Next() {
		var record models.TaskRecord
		err := rows.Scan(&record.ID, &record.Status, &record.Output, &record.TaskID, &record.StartedAt, &record.FinishedAt, &record.ClientID, &record.CreatedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, &record)
	}

	return records, nil
}

func GetBrokerRecords(brokerID int64, limit int) ([]*models.BrokerRecord, error) {
	query := "SELECT id, status, input, output, broker_id, created_at FROM broker_records WHERE broker_id = $1 order by id desc limit $2"
	rows, err := db.Query(query, brokerID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.BrokerRecord
	for rows.Next() {
		var record models.BrokerRecord
		err := rows.Scan(&record.ID, &record.Status, &record.Input, &record.Output, &record.BrokerID, &record.CreatedAt)
		if err != nil {
			return nil, err
		}
		records = append(records, &record)
	}

	return records, nil
}

func GetMessageFlows(userID int64, workflowName string, limit string, offset string) ([]*models.MessageFlow, error) {
	var query string
	var args []interface{}

	if userID != 0 && workflowName != "" {
		query = `SELECT mf.id, mf.status, sender_id, sender_type, receiver_id, 
				receiver_type, workflow_id, message, attempt, mf.created_at, flow, 
				deliver_id, request_size, response_size,  
				sender_name, receiver_name,cmd. w.name as workflow_name,start_input
				FROM message_flows mf 
				JOIN workflows w ON mf.workflow_id = w.id 
				WHERE w.user_id = $1 AND w.name = $2 
				ORDER BY mf.id DESC OFFSET $3 LIMIT $4  `
		args = []interface{}{userID, workflowName, offset, limit}
	} else if userID != 0 {
		query = `SELECT mf.id, mf.status, sender_id, sender_type, receiver_id, 
				receiver_type, workflow_id, message, attempt, mf.created_at, flow, 
				deliver_id, request_size, response_size, 
				sender_name, receiver_name,cmd, w.name as workflow_name,start_input
				FROM message_flows mf 
				JOIN workflows w ON mf.workflow_id = w.id  
				WHERE w.user_id = $1 ORDER BY id DESC OFFSET $2 LIMIT $3  `
		args = []interface{}{userID, offset, limit}
	} else {
		return nil, fmt.Errorf("empty query")
	}

	rows, err := Get().Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var flows []*models.MessageFlow
	for rows.Next() {
		var flow models.MessageFlow
		err := rows.Scan(
			&flow.ID,
			&flow.Status,
			&flow.SenderID,
			&flow.SenderType,
			&flow.ReceiverID,
			&flow.ReceiverType,
			&flow.WorkflowID,
			&flow.Message,
			&flow.Attempt,
			&flow.CreatedAt,
			&flow.Flow,
			&flow.DeliverID,
			&flow.RequestSize,
			&flow.ResponseSize,
			&flow.SenderName,
			&flow.ReceiverName,
			&flow.Cmd,
			&flow.WorkflowName,
			&flow.StartInput,
		)
		if err != nil {
			return nil, err
		}
		flows = append(flows, &flow)
	}

	return flows, nil
}

func InsertWorkflowRecord(record *models.WorkflowRecords) error {
	query := "INSERT INTO workflow_records (workflow_id, record, created_at, status, deliver_err, is_recovered) VALUES ($1, $2, $3, $4, $5, $6) RETURNING id"
	stmt, err := Get().Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	err = stmt.QueryRow(record.WorkflowID, record.Record, record.CreatedAt, record.Status, record.DeliverErr, record.IsRecovered).Scan(&record.ID)
	if err != nil {
		return err
	}
	return nil
}

func GetWorkflowRecords(workflowID int64, limit int) ([]*models.WorkflowRecords, error) {
	query := "SELECT id, workflow_id, record, created_at, status FROM workflow_records WHERE workflow_id = $1 ORDER BY ID DESC LIMIT $2"
	stmt, err := db.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(workflowID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.WorkflowRecords
	for rows.Next() {
		var record models.WorkflowRecords
		err := rows.Scan(&record.ID, &record.WorkflowID, &record.Record, &record.CreatedAt, &record.Status)
		if err != nil {
			return nil, err
		}
		records = append(records, &record)
	}

	return records, nil
}

func GetWorkflowRecordsRecovery(workflowID int64, limit int) ([]*models.WorkflowRecords, error) {
	query := "SELECT id,deliver_err FROM workflow_records WHERE workflow_id = $1 AND is_recovered = $2 AND status = $3 ORDER BY created_at ASC LIMIT $4"
	stmt, err := Get().Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	rows, err := stmt.Query(workflowID, false, 0, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*models.WorkflowRecords
	for rows.Next() {
		var record models.WorkflowRecords
		err := rows.Scan(&record.ID, &record.DeliverErr)
		if err != nil {
			return nil, err
		}
		records = append(records, &record)
	}

	return records, nil
}

func SetWfRecovered(id int64) error {
	query := "UPDATE workflow_records SET is_recovered = $1  WHERE id = $2"
	stmt, err := Get().Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()
	_, err = Get().Exec(query, true, id)
	if err != nil {
		return err
	}
	return nil
}

func InsertTrigger(trigger *models.Trigger) (int64, error) {
	var triggerID int64

	query := `INSERT INTO triggers (workflow_id, schedule, input, status, trigger_at, object_id, type, client) 
			  VALUES ($1, $2, $3, $4, $5,$6, $7, $8) RETURNING id`

	err := Get().QueryRow(query, trigger.WorkflowID, trigger.Schedule,
		trigger.Input, trigger.Status, trigger.TriggerAt,
		trigger.ObjectID, trigger.Type, trigger.Client,
	).Scan(&triggerID)
	if err != nil {
		return 0, err
	}

	return triggerID, nil
}

func DeleteTriggerByID(triggerID string) error {
	query := "DELETE FROM triggers WHERE id = $1"

	_, err := Get().Exec(query, triggerID)
	if err != nil {
		return err
	}

	return nil
}

func GetTriggersByCriteria(workflowID, objectID int64, triggerType string) ([]*models.Trigger, error) {
	var triggers []*models.Trigger

	query := `SELECT * FROM triggers WHERE workflow_id = $1 AND object_id = $2 AND type = $3`

	rows, err := Get().Query(query, workflowID, objectID, triggerType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var trigger models.Trigger
		err := rows.Scan(&trigger.ID, &trigger.WorkflowID, &trigger.ObjectID, &trigger.Type, &trigger.Schedule,
			&trigger.Input, &trigger.Status, &trigger.TriggerAt, &trigger.Client)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, &trigger)
	}

	return triggers, nil
}

func UpdateStatusByID(triggerID int64, newStatus int) error {
	query := "UPDATE triggers SET status = $1 WHERE id = $2"
	_, err := Get().Exec(query, newStatus, triggerID)
	return err
}

type Storage interface {
	UpdateWorkflow(workflow *workflow.WorkflowFile) error
}
