package deliverer

import (
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/THPTUHA/kairos/pkg/protocol/deliverprotocol"
	"github.com/THPTUHA/kairos/server/deliverer/internal/cancelctx"
	"github.com/THPTUHA/kairos/server/deliverer/internal/timers"
	"github.com/gorilla/websocket"
)

type WebsocketConfig struct {
	CheckOrigin        func(r *http.Request) bool
	ReadBufferSize     int
	WriteBufferSize    int
	UseWriteBufferPool bool
	MessageSizeLimit   int
	WriteTimeout       time.Duration
	Compression        bool

	CompressionLevel   int
	CompressionMinSize int

	PingPongConfig
}

type WebsocketHandler struct {
	node    *Node
	upgrade *websocket.Upgrader
	config  WebsocketConfig
}

var writeBufferPool = &sync.Pool{}

func NewWebsocketHandler(node *Node, config WebsocketConfig) *WebsocketHandler {
	upgrade := &websocket.Upgrader{
		ReadBufferSize:    config.ReadBufferSize,
		EnableCompression: config.Compression,
		Subprotocols:      []string{"json"},
	}
	if config.UseWriteBufferPool {
		upgrade.WriteBufferPool = writeBufferPool
	} else {
		upgrade.WriteBufferSize = config.WriteBufferSize
	}
	if config.CheckOrigin != nil {
		upgrade.CheckOrigin = config.CheckOrigin
	}

	return &WebsocketHandler{
		node:    node,
		config:  config,
		upgrade: upgrade,
	}
}

func (s *WebsocketHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request) {

	var protoType = ProtocolTypeJSON
	var useFramePingPong bool
	compression := s.config.Compression
	compressionLevel := s.config.CompressionLevel
	compressionMinSize := s.config.CompressionMinSize

	conn, err := s.upgrade.Upgrade(rw, r, nil)
	if err != nil {
		s.node.logger.log(newLogEntry(LogLevelDebug, "websocket upgrade error", map[string]any{"error": err.Error()}))
		return
	}

	if compression {
		err := conn.SetCompressionLevel(compressionLevel)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "websocket error setting compression level", map[string]any{"error": err.Error()}))
		}
	}

	writeTimeout := s.config.WriteTimeout
	if writeTimeout == 0 {
		writeTimeout = 1 * time.Second
	}
	messageSizeLimit := s.config.MessageSizeLimit
	if messageSizeLimit == 0 {
		messageSizeLimit = 65536 // 64KB
	}
	if messageSizeLimit > 0 {
		conn.SetReadLimit(int64(messageSizeLimit))
	}

	if useFramePingPong {
		pongWait := framePingInterval * 10 / 9
		_ = conn.SetReadDeadline(time.Now().Add(pongWait))
		conn.SetPongHandler(func(string) error {
			_ = conn.SetReadDeadline(time.Now().Add(pongWait))
			return nil
		})
	}

	go func() {
		opts := websocketTransportOptions{
			pingPong:           s.config.PingPongConfig,
			writeTimeout:       writeTimeout,
			compressionMinSize: compressionMinSize,
			protoType:          protoType,
		}

		graceCh := make(chan struct{})
		transport := newWebsocketTransport(conn, opts, graceCh, useFramePingPong)

		select {
		case <-s.node.NotifyShutdown():
			_ = transport.Close(DisconnectShutdown)
			return
		default:
		}

		ctxCh := make(chan struct{})
		defer close(ctxCh)

		c, closeFn, err := NewClient(cancelctx.New(r.Context(), ctxCh), s.node, transport)
		if err != nil {
			s.node.logger.log(newLogEntry(LogLevelError, "error creating client", map[string]any{"transport": transportWebsocket}))
			return
		}
		defer func() { _ = closeFn() }()

		if s.node.LogEnabled(LogLevelDebug) {
			s.node.logger.log(newLogEntry(LogLevelDebug, "client connection established", map[string]any{"client": c.ID(), "transport": transportWebsocket}))
			defer func(started time.Time) {
				s.node.logger.log(newLogEntry(LogLevelDebug, "client connection completed", map[string]any{"client": c.ID(), "transport": transportWebsocket, "duration": time.Since(started)}))
			}(time.Now())
		}

		for {
			_, r, err := conn.NextReader()
			if err != nil {
				break
			}
			proceed := HandleReadFrame(c, r)
			if !proceed {
				break
			}
		}

		if useFramePingPong {
			conn.SetPingHandler(nil)
			conn.SetPongHandler(nil)
		}

		_ = conn.SetReadDeadline(time.Now().Add(closeFrameWait))
		for {
			if _, _, err := conn.NextReader(); err != nil {
				close(graceCh)
				break
			}
		}
	}()
}

