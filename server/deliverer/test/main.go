package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	_ "net/http/pprof"

	"github.com/THPTUHA/kairos/server/deliverer"
)

var port = flag.Int("port", 8000, "Port to bind app to")

type clientMessage struct {
	Timestamp int64  `json:"timestamp"`
	Input     string `json:"input"`
}

func handleLog(e deliverer.LogEntry) {
	log.Printf("%s: %v", e.Message, e.Fields)
}

func authMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		newCtx := deliverer.SetCredentials(ctx, &deliverer.Credentials{
			UserID:   "42",
			ExpireAt: time.Now().Unix() + 60,
			Info:     []byte(`{"name": "Alexander"}`),
		})
		r = r.WithContext(newCtx)
		h.ServeHTTP(w, r)
	})
}

func waitExitSignal(n *deliverer.Node, s *http.Server) {
	sigCh := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = n.Shutdown(ctx)
		_ = s.Shutdown(ctx)
		done <- true
	}()
	<-done
}

const exampleChannel = "chat:index"

// Check whether channel is allowed for subscribing. In real case permission
// check will probably be more complex than in this example.
func channelSubscribeAllowed(channel string) bool {
	return channel == exampleChannel
}

func main() {
	node, _ := deliverer.New(deliverer.Config{
		LogLevel:   deliverer.LogLevelInfo,
		LogHandler: handleLog,
	})

	node.OnConnecting(func(ctx context.Context, e deliverer.ConnectEvent) (deliverer.ConnectReply, error) {
		cred, _ := deliverer.GetCredentials(ctx)
		return deliverer.ConnectReply{
			Data: []byte(`{}`),
			// Subscribe to a personal server-side channel.
			Subscriptions: map[string]deliverer.SubscribeOptions{
				"#" + cred.UserID: {
					EmitPresence:  true,
					EmitJoinLeave: true,
					PushJoinLeave: true,
				},
			},
		}, nil
	})

	node.OnConnect(func(client *deliverer.Client) {
		transport := client.Transport()
		log.Printf("[user %s] connected via %s with protocol: %s", client.UserID(), transport.Name(), transport.Protocol())

		go func() {
			for {
				select {
				case <-client.Context().Done():
					return
				case <-time.After(5 * time.Second):
					fmt.Println("SEND MESSAGE to client")
					err := client.Send([]byte(`{"time": "` + strconv.FormatInt(time.Now().Unix(), 10) + `"}`))
					if err != nil {
						if err == io.EOF {
							return
						}
						log.Printf("error sending message: %s", err)
					}
				}
			}
		}()

		client.OnRefresh(func(e deliverer.RefreshEvent, cb deliverer.RefreshCallback) {
			log.Printf("[user %s] connection is going to expire, refreshing", client.UserID())

			cb(deliverer.RefreshReply{
				ExpireAt: time.Now().Unix() + 60,
			}, nil)
		})

		client.OnSubscribe(func(e deliverer.SubscribeEvent, cb deliverer.SubscribeCallback) {
			log.Printf("[user %s] subscribes on %s", client.UserID(), e.Channel)

			if !channelSubscribeAllowed(e.Channel) {
				cb(deliverer.SubscribeReply{}, deliverer.ErrorPermissionDenied)
				return
			}

			cb(deliverer.SubscribeReply{
				Options: deliverer.SubscribeOptions{
					EmitPresence:  true,
					EmitJoinLeave: true,
					PushJoinLeave: true,
					Data:          []byte(`{"msg": "welcome"}`),
				},
			}, nil)
		})

		client.OnPublish(func(e deliverer.PublishEvent, cb deliverer.PublishCallback) {
			log.Printf("[user %s] publishes into channel %s: %s", client.UserID(), e.Channel, string(e.Data))

			if !client.IsSubscribed(e.Channel) {
				cb(deliverer.PublishReply{}, deliverer.ErrorPermissionDenied)
				return
			}

			var msg clientMessage
			err := json.Unmarshal(e.Data, &msg)
			if err != nil {
				cb(deliverer.PublishReply{}, deliverer.ErrorBadRequest)
				return
			}
			msg.Timestamp = time.Now().Unix()
			// data, _ := json.Marshal(msg)

			// result, err := node.Publish(
			// 	e.Channel, data,
			// 	deliverer.WithHistory(300, time.Minute),
			// 	deliverer.WithClientInfo(e.ClientInfo),
			// )

			cb(deliverer.PublishReply{}, err)
		})

		client.OnPresence(func(e deliverer.PresenceEvent, cb deliverer.PresenceCallback) {
			log.Printf("[user %s] calls presence on %s", client.UserID(), e.Channel)

			if !client.IsSubscribed(e.Channel) {
				cb(deliverer.PresenceReply{}, deliverer.ErrorPermissionDenied)
				return
			}
			cb(deliverer.PresenceReply{}, nil)
		})

		client.OnUnsubscribe(func(e deliverer.UnsubscribeEvent) {
			log.Printf("[user %s] unsubscribed from %s: %s", client.UserID(), e.Channel, e.Reason)
		})

		client.OnAlive(func() {
			log.Printf("[user %s] connection is still active", client.UserID())
		})

		client.OnDisconnect(func(e deliverer.DisconnectEvent) {
			log.Printf("[user %s] disconnected: %s", client.UserID(), e.Reason)
		})
	})

	if err := node.Run(); err != nil {
		log.Fatal(err)
	}

	go func() {
		// Publish personal notifications for user 42 periodically.
		i := 1
		for {
			_, err := node.Publish(
				"#42",
				[]byte(`{"personal": "`+strconv.Itoa(i)+`"}`),
			)
			if err != nil {
				log.Printf("error publishing to personal channel: %s", err)
			}
			i++
			time.Sleep(5000 * time.Millisecond)
		}
	}()

	go func() {
		// Publish to channel periodically.
		i := 1
		for {
			_, err := node.Publish(
				"chat:index",
				[]byte(`{"input": "Publish from server `+strconv.Itoa(i)+`"}`),
			)
			if err != nil {
				log.Printf("error publishing to channel: %s", err)
			}
			i++
			time.Sleep(1000 * time.Millisecond)
		}
	}()

	mux := http.DefaultServeMux

	websocketHandler := deliverer.NewWebsocketHandler(node, deliverer.WebsocketConfig{
		ReadBufferSize:     1024,
		UseWriteBufferPool: true,
		CheckOrigin: func(r *http.Request) bool {
			origin := r.Header.Get("Origin")
			return strings.Contains(origin, "localhost")
		},
	})

	mux.Handle("/connection/websocket", authMiddleware(websocketHandler))

	server := &http.Server{
		Handler:      mux,
		Addr:         ":" + strconv.Itoa(*port),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal(err)
		}
	}()

	waitExitSignal(node, server)
	log.Println("bye!")
}
