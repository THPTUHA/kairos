package plugin

import (
	"context"

	"github.com/THPTUHA/kairos/server/plugin/internal/plugin"
)

type grpcControllerServer struct {
	server *GRPCServer
}

func (s *grpcControllerServer) Shutdown(ctx context.Context, _ *plugin.Empty) (*plugin.Empty, error) {
	resp := &plugin.Empty{}

	s.server.Stop()
	return resp, nil
}
