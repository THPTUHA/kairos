package plugin

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/THPTUHA/kairos/server/plugin/internal/plugin"
	"github.com/THPTUHA/kairos/server/plugin/runner"
	"github.com/oklog/run"
	"google.golang.org/grpc"
)

type streamer interface {
	Send(*plugin.ConnInfo) error
	Recv() (*plugin.ConnInfo, error)
	Close()
}

type sendErr struct {
	i  *plugin.ConnInfo
	ch chan error
}

type gRPCBrokerServer struct {
	plugin.UnimplementedGRPCBrokerServer

	send chan *sendErr

	recv chan *plugin.ConnInfo
	quit chan struct{}
	o    sync.Once
}

func newGRPCBrokerServer() *gRPCBrokerServer {
	return &gRPCBrokerServer{
		send: make(chan *sendErr),
		recv: make(chan *plugin.ConnInfo),
		quit: make(chan struct{}),
	}
}

func (s *gRPCBrokerServer) StartStream(stream plugin.GRPCBroker_StartStreamServer) error {
	doneCh := stream.Context().Done()
	defer s.Close()

	go func() {
		for {
			select {
			case <-doneCh:
				return
			case <-s.quit:
				return
			case se := <-s.send:
				err := stream.Send(se.i)
				se.ch <- err
			}
		}
	}()

	for {
		i, err := stream.Recv()
		if err != nil {
			return err
		}
		select {
		case <-doneCh:
			return nil
		case <-s.quit:
			return nil
		case s.recv <- i:
		}
	}
	return nil

}

func (s *gRPCBrokerServer) Send(i *plugin.ConnInfo) error {
	ch := make(chan error)
	defer close(ch)

	select {
	case <-s.quit:
		return errors.New("broker closed")
	case s.send <- &sendErr{
		i:  i,
		ch: ch,
	}:
	}

	return <-ch
}

func (s *gRPCBrokerServer) Recv() (*plugin.ConnInfo, error) {
	select {
	case <-s.quit:
		return nil, errors.New("broker closed")
	case i := <-s.recv:
		return i, nil
	}
}

func (s *gRPCBrokerServer) Close() {
	s.o.Do(func() {
		close(s.quit)
	})
}

type gRPCBrokerClientImpl struct {
	client plugin.GRPCBrokerClient
	send   chan *sendErr
	recv   chan *plugin.ConnInfo
	quit   chan struct{}

	o sync.Once
}

func newGRPCBrokerClient(conn *grpc.ClientConn) *gRPCBrokerClientImpl {
	return &gRPCBrokerClientImpl{
		client: plugin.NewGRPCBrokerClient(conn),
		send:   make(chan *sendErr),
		recv:   make(chan *plugin.ConnInfo),
		quit:   make(chan struct{}),
	}
}

func (s *gRPCBrokerClientImpl) StartStream() error {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	defer s.Close()

	stream, err := s.client.StartStream(ctx)
	if err != nil {
		return err
	}
	doneCh := stream.Context().Done()

	go func() {
		for {
			select {
			case <-doneCh:
				return
			case <-s.quit:
				return
			case se := <-s.send:
				err := stream.Send(se.i)
				se.ch <- err
			}
		}
	}()

	for {
		i, err := stream.Recv()
		if err != nil {
			return err
		}
		select {
		case <-doneCh:
			return nil
		case <-s.quit:
			return nil
		case s.recv <- i:
		}
	}

	return nil
}

func (s *gRPCBrokerClientImpl) Send(i *plugin.ConnInfo) error {
	ch := make(chan error)
	defer close(ch)

	select {
	case <-s.quit:
		return errors.New("broker closed")
	case s.send <- &sendErr{
		i:  i,
		ch: ch,
	}:
	}

	return <-ch
}

func (s *gRPCBrokerClientImpl) Recv() (*plugin.ConnInfo, error) {
	select {
	case <-s.quit:
		return nil, errors.New("broker closed")
	case i := <-s.recv:
		return i, nil
	}
}

func (s *gRPCBrokerClientImpl) Close() {
	s.o.Do(func() {
		close(s.quit)
	})
}

type GRPCBroker struct {
	nextId   uint32
	streamer streamer
	doneCh   chan struct{}
	o        sync.Once

	clientStreams map[uint32]*gRPCBrokerPending
	serverStreams map[uint32]*gRPCBrokerPending

	unixSocketCfg  UnixSocketConfig
	addrTranslator runner.AddrTranslator

	dialMutex sync.Mutex

	sync.Mutex
}

