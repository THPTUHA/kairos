package agent

import (
	"bytes"
	"net"

	"github.com/THPTUHA/kairos/server/plugin/proto"
	"github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	pb "google.golang.org/protobuf/proto"
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
