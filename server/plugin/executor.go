package plugin

import (
	"context"

	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

type StatusHelper interface {
	Update([]byte, bool) (int64, error)
	Input() []byte
}

type Executor interface {
	Execute(args *proto.ExecuteRequest, cb StatusHelper) (*proto.ExecuteResponse, error)
}

type ExecutorPluginConfig map[string]string

type ExecutorServer struct {
	proto.ExecutorServer
	Impl   Executor
	broker *plugin.GRPCBroker
}

func (m ExecutorServer) Execute(ctx context.Context, req *proto.ExecuteRequest) (*proto.ExecuteResponse, error) {
	conn, err := m.broker.Dial(req.StatusServer)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	a := &GRPCStatusHelperClient{proto.NewStatusHelperClient(conn)}
	return m.Impl.Execute(req, a)
}

type GRPCStatusHelperClient struct{ client proto.StatusHelperClient }

func (m *GRPCStatusHelperClient) Update(b []byte, c bool) (int64, error) {
	resp, err := m.client.Update(context.Background(), &proto.StatusUpdateRequest{
		Output: b,
		Error:  c,
	})
	if err != nil {
		return 0, err
	}
	return resp.R, err
}

func (m *GRPCStatusHelperClient) Input() []byte {
	resp, err := m.client.Input(context.Background(), &proto.StatusInputRequest{})
	if err != nil {
		return nil
	}
	return resp.Input
}

type ExecutorPlugin struct {
	plugin.NetRPCUnsupportedPlugin
	Executor Executor
}

func (p *ExecutorPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	proto.RegisterExecutorServer(s, ExecutorServer{Impl: p.Executor, broker: broker})
	return nil
}

func (p *ExecutorPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return &ExecutorClient{client: proto.NewExecutorClient(c), broker: broker}, nil
}

type GRPCStatusHelperServer struct {
	proto.StatusHelperServer
	Impl StatusHelper
}

func (m *GRPCStatusHelperServer) Update(ctx context.Context, req *proto.StatusUpdateRequest) (resp *proto.StatusUpdateResponse, err error) {
	r, err := m.Impl.Update(req.Output, req.Error)
	if err != nil {
		return nil, err
	}
	return &proto.StatusUpdateResponse{R: r}, err
}

func (m *GRPCStatusHelperServer) Input(ctx context.Context, req *proto.StatusInputRequest) (resp *proto.StatusInputResponse, err error) {
	r := m.Impl.Input()
	if err != nil {
		return nil, err
	}
	return &proto.StatusInputResponse{Input: r}, err
}

type Broker interface {
	NextId() uint32
	AcceptAndServe(id uint32, s func([]grpc.ServerOption) *grpc.Server)
}

type ExecutorClient struct {
	client proto.ExecutorClient
	broker Broker
}

func (m *ExecutorClient) Execute(args *proto.ExecuteRequest, cb StatusHelper) (*proto.ExecuteResponse, error) {
	statusHelperServer := &GRPCStatusHelperServer{Impl: cb}

	initChan := make(chan bool, 1)
	var s *grpc.Server
	serverFunc := func(opts []grpc.ServerOption) *grpc.Server {
		s = grpc.NewServer(opts...)
		proto.RegisterStatusHelperServer(s, statusHelperServer)
		initChan <- true

		return s
	}

	brokerID := m.broker.NextId()
	go func() {
		m.broker.AcceptAndServe(brokerID, serverFunc)
		initChan <- true
	}()

	<-initChan

	args.StatusServer = brokerID
	r, err := m.client.Execute(context.Background(), args)

	if s != nil {
		s.Stop()
	}
	return r, err
}
