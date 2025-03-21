package deliverer

import (
	"sync"
	"time"

	"github.com/THPTUHA/kairos/server/deliverer/internal/queue"
	"github.com/THPTUHA/kairos/server/deliverer/internal/timers"
	"github.com/rs/zerolog/log"
)

type writerConfig struct {
	WriteManyFn  func(...queue.Item) error
	WriteFn      func(item queue.Item) error
	MaxQueueSize int
}

type writer struct {
	mu       sync.Mutex
	config   writerConfig
	messages *queue.Queue
	closed   bool
	closeCh  chan struct{}
}

const (
	defaultMaxMessagesInFrame = 16
)

func newWriter(config writerConfig, queueInitialCap int) *writer {
	if queueInitialCap == 0 {
		queueInitialCap = 2
	}
	w := &writer{
		config:   config,
		messages: queue.New(queueInitialCap),
		closeCh:  make(chan struct{}),
	}
	return w
}

func (w *writer) run(writeDelay time.Duration, maxMessagesInFrame int) {
	if maxMessagesInFrame == 0 {
		maxMessagesInFrame = defaultMaxMessagesInFrame
	}
	for {
		if ok := w.waitSendMessage(maxMessagesInFrame, writeDelay); !ok {
			return
		}
	}
}

func (w *writer) enqueue(item queue.Item) *Disconnect {
	ok := w.messages.Add(item)
	if !ok {
		return &DisconnectConnectionClosed
	}
	if w.config.MaxQueueSize > 0 && w.messages.Size() > w.config.MaxQueueSize {
		return &DisconnectSlow
	}
	return nil
}

func (w *writer) waitSendMessage(maxMessagesInFrame int, writeDelay time.Duration) bool {
	ok := w.messages.Wait()
	log.Info().Msg("wait send message")
	if !ok {
		return false
	}
	log.Info().Msg("wait send message successful")
	if writeDelay > 0 {
		tm := timers.AcquireTimer(writeDelay)
		if writeDelay > 0 {
			select {
			case <-tm.C:
			case <-w.closeCh:
				timers.ReleaseTimer(tm)
				return false
			}
		}
		timers.ReleaseTimer(tm)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	msg, ok := w.messages.Remove()
	if !ok {
		return !w.messages.Closed()
	}

	var writeErr error

	messageCount := w.messages.Len()
	if (maxMessagesInFrame == -1 || maxMessagesInFrame > 1) && messageCount > 0 {
		messagesCap := messageCount + 1
		if messagesCap > maxMessagesInFrame && maxMessagesInFrame > -1 {
			messagesCap = maxMessagesInFrame
		}

		messages := make([]queue.Item, 0, messagesCap)
		messages = append(messages, msg)

		for messageCount > 0 {
			messageCount--
			if maxMessagesInFrame > -1 && len(messages) >= maxMessagesInFrame {
				break
			}
			m, ok := w.messages.Remove()
			if ok {
				messages = append(messages, m)
			} else {
				if w.messages.Closed() {
					return false
				}
				break
			}
		}
		if len(messages) == 1 {
			writeErr = w.config.WriteFn(messages[0])
		} else {
			writeErr = w.config.WriteManyFn(messages...)
		}
	} else {
		writeErr = w.config.WriteFn(msg)
	}
	if writeErr != nil {
		return false
	}
	return true
}

func (w *writer) close(flushRemaining bool) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return nil
	}
	w.closed = true

	if flushRemaining {
		remaining := w.messages.CloseRemaining()
		if len(remaining) > 0 {
			_ = w.config.WriteManyFn(remaining...)
		}
	} else {
		w.messages.Close()
	}
	close(w.closeCh)
	return nil
}
