package agent

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/THPTUHA/kairos/server/plugin/proto"
	metrics "github.com/armon/go-metrics"
	"github.com/golang/protobuf/ptypes/empty"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// KiarosGRPCClient defines the interface that any gRPC client for
// kairos should implement.
type KiarosGRPCClient interface {
	GetActiveExecutions(string) ([]*proto.Execution, error)
	AgentRun(addr string, job *proto.Job, execution *proto.Execution) error
}

// GRPCClient is the local implementation of the KiarosGRPCClient interface.
type GRPCClient struct {
	dialOpt []grpc.DialOption
	agent   *Agent
	logger  *logrus.Entry
}

// NewGRPCClient returns a new instance of the gRPC client.
func NewGRPCClient(dialOpt grpc.DialOption, agent *Agent, logger *logrus.Entry) KiarosGRPCClient {
	if dialOpt == nil {
		dialOpt = grpc.WithInsecure()
	}
	return &GRPCClient{
		dialOpt: []grpc.DialOption{
			dialOpt,
			grpc.WithBlock(),
		},
		agent:  agent,
		logger: logger,
	}
}

func (grpcc *GRPCClient) SetExecution(execution *proto.Execution) error {
	var conn *grpc.ClientConn

	addr := grpcc.agent.raft.Leader()

	// Initiate a connection with the server
	conn, err := grpcc.Connect(string(addr))
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "SetExecution",
			"server_addr": addr,
		}).Error("grpc: error dialing.")
		return err
	}
	defer conn.Close()

	// Synchronous call
	d := proto.NewKairosClient(conn)
	_, err = d.SetExecution(context.Background(), execution)
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "SetExecution",
			"server_addr": addr,
		}).Error("grpc: Error calling gRPC method")
		return err
	}
	return nil
}

// Connect dialing to a gRPC server
func (grpcc *GRPCClient) Connect(addr string) (*grpc.ClientConn, error) {
	// Initiate a connection with the server
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	conn, err := grpc.DialContext(ctx, addr, grpcc.dialOpt...)
	if err != nil {
		return nil, err
	}
	return conn, nil
}

// GetActiveExecutions returns the active executions of a server node
func (grpcc *GRPCClient) GetActiveExecutions(addr string) ([]*proto.Execution, error) {
	var conn *grpc.ClientConn

	// Initiate a connection with the server
	conn, err := grpcc.Connect(addr)
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "GetActiveExecutions",
			"server_addr": addr,
		}).Error("grpc: error dialing.")
		return nil, err
	}
	defer conn.Close()

	// Synchronous call
	d := proto.NewKairosClient(conn)
	gaer, err := d.GetActiveExecutions(context.Background(), &empty.Empty{})
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "GetActiveExecutions",
			"server_addr": addr,
		}).Error("grpc: Error calling gRPC method")
		return nil, err
	}

	return gaer.Executions, nil
}

// ExecutionDone calls the ExecutionDone gRPC method
func (grpcc *GRPCClient) ExecutionDone(addr string, execution *Execution) error {
	defer metrics.MeasureSince([]string{"grpc", "call_execution_done"}, time.Now())
	var conn *grpc.ClientConn

	conn, err := grpcc.Connect(addr)
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "ExecutionDone",
			"server_addr": addr,
		}).Error("grpc: error dialing.")
		return err
	}
	defer conn.Close()

	d := proto.NewKairosClient(conn)
	edr, err := d.ExecutionDone(context.Background(), &proto.ExecutionDoneRequest{Execution: execution.ToProto()})
	if err != nil {
		if err.Error() == fmt.Sprintf("rpc error: code = Unknown desc = %s", ErrNotLeader.Error()) {
			grpcc.logger.Info("grpc: ExecutionDone forwarded to the leader")
			return nil
		}

		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "ExecutionDone",
			"server_addr": addr,
		}).Error("grpc: Error calling gRPC method")
		return err
	}
	grpcc.logger.WithFields(logrus.Fields{
		"method":      "ExecutionDone",
		"server_addr": addr,
		"from":        edr.From,
		"payload":     string(edr.Payload),
	}).Debug("grpc: Response from method")
	return nil
}

// AgentRun runs a job in the given agent
func (grpcc *GRPCClient) AgentRun(addr string, job *proto.Job, execution *proto.Execution) error {
	defer metrics.MeasureSince([]string{"grpc_client", "agent_run"}, time.Now())
	var conn *grpc.ClientConn

	// Initiate a connection with the server
	conn, err := grpcc.Connect(string(addr))
	if err != nil {
		grpcc.logger.WithError(err).WithFields(logrus.Fields{
			"method":      "AgentRun",
			"server_addr": addr,
		}).Error("grpc: error dialing.")
		return err
	}
	defer conn.Close()

	// Streaming call
	a := proto.NewAgentClient(conn)
	stream, err := a.AgentRun(context.Background(), &proto.AgentRunRequest{
		Job:       job,
		Execution: execution,
	})
	if err != nil {
		return err
	}

	var first bool
	for {
		ars, err := stream.Recv()

		// Stream ends
		if err == io.EOF {
			addr := grpcc.agent.raft.Leader()
			if err := grpcc.ExecutionDone(string(addr), NewExecutionFromProto(execution)); err != nil {
				return err
			}
			return nil
		}

		// Error received from the stream
		if err != nil {
			// At this point the execution status will be unknown, set the FinishedAt time and an explanatory message
			execution.FinishedAt = timestamppb.Now()
			execution.Output = []byte(ErrBrokenStream.Error() + ": " + err.Error())

			grpcc.logger.WithError(err).Error(ErrBrokenStream)

			addr := grpcc.agent.raft.Leader()
			if err := grpcc.ExecutionDone(string(addr), NewExecutionFromProto(execution)); err != nil {
				return err
			}
			return err
		}

		// Registers an active stream
		grpcc.agent.activeExecutions.Store(ars.Execution.Key(), ars.Execution)
		grpcc.logger.WithField("key", ars.Execution.Key()).Debug("grpc: received execution stream")

		execution = ars.Execution
		defer grpcc.agent.activeExecutions.Delete(execution.Key())

		// Store the received execution in the raft log and store
		if !first {
			if err := grpcc.SetExecution(ars.Execution); err != nil {
				return err
			}
			first = true
		}

	}
}
