package client

import (
	"context"

	"github.com/THPTUHA/kairos/server/deliverer/jwtverify"
	"github.com/THPTUHA/kairos/server/deliverer/proxy"
	"github.com/THPTUHA/kairos/server/deliverer/rule"
	"github.com/centrifugal/centrifuge"
)

// ProxyMap is a structure which contains all configured and already initialized
// proxies which can be used from inside client event handlers.
type ProxyMap struct {
	ConnectProxy           proxy.ConnectProxy
	RefreshProxy           proxy.RefreshProxy
	RpcProxies             map[string]proxy.RPCProxy
	PublishProxies         map[string]proxy.PublishProxy
	SubscribeProxies       map[string]proxy.SubscribeProxy
	SubRefreshProxies      map[string]proxy.SubRefreshProxy
	SubscribeStreamProxies map[string]*proxy.SubscribeStreamProxy
}

// Handler ...
type Handler struct {
	node              *centrifuge.Node
	ruleContainer     *rule.Container
	tokenVerifier     *jwtverify.VerifierJWT
	subTokenVerifier  *jwtverify.VerifierJWT
	proxyMap          *ProxyMap
	rpcExtension      map[string]RPCExtensionFunc
	granularProxyMode bool
}

// NewHandler ...
func NewHandler(
	node *centrifuge.Node,
	ruleContainer *rule.Container,
	tokenVerifier *jwtverify.VerifierJWT,
	subTokenVerifier *jwtverify.VerifierJWT,
	proxyMap *ProxyMap,
	granularProxyMode bool,
) *Handler {
	return &Handler{
		node:              node,
		ruleContainer:     ruleContainer,
		tokenVerifier:     tokenVerifier,
		subTokenVerifier:  subTokenVerifier,
		proxyMap:          proxyMap,
		granularProxyMode: granularProxyMode,
		rpcExtension:      make(map[string]RPCExtensionFunc),
	}
}

// RPCExtensionFunc ...
type RPCExtensionFunc func(c Client, e centrifuge.RPCEvent) (centrifuge.RPCReply, error)

