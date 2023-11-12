package models

type MessageFlow struct {
	ID           int64  `json:"id"`
	Status       int    `json:"status"`
	SenderID     int64  `json:"sender_id"`
	SenderType   int    `json:"sender_type"`
	ReceiverID   int64  `json:"receiver_id"`
	ReceiverType int    `json:"receiver_type"`
	WorkflowID   int64  `json:"workflow_id"`
	Message      string `json:"message"`
}
