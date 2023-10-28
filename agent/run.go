package agent

import (
	"fmt"
	"sync"

	"github.com/hashicorp/serf/serf"
)

// Run call the agents to run a task. Returns a task with its new status and next schedule.
func (a *Agent) Run(taskID int64, ex *Execution) (*Task, error) {
	task, err := a.Store.GetTask(taskID, nil)
	if err != nil {
		return nil, fmt.Errorf("agent: Run error retrieving task: %d from store: %w", taskID, err)
	}

	// In the first execution attempt we build and filter the target nodes
	// but we use the existing node target in case of retry.
	var targetNodes []Node
	if ex.Attempt <= 1 {
		targetNodes = a.getTargetNodes(task.Tags, defaultSelector)
	} else {
		// In case of retrying, find the node or return with an error
		for _, m := range a.serf.Members() {
			if ex.NodeName == m.Name {
				if m.Status == serf.StatusAlive {
					targetNodes = []Node{m}
					break
				} else {
					return nil, fmt.Errorf("retry node is gone: %s for task %d", ex.NodeName, ex.TaskID)
				}
			}
		}
	}

	// In case no nodes found, return reporting the error
	if len(targetNodes) < 1 {
		return nil, fmt.Errorf("no target nodes found to run task %d", ex.TaskID)
	}
	a.logger.WithField("nodes", targetNodes).Debug("agent: Filtered nodes to run")

	var wg sync.WaitGroup
	for _, v := range targetNodes {
		addr, ok := v.Tags["rpc_addr"]
		if !ok {
			addr = v.Addr.String()
		}

		wg.Add(1)
		go func(node string, wg *sync.WaitGroup) {
			defer wg.Done()
			a.logger.WithFields(map[string]interface{}{
				"task_name": task.Key,
				"node":      node,
			}).Info("agent: Calling AgentRun")

			err := a.GRPCClient.AgentRun(node, task.ToProto(), ex.ToProto())
			if err != nil {
				a.logger.WithFields(map[string]interface{}{
					"task_name": task.Key,
					"node":      node,
				}).Error("agent: Error calling AgentRun")
			}
		}(addr, &wg)
	}

	wg.Wait()
	return task, nil
}
