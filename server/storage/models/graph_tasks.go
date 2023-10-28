package models

type GraphTask struct {
	ID           int64 `json:"id"`
	TaskParentID int64 `json:"task_parent_id"`
	TaskChildID  int64 `json:"task_child_id"`
	WorkflowID   int64 `json:"workflow_id"`
}
