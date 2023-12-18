package plugin

import (
	"bufio"
	"bytes"
	"context"
	"io"

	"github.com/THPTUHA/kairos/server/plugin/internal/plugin"
	empty "github.com/golang/protobuf/ptypes/empty"
	hclog "github.com/hashicorp/go-hclog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const grpcStdioBuffer = 1 * 1024

type grpcStdioServer struct {
	stdoutCh <-chan []byte
	stderrCh <-chan []byte
}

func newGRPCStdioServer(log hclog.Logger, srcOut, srcErr io.Reader) *grpcStdioServer {
	stdoutCh := make(chan []byte)
	stderrCh := make(chan []byte)

	go copyChan(log, stdoutCh, srcOut)
	go copyChan(log, stderrCh, srcErr)

	return &grpcStdioServer{
		stdoutCh: stdoutCh,
		stderrCh: stderrCh,
	}
}

func (s *grpcStdioServer) StreamStdio(
	_ *empty.Empty,
	srv plugin.GRPCStdio_StreamStdioServer,
) error {

	var data plugin.StdioData

	for {
		select {
		case data.Data = <-s.stdoutCh:
			data.Channel = plugin.StdioData_STDOUT

		case data.Data = <-s.stderrCh:
			data.Channel = plugin.StdioData_STDERR

		case <-srv.Context().Done():
			return nil
		}
		if len(data.Data) == 0 {
			continue
		}

		if err := srv.Send(&data); err != nil {
			return err
		}
	}
}

type grpcStdioClient struct {
	log         hclog.Logger
	stdioClient plugin.GRPCStdio_StreamStdioClient
}

func newGRPCStdioClient(
	ctx context.Context,
	log hclog.Logger,
	conn *grpc.ClientConn,
) (*grpcStdioClient, error) {
	client := plugin.NewGRPCStdioClient(conn)

	stdioClient, err := client.StreamStdio(ctx, &empty.Empty{})

	if status.Code(err) == codes.Unavailable || status.Code(err) == codes.Unimplemented {
		log.Warn("stdio service not available, stdout/stderr syncing unavailable")
		stdioClient = nil
		err = nil
	}
	if err != nil {
		return nil, err
	}

	return &grpcStdioClient{
		log:         log,
		stdioClient: stdioClient,
	}, nil
}

func (c *grpcStdioClient) Run(stdout, stderr io.Writer) {
	if c.stdioClient == nil {
		c.log.Warn("stdio service unavailable, run will do nothing")
		return
	}

	for {
		c.log.Trace("waiting for stdio data")
		data, err := c.stdioClient.Recv()
		if err != nil {
			if err == io.EOF ||
				status.Code(err) == codes.Unavailable ||
				status.Code(err) == codes.Canceled ||
				status.Code(err) == codes.Unimplemented ||
				err == context.Canceled {
				c.log.Debug("received EOF, stopping recv loop", "err", err)
				return
			}

			c.log.Error("error receiving data", "err", err)
			return
		}

		var w io.Writer
		switch data.Channel {
		case plugin.StdioData_STDOUT:
			w = stdout

		case plugin.StdioData_STDERR:
			w = stderr

		default:
			c.log.Warn("unknown channel, dropping", "channel", data.Channel)
			continue
		}

		if c.log.IsTrace() {
			c.log.Trace("received data", "channel", data.Channel.String(), "len", len(data.Data))
		}
		if _, err := io.Copy(w, bytes.NewReader(data.Data)); err != nil {
			c.log.Error("failed to copy all bytes", "err", err)
		}
	}
}

func copyChan(log hclog.Logger, dst chan<- []byte, src io.Reader) {
	bufsrc := bufio.NewReader(src)

	for {
		var data [1024]byte
		n, err := bufsrc.Read(data[:])
		if n > 0 {
			dst <- data[:n]
		}

		if err == io.EOF {
			log.Debug("stdio EOF, exiting copy loop")
			return
		}
		if err != nil {
			log.Warn("error copying stdio data, stopping copy", "err", err)
			return
		}
	}
}
