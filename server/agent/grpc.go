package agent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/THPTUHA/kairos/server/plugin/proto"
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
