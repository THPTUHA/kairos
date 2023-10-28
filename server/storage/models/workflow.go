package models

type Workflow struct {
	ID           int64  `json:"id"`
	Key          string `json:"key"`
	Status       int    `json:"status"`
	CollectionID int64  `json:"collection_id"`
	CreatedAt    int    `json:"created_at"`
	UpdatedAt    int    `json:"updated_at"`
}
