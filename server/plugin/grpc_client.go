package plugin

import (
	"context"
	"fmt"
	"math"
	"net"
	"time"

	"github.com/THPTUHA/kairos/server/plugin/internal/plugin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func dialGRPCConn(dialer func(string, time.Duration) (net.Conn, error), dialOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	opts := make([]grpc.DialOption, 0)
	opts = append(opts, grpc.WithDialer(dialer))

	opts = append(opts, grpc.FailOnNonTempDialError(true))

	opts = append(opts, grpc.WithInsecure())

	opts = append(opts,
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt32)),
		grpc.WithDefaultCallOptions(grpc.MaxCallSendMsgSize(math.MaxInt32)))

	opts = append(opts, dialOpts...)

	conn, err := grpc.Dial("unused", opts...)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func newGRPCClient(doneCtx context.Context, c *Client) (*GRPCClient, error) {
	conn, err := dialGRPCConn(c.dialer, c.config.GRPCDialOptions...)
	if err != nil {
		return nil, err
	}

	brokerGRPCClient := newGRPCBrokerClient(conn)
	broker := newGRPCBroker(brokerGRPCClient, c.unixSocketCfg, c.runner)
	go broker.Run()
	go brokerGRPCClient.StartStream()

	stdioClient, err := newGRPCStdioClient(doneCtx, c.logger.Named("stdio"), conn)
	if err != nil {
		return nil, err
	}
	go stdioClient.Run(c.config.SyncStdout, c.config.SyncStderr)

	cl := &GRPCClient{
		Conn:       conn,
		Plugins:    c.config.Plugins,
		doneCtx:    doneCtx,
		broker:     broker,
		controller: plugin.NewGRPCControllerClient(conn),
	}

	return cl, nil
}

type GRPCClient struct {
	Conn    *grpc.ClientConn
	Plugins map[string]Plugin

	doneCtx context.Context
	broker  *GRPCBroker

	controller plugin.GRPCControllerClient
}

func (c *GRPCClient) Close() error {
	c.broker.Close()
	c.controller.Shutdown(c.doneCtx, &plugin.Empty{})
	return c.Conn.Close()
}

func (c *GRPCClient) Dispense(name string) (interface{}, error) {
	raw, ok := c.Plugins[name]
	if !ok {
		return nil, fmt.Errorf("unknown plugin type: %s", name)
	}

	p, ok := raw.(GRPCPlugin)
	if !ok {
		return nil, fmt.Errorf("plugin %q doesn't support gRPC", name)
	}
	return p.GRPCClient(c.doneCtx, c.broker, c.Conn)
}

func (c *GRPCClient) Ping() error {
	client := grpc_health_v1.NewHealthClient(c.Conn)
	_, err := client.Check(context.Background(), &grpc_health_v1.HealthCheckRequest{
		Service: GRPCServiceName,
	})

	return err
}