// Setup event handlers.
func (h *Handler) Setup() error {
	var connectProxyHandler proxy.ConnectingHandlerFunc
	if h.proxyMap.ConnectProxy != nil {
		connectProxyHandler = proxy.NewConnectHandler(proxy.ConnectHandlerConfig{
			Proxy: h.proxyMap.ConnectProxy,
		}, h.ruleContainer).Handle(h.node)
	}

	var refreshProxyHandler proxy.RefreshHandlerFunc
	if h.proxyMap.RefreshProxy != nil {
		refreshProxyHandler = proxy.NewRefreshHandler(proxy.RefreshHandlerConfig{
			Proxy: h.proxyMap.RefreshProxy,
		}).Handle(h.node)
	}

	h.node.OnConnecting(func(ctx context.Context, e centrifuge.ConnectEvent) (centrifuge.ConnectReply, error) {
		reply, err := h.OnClientConnecting(ctx, e, connectProxyHandler, refreshProxyHandler != nil)
		if err != nil {
			return centrifuge.ConnectReply{}, err
		}
		return reply, err
	})

	var rpcProxyHandler proxy.RPCHandlerFunc
	if len(h.proxyMap.RpcProxies) > 0 {
		rpcProxyHandler = proxy.NewRPCHandler(proxy.RPCHandlerConfig{
			Proxies:           h.proxyMap.RpcProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var publishProxyHandler proxy.PublishHandlerFunc
	if len(h.proxyMap.PublishProxies) > 0 {
		publishProxyHandler = proxy.NewPublishHandler(proxy.PublishHandlerConfig{
			Proxies:           h.proxyMap.PublishProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var subscribeProxyHandler proxy.SubscribeHandlerFunc
	if len(h.proxyMap.SubscribeProxies) > 0 {
		subscribeProxyHandler = proxy.NewSubscribeHandler(proxy.SubscribeHandlerConfig{
			Proxies:           h.proxyMap.SubscribeProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var proxySubscribeStreamHandler proxy.SubscribeStreamHandlerFunc
	if len(h.proxyMap.SubscribeStreamProxies) > 0 {
		proxySubscribeStreamHandler = proxy.NewSubscribeStreamHandler(proxy.SubscribeStreamHandlerConfig{
			Proxies:           h.proxyMap.SubscribeStreamProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	var subRefreshProxyHandler proxy.SubRefreshHandlerFunc
	if len(h.proxyMap.SubRefreshProxies) > 0 {
		subRefreshProxyHandler = proxy.NewSubRefreshHandler(proxy.SubRefreshHandlerConfig{
			Proxies:           h.proxyMap.SubRefreshProxies,
			GranularProxyMode: h.granularProxyMode,
		}).Handle(h.node)
	}

	ruleConfig := h.ruleContainer.Config()
	usePersonalChannel := ruleConfig.UserSubscribeToPersonal
	singleConnection := ruleConfig.UserPersonalSingleConnection
	concurrency := ruleConfig.ClientConcurrency

	h.node.OnConnect(func(client *centrifuge.Client) {
		userID := client.UserID()
		if usePersonalChannel && singleConnection && userID != "" {
			personalChannel := h.ruleContainer.PersonalChannel(userID)
			presenceStats, err := h.node.PresenceStats(personalChannel)
			if err != nil {
				h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error calling presence stats", map[string]any{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
				client.Disconnect(centrifuge.DisconnectServerError)
				return
			}
			if presenceStats.NumClients >= 2 {
				err = h.node.Disconnect(
					client.UserID(),
					centrifuge.WithCustomDisconnect(centrifuge.DisconnectConnectionLimit),
					centrifuge.WithDisconnectClientWhitelist([]string{client.ID()}),
				)
				if err != nil {
					h.node.Log(centrifuge.NewLogEntry(centrifuge.LogLevelError, "error sending disconnect", map[string]any{"error": err.Error(), "client": client.ID(), "user": client.UserID()}))
					client.Disconnect(centrifuge.DisconnectServerError)
					return
				}
			}
		}

		var semaphore chan struct{}
		if concurrency > 1 {
			semaphore = make(chan struct{}, concurrency)
		}

		client.OnRefresh(func(event centrifuge.RefreshEvent, cb centrifuge.RefreshCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, extra, err := h.OnRefresh(client, event, refreshProxyHandler)
				if err != nil {
					cb(reply, err)
					return
				}
				if extra.CheckSubs && tokenChannelsChanged(client, extra.Subs) {
					// Check whether server-side subscriptions changed. If yes – disconnect
					// the client to make its token-based server-side subscriptions actual.
					// Theoretically we could avoid disconnection here using Subscribe/Unsubscribe
					// methods, but disconnection seems the good first step for the scenario which
					// should be pretty rare given the stable nature of server-side subscriptions.
					cb(reply, centrifuge.DisconnectInsufficientState)
					return
				}
				cb(reply, nil)
			})
		})

		if rpcProxyHandler != nil || len(h.rpcExtension) > 0 {
			client.OnRPC(func(event centrifuge.RPCEvent, cb centrifuge.RPCCallback) {
				h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
					reply, err := h.OnRPC(client, event, rpcProxyHandler)
					cb(reply, err)
				})
			})
		}

		client.OnSubscribe(func(event centrifuge.SubscribeEvent, cb centrifuge.SubscribeCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, _, err := h.OnSubscribe(client, event, subscribeProxyHandler, proxySubscribeStreamHandler)
				cb(reply, err)
			})
		})

		client.OnSubRefresh(func(event centrifuge.SubRefreshEvent, cb centrifuge.SubRefreshCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, _, err := h.OnSubRefresh(client, subRefreshProxyHandler, event)
				cb(reply, err)
			})
		})

		client.OnPublish(func(event centrifuge.PublishEvent, cb centrifuge.PublishCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPublish(client, event, publishProxyHandler)
				cb(reply, err)
			})
		})

		client.OnPresence(func(event centrifuge.PresenceEvent, cb centrifuge.PresenceCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPresence(client, event)
				cb(reply, err)
			})
		})

		client.OnPresenceStats(func(event centrifuge.PresenceStatsEvent, cb centrifuge.PresenceStatsCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnPresenceStats(client, event)
				cb(reply, err)
			})
		})

		client.OnHistory(func(event centrifuge.HistoryEvent, cb centrifuge.HistoryCallback) {
			h.runConcurrentlyIfNeeded(client.Context(), concurrency, semaphore, func() {
				reply, err := h.OnHistory(client, event)
				cb(reply, err)
			})
		})

		client.OnUnsubscribe(func(e centrifuge.UnsubscribeEvent) {
			if len(h.proxyMap.SubscribeStreamProxies) > 0 {
				storage, release := client.AcquireStorage()
				streamCancelKey := "stream_cancel_" + e.Channel
				cancelFunc, ok := storage[streamCancelKey].(func())
				if ok {
					cancelFunc()
					delete(storage, streamCancelKey)
					delete(storage, "stream_publisher_"+e.Channel)
				}
				release(storage)
			}
		})
	})
	return nil
}
