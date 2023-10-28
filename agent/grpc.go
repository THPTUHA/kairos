package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/THPTUHA/kairos/server/plugin"
	"github.com/THPTUHA/kairos/server/plugin/proto"
	metrics "github.com/armon/go-metrics"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
)

var (
	ErrBrokenStream = errors.New("grpc: Error on execution streaming, agent connection was abruptly terminated")
	ErrNotLeader    = errors.New("grpc: Error, server is not leader, this operation should be run on the leader")
)

// KairosGRPCServer defines the basics that a gRPC server should implement.
type KairosGRPCServer interface {
	proto.KairosServer
	Serve(net.Listener) error
}

// GRPCServer is the local implementation of the gRPC server interface.
type GRPCServer struct {
	proto.KairosServer
	agent  *Agent
	logger *logrus.Entry
}

func NewGRPCServer(agent *Agent, logger *logrus.Entry) KairosGRPCServer {
	return &GRPCServer{
		agent:  agent,
		logger: logger,
	}
}

// Serve creates and start a new gRPC dkron server
func (grpcs *GRPCServer) Serve(lis net.Listener) error {
	grpcServer := grpc.NewServer()
	proto.RegisterKairosServer(grpcServer, grpcs)

	as := NewAgentServer(grpcs.agent, grpcs.logger)
	proto.RegisterAgentServer(grpcServer, as)
	go grpcServer.Serve(lis)

	return nil
}

// Encode is used to encode a Protoc object with type prefix
func Encode(t MessageType, msg interface{}) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte(uint8(t))
	m, err := pb.Marshal(msg.(pb.Message))
	if err != nil {
		return nil, err
	}
	_, err = buf.Write(m)
	return buf.Bytes(), err
}

// SetTask broadcast a state change to the cluster members that will store the task.
// Then restart the scheduler
// This only works on the leader
func (grpcs *GRPCServer) SetTask(ctx context.Context, setTaskReq *proto.SetTaskRequest) (*proto.SetTaskResponse, error) {
	grpcs.logger.WithFields(logrus.Fields{
		"task": setTaskReq.Task.Key,
	}).Debug("grpc: Received SetTask")

	if err := grpcs.agent.applySetTask(setTaskReq.Task); err != nil {
		return nil, err
	}

	// If everything is ok, add the task to the scheduler
	task := NewTaskFromProto(setTaskReq.Task, grpcs.logger)
	task.Agent = grpcs.agent
	if err := grpcs.agent.sched.AddTask(task); err != nil {
		return nil, err
	}

	return &proto.SetTaskResponse{}, nil
}

// SetExecution broadcast a state change to the cluster members that will store the execution.
// This only works on the leader
func (grpcs *GRPCServer) SetExecution(ctx context.Context, execution *proto.Execution) (*empty.Empty, error) {
	grpcs.logger.WithFields(logrus.Fields{
		"execution": execution.Key(),
	}).Debug("grpc: Received SetExecution")

	cmd, err := Encode(SetExecutionType, execution)
	if err != nil {
		grpcs.logger.WithError(err).Fatal("agent: encode error in SetExecution")
		return nil, err
	}
	af := grpcs.agent.raft.Apply(cmd, raftTimeout)
	if err := af.Error(); err != nil {
		grpcs.logger.WithError(err).Fatal("agent: error applying SetExecutionType")
		return nil, err
	}

	return new(empty.Empty), nil
}

// DeleteTask broadcast a state change to the cluster members that will delete the task.
// This only works on the leader
func (grpcs *GRPCServer) DeleteTask(ctx context.Context, delTaskReq *proto.DeleteTaskRequest) (*proto.DeleteTaskResponse, error) {
	grpcs.logger.WithField("task", delTaskReq.GetTaskId()).Debug("grpc: Received DeleteTask")

	cmd, err := Encode(DeleteTaskType, delTaskReq)
	if err != nil {
		return nil, err
	}
	af := grpcs.agent.raft.Apply(cmd, raftTimeout)
	if err := af.Error(); err != nil {
		return nil, err
	}
	res := af.Response()
	task, ok := res.(*Task)
	if !ok {
		return nil, fmt.Errorf("grpc: Error wrong response from apply in DeleteTask: %v", res)
	}
	jpb := task.ToProto()

	// If everything is ok, remove the task
	grpcs.agent.sched.RemoveTask(task.Key)
	if task.Ephemeral {
		grpcs.logger.WithField("task", task.Key).Info("grpc: Done deleting ephemeral task")
	}

	return &proto.DeleteTaskResponse{Task: jpb}, nil
}

