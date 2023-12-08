package models

const (
	DeliverFlow = iota
	RecieverFlow
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
	Attemp       int    `json:"attempt"`
	Flow         int    `json:"flow"`
	CreatedAt    int64  `json:"created_at"`
	DeliverID    int64  `json:"deliver_id"`
	ElapsedTime  int64  `json:"elapsed_time"`
	RequestSize  int    `json:"request_size"`
	ResponseSize int    `json:"response_size"`
	Cmd          int    `json:"cmd"`
	WorkflowName string `json:"workflow_name"`
}
