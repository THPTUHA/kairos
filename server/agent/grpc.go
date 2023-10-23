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

// SetJob broadcast a state change to the cluster members that will store the job.
// Then restart the scheduler
// This only works on the leader
func (grpcs *GRPCServer) SetJob(ctx context.Context, setJobReq *proto.SetJobRequest) (*proto.SetJobResponse, error) {
	grpcs.logger.WithFields(logrus.Fields{
		"job": setJobReq.Job.Name,
	}).Debug("grpc: Received SetJob")

	if err := grpcs.agent.applySetJob(setJobReq.Job); err != nil {
		return nil, err
	}

	// If everything is ok, add the job to the scheduler
	job := NewJobFromProto(setJobReq.Job, grpcs.logger)
	job.Agent = grpcs.agent
	if err := grpcs.agent.sched.AddJob(job); err != nil {
		return nil, err
	}

	return &proto.SetJobResponse{}, nil
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

// DeleteJob broadcast a state change to the cluster members that will delete the job.
// This only works on the leader
func (grpcs *GRPCServer) DeleteJob(ctx context.Context, delJobReq *proto.DeleteJobRequest) (*proto.DeleteJobResponse, error) {
	grpcs.logger.WithField("job", delJobReq.GetJobName()).Debug("grpc: Received DeleteJob")

	cmd, err := Encode(DeleteJobType, delJobReq)
	if err != nil {
		return nil, err
	}
	af := grpcs.agent.raft.Apply(cmd, raftTimeout)
	if err := af.Error(); err != nil {
		return nil, err
	}
	res := af.Response()
	job, ok := res.(*Job)
	if !ok {
		return nil, fmt.Errorf("grpc: Error wrong response from apply in DeleteJob: %v", res)
	}
	jpb := job.ToProto()

	// If everything is ok, remove the job
	grpcs.agent.sched.RemoveJob(job.Name)
	if job.Ephemeral {
		grpcs.logger.WithField("job", job.Name).Info("grpc: Done deleting ephemeral job")
	}

	return &proto.DeleteJobResponse{Job: jpb}, nil
}

// ExecutionDone saves the execution to the store
func (grpcs *GRPCServer) ExecutionDone(ctx context.Context, execDoneReq *proto.ExecutionDoneRequest) (*proto.ExecutionDoneResponse, error) {
	defer metrics.MeasureSince([]string{"grpc", "execution_done"}, time.Now())
	grpcs.logger.WithFields(logrus.Fields{
		"group": execDoneReq.Execution.Group,
		"job":   execDoneReq.Execution.JobName,
		"from":  execDoneReq.Execution.NodeName,
	}).Debug("grpc: Received execution done")

	// Get the leader address and compare with the current node address.
	// Forward the request to the leader in case current node is not the leader.
	if !grpcs.agent.IsLeader() {
		addr := grpcs.agent.raft.Leader()
		grpcs.agent.GRPCClient.ExecutionDone(string(addr), NewExecutionFromProto(execDoneReq.Execution))
		return nil, ErrNotLeader
	}

	// This is the leader at this point, so process the execution, encode the value and apply the log to the cluster.
	// Get the defined output types for the job, and call them
	job, err := grpcs.agent.Store.GetJob(execDoneReq.Execution.JobName, nil)
	if err != nil {
		return nil, err
	}

	pbex := *execDoneReq.Execution
	for k, v := range job.Processors {
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

	// Retrieve the fresh, updated job from the store to work on stored values
	job, err = grpcs.agent.Store.GetJob(job.Name, nil)
	if err != nil {
		grpcs.logger.WithError(err).WithField("job", execDoneReq.Execution.JobName).Error("grpc: Error retrieving job from store")
		return nil, err
	}

	// If the execution failed, retry it until retries limit (default: don't retry)
	// Don't retry if the status is unknown
	execution := NewExecutionFromProto(&pbex)
	if !execution.Success &&
		uint(execution.Attempt) < job.Retries+1 &&
		!strings.HasPrefix(execution.Output, ErrBrokenStream.Error()) {
		// Increment the attempt counter
		execution.Attempt++

		// Keep all execution properties intact except the last output
		execution.Output = ""

		grpcs.logger.WithFields(logrus.Fields{
			"attempt":   execution.Attempt,
			"execution": execution,
		}).Debug("grpc: Retrying execution")

		if _, err := grpcs.agent.Run(job.Name, execution); err != nil {
			return nil, err
		}
		return &proto.ExecutionDoneResponse{
			From:    grpcs.agent.config.NodeName,
			Payload: []byte("retry"),
		}, nil
	}

	exg, err := grpcs.agent.Store.GetExecutionGroup(execution,
		&ExecutionOptions{
			Timezone: job.GetTimeLocation(),
		},
	)
	if err != nil {
		grpcs.logger.WithError(err).WithField("group", execution.Group).Error("grpc: Error getting execution group.")
		return nil, err
	}

	// Send notification
	if err := SendPostNotifications(grpcs.agent.config, execution, exg, job, grpcs.logger); err != nil {
		return nil, err
	}

	// Jobs that have dependent jobs are a bit more expensive because we need to call the Status() method for every execution.
	// Check first if there's dependent jobs and then check for the job status to begin execution dependent jobs on success.
	if len(job.DependentJobs) > 0 && job.Status == StatusSuccess {
		for _, djn := range job.DependentJobs {
			dj, err := grpcs.agent.Store.GetJob(djn, nil)
			if err != nil {
				return nil, err
			}
			dj.Agent = grpcs.agent
			grpcs.logger.WithField("job", djn).Debug("grpc: Running dependent job")
			dj.Run()
		}
	}

	if job.Ephemeral && job.Status == StatusSuccess {
		if _, err := grpcs.DeleteJob(ctx, &proto.DeleteJobRequest{JobName: job.Name}); err != nil {
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