// HandleReadFrame is a helper to read Centrifuge commands from frame-based io.Reader and
// process them. Frame-based means that EOF treated as the end of the frame, not the entire
// connection close.
func HandleReadFrame(c *Client, r io.Reader) bool {
	protoType := c.Transport().Protocol().toProto()
	decoder := deliverprotocol.GetStreamCommandDecoder(protoType, r)
	defer deliverprotocol.PutStreamCommandDecoder(protoType, decoder)

	hadCommands := false

	for {
		cmd, cmdProtocolSize, err := decoder.Decode()
		if cmd != nil {
			hadCommands = true
			proceed := c.HandleCommand(cmd, cmdProtocolSize)
			if !proceed {
				return false
			}
		}
		if err != nil {
			if err == io.EOF {
				if !hadCommands {
					c.node.logger.log(newLogEntry(LogLevelInfo, "empty request received", map[string]any{"client": c.ID(), "user": c.UserID()}))
					c.Disconnect(DisconnectBadRequest)
					return false
				}
				break
			} else {
				c.node.logger.log(newLogEntry(LogLevelInfo, "error reading command", map[string]any{"client": c.ID(), "user": c.UserID(), "error": err.Error()}))
				c.Disconnect(DisconnectBadRequest)
				return false
			}
		}
	}
	return true
}

const (
	transportWebsocket = "websocket"
)

type websocketTransport struct {
	mu              sync.RWMutex
	conn            *websocket.Conn
	closed          bool
	closeCh         chan struct{}
	graceCh         chan struct{}
	opts            websocketTransportOptions
	nativePingTimer *time.Timer
}

type websocketTransportOptions struct {
	protoType          ProtocolType
	pingPong           PingPongConfig
	writeTimeout       time.Duration
	compressionMinSize int
}

func newWebsocketTransport(conn *websocket.Conn, opts websocketTransportOptions, graceCh chan struct{}, useNativePingPong bool) *websocketTransport {
	transport := &websocketTransport{
		conn:    conn,
		closeCh: make(chan struct{}),
		graceCh: graceCh,
		opts:    opts,
	}
	if useNativePingPong {
		transport.addPing()
	}
	return transport
}

func (t *websocketTransport) Name() string {
	return transportWebsocket
}

func (t *websocketTransport) Protocol() ProtocolType {
	return t.opts.protoType
}

func (t *websocketTransport) DisabledPushFlags() uint64 {
	return PushFlagDisconnect
}

func (t *websocketTransport) PingPongConfig() PingPongConfig {
	t.mu.RLock()
	useNativePingPong := t.nativePingTimer != nil
	t.mu.RUnlock()
	if useNativePingPong {
		return PingPongConfig{
			PingInterval: -1,
			PongTimeout:  -1,
		}
	}
	return t.opts.pingPong
}

func (t *websocketTransport) writeData(data []byte) error {
	if t.opts.compressionMinSize > 0 {
		t.conn.EnableWriteCompression(len(data) > t.opts.compressionMinSize)
	}
	var messageType = websocket.TextMessage
	if t.opts.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Now().Add(t.opts.writeTimeout))
	}
	err := t.conn.WriteMessage(messageType, data)
	if err != nil {
		return err
	}
	if t.opts.writeTimeout > 0 {
		_ = t.conn.SetWriteDeadline(time.Time{})
	}
	return nil
}

func (t *websocketTransport) Write(message []byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		protoType := t.Protocol().toProto()
		if protoType == deliverprotocol.TypeJSON {
			return t.writeData(message)
		}
		encoder := deliverprotocol.GetDataEncoder(protoType)
		defer deliverprotocol.PutDataEncoder(protoType, encoder)
		_ = encoder.Encode(message)
		return t.writeData(encoder.Finish())
	}
}

func (t *websocketTransport) WriteMany(messages ...[]byte) error {
	select {
	case <-t.closeCh:
		return nil
	default:
		protoType := t.Protocol().toProto()
		encoder := deliverprotocol.GetDataEncoder(protoType)
		defer deliverprotocol.PutDataEncoder(protoType, encoder)
		for i := range messages {
			_ = encoder.Encode(messages[i])
		}
		return t.writeData(encoder.Finish())
	}
}

const closeFrameWait = 5 * time.Second

func (t *websocketTransport) Close(disconnect Disconnect) error {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return nil
	}
	t.closed = true
	close(t.closeCh)
	if t.nativePingTimer != nil {
		t.nativePingTimer.Stop()
	}
	t.mu.Unlock()

	if disconnect.Code != DisconnectConnectionClosed.Code {
		msg := websocket.FormatCloseMessage(int(disconnect.Code), disconnect.Reason)
		err := t.conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(time.Second))
		if err != nil {
			return t.conn.Close()
		}
		select {
		case <-t.graceCh:
		default:
			tm := timers.AcquireTimer(closeFrameWait)
			select {
			case <-t.graceCh:
			case <-tm.C:
			}
			timers.ReleaseTimer(tm)
		}
		return t.conn.Close()
	}
	return t.conn.Close()
}

var framePingInterval = 25 * time.Second

func (t *websocketTransport) ping() {
	select {
	case <-t.closeCh:
		return
	default:
		deadline := time.Now().Add(framePingInterval / 2)
		err := t.conn.WriteControl(websocket.PingMessage, nil, deadline)
		if err != nil {
			_ = t.Close(DisconnectWriteError)
			return
		}
		t.addPing()
	}
}

func (t *websocketTransport) addPing() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.nativePingTimer = time.AfterFunc(framePingInterval, t.ping)
	t.mu.Unlock()
}
