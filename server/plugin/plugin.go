package plugin

import "github.com/hashicorp/go-plugin"

type InputTask struct {
	DeliverID  int64  `json:"deliver_id"`
	WorkflowID int64  `json:"workflow_id"`
	Input      string `json:"input"`
}

var PluginMap = map[string]plugin.Plugin{
	"executor": &ExecutorPlugin{},
}
