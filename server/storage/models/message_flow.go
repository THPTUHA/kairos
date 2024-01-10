package models

const (
	DeliverFlow = iota
	RecieverFlow
	LogFlow
	KairosLogFlow
)

type MessageFlow struct {
	ID           int64  `json:"id"`
	Status       int    `json:"status"`
	SenderID     int64  `json:"sender_id"`
	SenderType   int    `json:"sender_type"`
	SenderName   string `json:"sender_name"`
	ReceiverID   int64  `json:"receiver_id"`
	ReceiverType int    `json:"receiver_type"`
	ReceiverName string `json:"receiver_name"`
	WorkflowID   int64  `json:"workflow_id"`
	Message      string `json:"message"`
	Attempt      int    `json:"attempt"`
	Flow         int    `json:"flow"`
	CreatedAt    int64  `json:"created_at"`
	DeliverID    int64  `json:"deliver_id"`
	RequestSize  int    `json:"request_size"`
	ResponseSize int    `json:"response_size"`
	Cmd          int    `json:"cmd"`
	WorkflowName string `json:"workflow_name"`
	Start        bool   `json:"start"`
	StartInput   string `json:"start_input"`
	Group        string `json:"group"`
	TaskID       int64  `json:"task_id"`
	SendAt       int64  `json:"send_at"`
	ReceiveAt    int64  `json:"receive_at"`
	TaskName     string `json:"task_name"`
	Part         string `json:"part"`
	Parent       string `json:"parent"`
	BeginPart    bool   `json:"begin_part"`
	FinishPart   bool   `json:"finish_part"`
	Tracking     string `json:"tracking"`
	BrokerGroup  string `json:"broker_group"`
	TriggerID    int64  `json:"trigger_id"`
}