type gRPCBrokerPending struct {
	ch     chan *plugin.ConnInfo
	doneCh chan struct{}
	once   sync.Once
}

func newGRPCBroker(s streamer, unixSocketCfg UnixSocketConfig, addrTranslator runner.AddrTranslator) *GRPCBroker {
	return &GRPCBroker{
		streamer: s,
		doneCh:   make(chan struct{}),

		clientStreams: make(map[uint32]*gRPCBrokerPending),
		serverStreams: make(map[uint32]*gRPCBrokerPending),

		unixSocketCfg:  unixSocketCfg,
		addrTranslator: addrTranslator,
	}
}

func (b *GRPCBroker) Accept(id uint32) (net.Listener, error) {
	listener, err := serverListener(b.unixSocketCfg)
	if err != nil {
		return nil, err
	}

	advertiseNet := listener.Addr().Network()
	advertiseAddr := listener.Addr().String()
	advertiseNet, advertiseAddr, err = b.addrTranslator.HostToPlugin(advertiseNet, advertiseAddr)
	if err != nil {
		return nil, err
	}

	err = b.streamer.Send(&plugin.ConnInfo{
		ServiceId: id,
		Network:   advertiseNet,
		Address:   advertiseAddr,
	})
	if err != nil {
		return nil, err
	}

	return listener, nil
}

func (b *GRPCBroker) AcceptAndServe(id uint32, newGRPCServer func([]grpc.ServerOption) *grpc.Server) {
	ln, err := b.Accept(id)
	if err != nil {
		log.Printf("[ERR] plugin: plugin acceptAndServe error: %s", err)
		return
	}
	defer ln.Close()

	var opts []grpc.ServerOption

	server := newGRPCServer(opts)

	var g run.Group
	{
		g.Add(func() error {
			return server.Serve(ln)
		}, func(err error) {
			server.GracefulStop()
		})
	}
	{
		closeCh := make(chan struct{})
		g.Add(func() error {
			select {
			case <-b.doneCh:
			case <-closeCh:
			}
			return nil
		}, func(err error) {
			close(closeCh)
		})
	}

	g.Run()
}

func (b *GRPCBroker) Close() error {
	b.streamer.Close()
	b.o.Do(func() {
		close(b.doneCh)
	})
	return nil
}

func (b *GRPCBroker) Dial(id uint32) (conn *grpc.ClientConn, err error) {
	var c *plugin.ConnInfo

	p := b.getClientStream(id)
	select {
	case c = <-p.ch:
		close(p.doneCh)
	case <-time.After(5 * time.Second):
		return nil, fmt.Errorf("timeout waiting for connection info")
	}

	network, address := c.Network, c.Address
	if b.addrTranslator != nil {
		network, address, err = b.addrTranslator.PluginToHost(network, address)
		if err != nil {
			return nil, err
		}
	}

	var addr net.Addr
	switch network {
	case "tcp":
		addr, err = net.ResolveTCPAddr("tcp", address)
	case "unix":
		addr, err = net.ResolveUnixAddr("unix", address)
	default:
		err = fmt.Errorf("Unknown address type: %s", c.Address)
	}
	if err != nil {
		return nil, err
	}

	return dialGRPCConn(netAddrDialer(addr))
}

func (m *GRPCBroker) NextId() uint32 {
	return atomic.AddUint32(&m.nextId, 1)
}

func (m *GRPCBroker) Run() {
	for {
		msg, err := m.streamer.Recv()
		if err != nil {
			break
		}
		var p *gRPCBrokerPending

		p = m.getClientStream(msg.ServiceId)
		go m.timeoutWait(msg.ServiceId, p)

		select {
		case p.ch <- msg:
		default:
		}
	}
}

func (m *GRPCBroker) getClientStream(id uint32) *gRPCBrokerPending {
	m.Lock()
	defer m.Unlock()

	p, ok := m.clientStreams[id]
	if ok {
		return p
	}

	m.clientStreams[id] = &gRPCBrokerPending{
		ch:     make(chan *plugin.ConnInfo, 1),
		doneCh: make(chan struct{}),
	}
	return m.clientStreams[id]
}

func (m *GRPCBroker) timeoutWait(id uint32, p *gRPCBrokerPending) {
	select {
	case <-p.doneCh:
	case <-time.After(5 * time.Second):
	}

	m.Lock()
	defer m.Unlock()
	delete(m.clientStreams, id)
}
