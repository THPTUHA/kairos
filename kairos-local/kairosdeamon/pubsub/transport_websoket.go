package pubsub

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
	"github.com/gorilla/websocket"
)

type websocketConfig struct {
	NetDialContext    func(ctx context.Context, network, addr string) (net.Conn, error)
	HandshakeTimeout  time.Duration
	EnableCompression bool
	Header            http.Header
}

type websocketTransport struct {
	mu             sync.Mutex
	conn           *websocket.Conn
	protocolType   deliverprotocol.Type
	commandEncoder deliverprotocol.CommandEncoder
	replyCh        chan *deliverprotocol.Reply
	config         websocketConfig
	disconnect     *disconnect
	closed         bool
	closeCh        chan struct{}
}

func newWebsocketTransport(url string, protocolType deliverprotocol.Type, config websocketConfig) (transport, error) {
	wsHeaders := config.Header

	dialer := &websocket.Dialer{}
	dialer.Proxy = http.ProxyFromEnvironment
	dialer.NetDialContext = config.NetDialContext

	dialer.HandshakeTimeout = config.HandshakeTimeout
	dialer.EnableCompression = config.EnableCompression

	conn, resp, err := dialer.Dial(url, wsHeaders)
	if err != nil {
		return nil, fmt.Errorf("error dial: %v", err)
	}
	if resp.StatusCode != http.StatusSwitchingProtocols {
		return nil, fmt.Errorf("wrong status code while connecting to server: %d", resp.StatusCode)
	}

	t := &websocketTransport{
		conn:           conn,
		replyCh:        make(chan *deliverprotocol.Reply),
		config:         config,
		closeCh:        make(chan struct{}),
		commandEncoder: newCommandEncoder(protocolType),
		protocolType:   protocolType,
	}
	go t.reader()
	return t, nil
}

func extractDisconnectWebsocket(err error) *disconnect {
	if err != nil {
		if closeErr, ok := err.(*websocket.CloseError); ok {
			var d disconnect
			err := json.Unmarshal([]byte(closeErr.Text), &d)
			if err == nil {
				return &d
			} else {
				code := uint32(closeErr.Code)
				reason := closeErr.Text
				reconnect := code < 3500 || code >= 5000 || (code >= 4000 && code < 4500)
				if code < 3000 {
					switch code {
					case websocket.CloseMessageTooBig:
						code = disconnectMessageSizeLimit
					default:
						code = connectingTransportClosed
					}
				}
				return &disconnect{
					Code:      code,
					Reason:    reason,
					Reconnect: reconnect,
				}
			}
		}
	}
	return nil
}

func (t *websocketTransport) reader() {
	defer func() { _ = t.Close() }()
	defer close(t.replyCh)

	for {
		_, data, err := t.conn.ReadMessage()
		if err != nil {
			fmt.Printf("[WEBSOCKER ERR] %s \n", err)
			disconnect := extractDisconnectWebsocket(err)
			t.disconnect = disconnect
			return
		}
	loop:
		for {
			decoder := newReplyDecoder(t.protocolType, data)
			for {
				reply, err := decoder.Decode()
				if err != nil {
					if err == io.EOF {
						break loop
					}
					t.disconnect = &disconnect{Code: disconnectBadProtocol, Reason: "decode error", Reconnect: false}
					return
				}
				fmt.Printf("[RELY RECIVER FROM WEBSOCKET] %s\n", reply.String())
				select {
				case <-t.closeCh:
					fmt.Println("Close websocket")
					return
				case t.replyCh <- reply:
				}
			}
		}
	}
}

func (t *websocketTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	close(t.closeCh)
	_ = t.conn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Now().Add(time.Second))
	return t.conn.Close()
}

func (t *websocketTransport) Read() (*deliverprotocol.Reply, *disconnect, error) {
	reply, ok := <-t.replyCh
	fmt.Printf("[RECIVER REPLY FROM REPLYCH] %s\n", reply.String())
	if !ok {
		return nil, t.disconnect, io.EOF
	}
	return reply, nil, nil
}

func (t *websocketTransport) Write(cmd *deliverprotocol.Command, timeout time.Duration) error {
	data, err := t.commandEncoder.Encode(cmd)
	if err != nil {
		return err
	}
	return t.writeData(data, timeout)
}

func (t *websocketTransport) writeData(data []byte, timeout time.Duration) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if timeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(timeout))
	}
	var err error
	err = t.conn.WriteMessage(websocket.TextMessage, data)
	if timeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Time{})
	}
	return err
}
