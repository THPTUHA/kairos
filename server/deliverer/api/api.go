package api

import (
	"context"
	"crypto/subtle"

	"github.com/THPTUHA/kairos/server/deliverer/rule"
	"github.com/centrifugal/centrifuge"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// Executor can run API methods.
type Executor struct {
	node             *centrifuge.Node
	ruleContainer    *rule.Container
	protocol         string
	rpcExtension     map[string]RPCHandler
	surveyCaller     SurveyCaller
	useOpenTelemetry bool
}

// SurveyCaller can do surveys.
type SurveyCaller interface {
	Channels(ctx context.Context, cmd *ChannelsRequest) (map[string]*ChannelInfo, error)
}

// NewExecutor ...
func NewExecutor(n *centrifuge.Node, ruleContainer *rule.Container, surveyCaller SurveyCaller, protocol string, useOpentelemetry bool) *Executor {
	e := &Executor{
		node:             n,
		ruleContainer:    ruleContainer,
		protocol:         protocol,
		surveyCaller:     surveyCaller,
		rpcExtension:     make(map[string]RPCHandler),
		useOpenTelemetry: useOpentelemetry,
	}
	return e
}

func authorize(ctx context.Context, key []byte) error {
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if len(md["authorization"]) > 0 && subtle.ConstantTimeCompare([]byte(md["authorization"][0]), key) == 1 {
			return nil
		}
	}
	return status.Error(codes.Unauthenticated, "unauthenticated")
}

// GRPCKeyAuth allows to set simple authentication based on string key from configuration.
// Client should provide per RPC credentials: set authorization key to metadata with value
// `apikey <KEY>`.
func GRPCKeyAuth(key string) grpc.ServerOption {
	authKey := []byte("apikey " + key)
	return grpc.UnaryInterceptor(func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		if err := authorize(ctx, authKey); err != nil {
			return nil, err
		}
		return handler(ctx, req)
	})
}