// ExecutionDone saves the execution to the store
func (grpcs *GRPCServer) ExecutionDone(ctx context.Context, execDoneReq *proto.ExecutionDoneRequest) (*proto.ExecutionDoneResponse, error) {
	defer metrics.MeasureSince([]string{"grpc", "execution_done"}, time.Now())
	grpcs.logger.WithFields(logrus.Fields{
		"group":  execDoneReq.Execution.Group,
		"taskID": execDoneReq.Execution.TaskId,
		"from":   execDoneReq.Execution.NodeName,
	}).Debug("grpc: Received execution done")

	// Get the leader address and compare with the current node address.
	// Forward the request to the leader in case current node is not the leader.
	if !grpcs.agent.IsLeader() {
		addr := grpcs.agent.raft.Leader()
		grpcs.agent.GRPCClient.ExecutionDone(string(addr), NewExecutionFromProto(execDoneReq.Execution))
		return nil, ErrNotLeader
	}

	// This is the leader at this point, so process the execution, encode the value and apply the log to the cluster.
	// Get the defined output types for the task, and call them
	task, err := grpcs.agent.Store.GetTask(execDoneReq.Execution.TaskId, nil)
	if err != nil {
		return nil, err
	}

	pbex := *execDoneReq.Execution
	for k, v := range task.Processors {
		grpcs.logger.WithField("plugin", k).Info("grpc: Processing execution with plugin")
		if processor, ok := grpcs.agent.ProcessorPlugins[k]; ok {
			v["reporting_node"] = grpcs.agent.config.NodeName
			pbex = processor.Process(&plugin.ProcessorArgs{Execution: pbex, Config: v})
		} else {
			grpcs.logger.WithField("plugin", k).Error("grpc: Specified plugin not found")
		}
	}

	execDoneReq.Execution = &pbex
	cmd, err := Encode(ExecutionDoneType, execDoneReq)
	if err != nil {
		return nil, err
	}
	af := grpcs.agent.raft.Apply(cmd, raftTimeout)
	if err := af.Error(); err != nil {
		return nil, err
	}

	// Retrieve the fresh, updated task from the store to work on stored values
	task, err = grpcs.agent.Store.GetTask(task.ID, nil)
	if err != nil {
		grpcs.logger.WithError(err).WithField("task", execDoneReq.Execution.TaskId).Error("grpc: Error retrieving task from store")
		return nil, err
	}

	// If the execution failed, retry it until retries limit (default: don't retry)
	// Don't retry if the status is unknown
	execution := NewExecutionFromProto(&pbex)
	if !execution.Success &&
		uint(execution.Attempt) < task.Retries+1 &&
		!strings.HasPrefix(execution.Output, ErrBrokenStream.Error()) {
		// Increment the attempt counter
		execution.Attempt++

		// Keep all execution properties intact except the last output
		execution.Output = ""

		grpcs.logger.WithFields(logrus.Fields{
			"attempt":   execution.Attempt,
			"execution": execution,
		}).Debug("grpc: Retrying execution")

		if _, err := grpcs.agent.Run(task.ID, execution); err != nil {
			return nil, err
		}
		return &proto.ExecutionDoneResponse{
			From:    grpcs.agent.config.NodeName,
			Payload: []byte("retry"),
		}, nil
	}

	exg, err := grpcs.agent.Store.GetExecutionGroup(execution,
		&ExecutionOptions{
			Timezone: task.GetTimeLocation(),
		},
	)
	if err != nil {
		grpcs.logger.WithError(err).WithField("group", execution.Group).Error("grpc: Error getting execution group.")
		return nil, err
	}

	// Send notification
	if err := SendPostNotifications(grpcs.agent.config, execution, exg, task, grpcs.logger); err != nil {
		return nil, err
	}

	if task.Ephemeral && task.Status == StatusSuccess {
		if _, err := grpcs.DeleteTask(ctx, &proto.DeleteTaskRequest{TaskId: task.ID}); err != nil {
			return nil, err
		}
		return &proto.ExecutionDoneResponse{
			From:    grpcs.agent.config.NodeName,
			Payload: []byte("deleted"),
		}, nil
	}

	return &proto.ExecutionDoneResponse{
		From:    grpcs.agent.config.NodeName,
		Payload: []byte("saved"),
	}, nil
}
