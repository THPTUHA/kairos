import { Subscription } from './subscription';
import {
  errorCodes, disconnectedCodes,
  connectingCodes, subscribingCodes
} from './codes';

import { WebsocketTransport } from './transport_websocket';

import { JsonCodec } from './json';

import {
  isFunction, startsWith, errorExists,
  backoff, ttlMilliseconds, getTimeNow
} from './utils';

import {
  State, Options, SubscriptionState, ClientEvents,
  TypedEventEmitter, SubscriptionOptions, PublishResult,
  PresenceResult, PresenceStatsResult, SubscribedContext,
  CmdTask,
  CmdReplyTask,
} from './types';

import EventEmitter from 'events';
import { InputTaskCmd, ReplyInputTaskCmd, ReplyMessageCmd, SuccessReceiveInputTaskCmd } from './consts';

const defaults: Options = {
  secret_key: '',
  api_key: '',
  getToken: null,
  data: null,
  getData: null,
  readableStream: null,
  websocket: null,
  eventsource: null,
  minReconnectDelay: 500,
  maxReconnectDelay: 20000,
  timeout: 5000,
  maxServerPingDelay: 10000,
  networkEventTarget: null,
}

interface serverSubscription {
}

export class UnauthorizedError extends Error {
  constructor(message: any) {
    super(message);
    this.name = this.constructor.name;
  }
}

export class Kairos extends (EventEmitter as new () => TypedEventEmitter<ClientEvents>) {
  state: State;
  private _endpoint: string;
  private _apiUrl: string;
  private _transport: WebsocketTransport | null;
  private _transportId: number;
  private _deviceWentOffline: boolean;
  private _transportClosed: boolean;
  private _reconnecting: boolean;
  private _reconnectTimeout?: null | ReturnType<typeof setTimeout> = null;
  private _reconnectAttempts: number;
  private _client: null;
  private _subs: Record<string, Subscription>;
  private _serverSubs: Record<string, serverSubscription>;
  private _commandId: number;
  private _refreshRequired: boolean;
  private _refreshTimeout?: null | ReturnType<typeof setTimeout> = null;
  private _callbacks: Record<number, any>;
  private _secret_key: string;
  private _api_key: string;
  private _data: any;
  private _dispatchPromise: Promise<void>;
  private _serverPing: number;
  private _serverPingTimeout?: null | ReturnType<typeof setTimeout> = null;
  private _promises: Record<number, any>;
  private _promiseId: number;
  private _networkEventsSet: boolean;
  private _config: Options;
  protected _codec: any;

  static SubscriptionState: typeof SubscriptionState;
  static State: typeof State;
  static UnauthorizedError: typeof UnauthorizedError;

  constructor(endpoint: string, _apiUrl: string, options?: Partial<Options>) {
    super();
    this.state = State.Disconnected;
    this._endpoint = endpoint;
    this._apiUrl = _apiUrl;
    this._transport = null;
    this._transportId = 0;
    this._deviceWentOffline = false;
    this._transportClosed = true;
    this._codec = new JsonCodec();
    this._reconnecting = false;
    this._reconnectTimeout = null;
    this._reconnectAttempts = 0;
    this._client = null;
    this._subs = {};
    this._serverSubs = {};
    this._commandId = 0;
    this._refreshRequired = false;
    this._refreshTimeout = null;
    this._callbacks = {};
    this._secret_key = '';
    this._api_key = '';
    this._data = null;
    this._dispatchPromise = Promise.resolve();
    this._serverPing = 0;
    this._serverPingTimeout = null;
    this._promises = {};
    this._promiseId = 0;
    this._networkEventsSet = false;

    this._config = { ...defaults, ...options };
    this._configure();
    this.on('error', function (e) {
      console.log("Kairos err", e)
      Function.prototype();
    });
  }

  newSubscription(channel: string, options?: Partial<SubscriptionOptions>): Subscription {
    if (this.getSubscription(channel) !== null) {
      throw new Error('Subscription to the channel ' + channel + ' already exists');
    }
  
    const sub = new Subscription(this, channel, options);
    this._subs[channel] = sub;
    console.log("SUB", this._subs)

    return sub;
  }

  getSubscription(channel: string): Subscription | null {
    return this._getSub(channel);
  }

  removeSubscription(sub: Subscription | null) {
    if (!sub) {
      return;
    }
    if (sub.state !== SubscriptionState.Unsubscribed) {
      sub.unsubscribe();
    }
    this._removeSubscription(sub);
  }

  /** Get a map with all current client-side subscriptions. */
  subscriptions(): Record<string, Subscription> {
    return this._subs;
  }

  ready(timeout?: number): Promise<void> {
    if (this.state === State.Disconnected) {
      return Promise.reject({ code: errorCodes.clientDisconnected, message: 'client disconnected' });
    }
    if (this.state === State.Connected) {
      return Promise.resolve();
    }
    return new Promise((res, rej) => {
      const ctx: any = {
        resolve: res,
        reject: rej
      };
      if (timeout) {
        ctx.timeout = setTimeout(function () {
          rej({ code: errorCodes.timeout, message: 'timeout' });
        }, timeout);
      }
      this._promises[this._nextPromiseId()] = ctx;
    });
  }

  connect() {
    if (this._isConnected()) {
      return;
    }
    if (this._isConnecting()) {
      return;
    }
    this._reconnectAttempts = 0;
    this._startConnecting();
  }

  disconnect() {
    this._disconnect(disconnectedCodes.disconnectCalled, 'disconnect called', false);
  }

  setToken(secret_key: string) {
    this._secret_key = secret_key;
  }

  send(data: any): Promise<void> {
    const cmd = {
      send: {
        data: data
      }
    };

    const self = this;

    return this._methodCall().then(function () {
      const sent = self._transportSendCommands([cmd]);
      if (!sent) {
        return Promise.reject(self._createErrorObject(errorCodes.transportWriteError, 'transport write error'));
      }
      return Promise.resolve();
    });
  }

  publish(channel: string, data: any, workflow_id?: number): Promise<PublishResult> {
    const payload: CmdReplyTask = {
      send_at: getTimeNow(),
      cmd: ReplyMessageCmd,
      run_on: `${channel}_channel`,
      content: data,
      deliver_id: data.deliver_id,
    }
    
    if(workflow_id){
      payload.workflow_id = workflow_id
    }
    console.log("PAYLOAD",payload)

    if (!channel.includes("-")) {
      if (channel.includes(":")) {
        const root = channel.split(":")
        if (!root[0]) {
          throw Error("Not permission")
        }
        const e = this._data[root[0]]
        if (!e) {
          throw Error("Not permission")
        }

        channel = `${root[0]}-${e.id}:${root[1]}`
      } else {
        const e = this._data[channel]
        if (!e) {
          throw Error("Not permission")
        }
        channel = `${channel}-${e.id}`
      }
    }
   
    const cmd = {
      publish: {
        channel: channel,
        data: payload
      }
    };

    const self = this;

    return this._methodCall().then(function () {
      return self._callPromise(cmd, function () {
        return {};
      });
    });
  }
  /** presence for a channel. */
  presence(channel: string): Promise<PresenceResult> {
    const cmd = {
      presence: {
        channel: channel
      }
    };

    const self = this;

    return this._methodCall().then(function () {
      return self._callPromise(cmd, function (reply: any) {
        const clients = reply.presence.presence;
        for (const clientId in clients) {
          if (clients.hasOwnProperty(clientId)) {
            const connInfo = clients[clientId]['conn_info'];
            const chanInfo = clients[clientId]['chan_info'];
            if (connInfo) {
              clients[clientId].connInfo = connInfo;
            }
            if (chanInfo) {
              clients[clientId].chanInfo = chanInfo;
            }
          }
        }
        return {
          'clients': clients
        };
      });
    });
  }

  presenceStats(channel: string): Promise<PresenceStatsResult> {
    const cmd = {
      'presence_stats': {
        channel: channel
      }
    };

    const self = this;

    return this._methodCall().then(function () {
      return self._callPromise(cmd, function (reply: any) {
        const result = reply.presence_stats;
        return {
          'numUsers': result.num_users,
          'numClients': result.num_clients
        };
      });
    });
  }

  /** @internal */
  protected _formatOverride() {
    return;
  }

  private _configure() {
    if (!('Promise' in globalThis)) {
      throw new Error('Promise polyfill required');
    }

    if (!this._endpoint) {
      throw new Error('endpoint configuration required');
    }

    if (this._config.secret_key !== null) {
      this._secret_key = this._config.secret_key;
    }

    if (this._config.api_key !== null) {
      this._api_key = this._config.api_key;
    }

    if (this._config.data !== null) {
      this._data = this._config.data;
    }

    this._codec = new JsonCodec();
    this._formatOverride();
  }

  private _setState(newState: State) {
    if (this.state !== newState) {
      this._reconnecting = false;
      const oldState = this.state;
      this.state = newState;
      this.emit('state', { newState, oldState });
      return true;
    }
    return false;
  }

  private _isDisconnected() {
    return this.state === State.Disconnected;
  }

  private _isConnecting() {
    return this.state === State.Connecting;
  }

  private _isConnected() {
    return this.state === State.Connected;
  }

  private _nextCommandId() {
    return ++this._commandId;
  }

  private _setNetworkEvents() {
    if (this._networkEventsSet) {
      return;
    }
    let eventTarget: EventTarget | null = null;
    if (this._config.networkEventTarget !== null) {
      eventTarget = this._config.networkEventTarget;
    } else if (typeof globalThis.addEventListener !== 'undefined') {
      eventTarget = globalThis as EventTarget;
    }
    if (eventTarget) {
      eventTarget.addEventListener('offline', () => {
        if (this.state === State.Connected || this.state === State.Connecting) {
          this._disconnect(connectingCodes.transportClosed, 'transport closed', true);
          this._deviceWentOffline = true;
        }
      });
      eventTarget.addEventListener('online', () => {
        if (this.state !== State.Connecting) {
          return;
        }
        if (this._deviceWentOffline && !this._transportClosed) {
          this._deviceWentOffline = false;
          this._transportClosed = true;
        }
        this._clearReconnectTimeout();
        this._startReconnecting();
      });
      this._networkEventsSet = true;
    }
  }

  private _getReconnectDelay() {
    const delay = backoff(this._reconnectAttempts, this._config.minReconnectDelay, this._config.maxReconnectDelay);
    this._reconnectAttempts += 1;
    return delay;
  }

  private _clearOutgoingRequests() {
    for (const id in this._callbacks) {
      if (this._callbacks.hasOwnProperty(id)) {
        const callbacks = this._callbacks[id];
        clearTimeout(callbacks.timeout);
        const errback = callbacks.errback;
        if (!errback) {
          continue;
        }
        errback({ error: this._createErrorObject(errorCodes.connectionClosed, 'connection closed') });
      }
    }
    this._callbacks = {};
  }

  private _clearConnectedState() {
    this._client = null;
    this._clearServerPingTimeout();
    this._clearRefreshTimeout();

    for (const channel in this._subs) {
      if (!this._subs.hasOwnProperty(channel)) {
        continue;
      }
      const sub = this._subs[channel];
      if (sub.state === SubscriptionState.Subscribed) {
        // @ts-ignore
        sub._setSubscribing(subscribingCodes.transportClosed, 'transport closed');
      }
    }

    for (const channel in this._serverSubs) {
      if (this._serverSubs.hasOwnProperty(channel)) {
        this.emit('subscribing', { channel: channel });
      }
    }
  }

  private _handleWriteError(commands: any[]) {
    for (const command of commands) {
      const id = command.id;
      if (!(id in this._callbacks)) {
        continue;
      }
      const callbacks = this._callbacks[id];
      clearTimeout(this._callbacks[id].timeout);
      delete this._callbacks[id];
      const errback = callbacks.errback;
      errback({ error: this._createErrorObject(errorCodes.transportWriteError, 'transport write error') });
    }
  }

  private _transportSendCommands(commands: any[]) {
    if (!commands.length) {
      return true;
    }
    if (!this._transport) {
      return false
    }
    try {
      this._transport.send(this._codec.encodeCommands(commands));
    } catch (e) {
      this._handleWriteError(commands);
      return false;
    }
    return true;
  }

  private _initializeTransport() {
    let websocket: any;
    if (this._config.websocket !== null) {
      websocket = this._config.websocket;
    } else {
      if (!(typeof globalThis.WebSocket !== 'function' && typeof globalThis.WebSocket !== 'object')) {
        websocket = globalThis.WebSocket;
      }
    }

    if (startsWith(this._endpoint, 'http')) {
      throw new Error('Provide explicit transport endpoints configuration in case of using HTTP ');
    } else {
      this._transport = new WebsocketTransport(this._endpoint as string, {
        websocket: websocket
      });
      if (!this._transport.supported()) {
        throw new Error('WebSocket not available');
      }
    }

    const self = this;
    const transport = this._transport;
    const transportId = this._nextTransportId();
    let wasOpen = false;

    let optimistic = true;
    this._setNetworkEvents();

    this._transportClosed = false;

    let connectTimeout: any;
    connectTimeout = setTimeout(function () {
      transport.close();
    }, this._config.timeout);

    this._transport.initialize({
      onOpen: function () {
        if (connectTimeout) {
          clearTimeout(connectTimeout);
          connectTimeout = null;
        }
        if (self._transportId != transportId) {
          transport.close();
          return;
        }
        wasOpen = true;
        self._sendConnect(false);
        if (optimistic) {
          self._sendSubscribeCommands(true, false);
        }
      },
      onError: function (e: any) {
        if (self._transportId != transportId) {
          console.log("error callback from non-actual transport", e)
          return;
        }
      },
      onClose: function (closeEvent) {
        if (connectTimeout) {
          clearTimeout(connectTimeout);
          connectTimeout = null;
        }
        if (self._transportId != transportId) {
          console.log("'close callback from non-actual transport'")
          return;
        }
        self._transportClosed = true;

        let reason = 'connection closed';
        let needReconnect = true;
        let code = 0;

        if (closeEvent && 'code' in closeEvent && closeEvent.code) {
          code = closeEvent.code;
        }

        if (closeEvent && closeEvent.reason) {
          try {
            const advice = JSON.parse(closeEvent.reason);
            reason = advice.reason;
            needReconnect = advice.reconnect;
          } catch (e) {
            reason = closeEvent.reason;
            if ((code >= 3500 && code < 4000) || (code >= 4500 && code < 5000)) {
              needReconnect = false;
            }
          }
        }
        if (code < 3000) {
          if (code === 1009) {
            code = disconnectedCodes.messageSizeLimit;
            reason = 'message size limit exceeded';
            needReconnect = false;
          } else {
            code = connectingCodes.transportClosed;
            reason = 'transport closed';
          }
        }

        if (self._isConnecting() && !wasOpen) {
          self.emit('error', {
            type: 'transport',
            error: {
              code: errorCodes.transportClosed,
              message: 'transport closed'
            },
            transport: transport.name()
          });
        }

        self._reconnecting = false;
        self._disconnect(code, reason, needReconnect);
      },
      onMessage: function (data) {
        self._dataReceived(data);
      }
    });
  }

  private async _fetchCertificate() {
    const headers = {
      "Content-Type": "application/json",
      "Authorization": `Bearer ${this._secret_key}`,
      "api-key": this._api_key
    };
    try {
      const response = await fetch(this._apiUrl, {
        method: "GET",
        headers: headers
      });

      if (!response.ok) {
        throw new Error(`HTTP error! Status: ${response.status}`);
      }

      const data = await response.json();
      return data.permissions;
    } catch (error) {
      console.error("Error fetching certificate:", error);
      throw error;
    }
  }

  private _sendConnect(skipSending: boolean): any {
    const connectCommand = this._constructConnectCommand();
    const self = this;
    this._call(connectCommand, skipSending).then(resolveCtx => {
      // @ts-ignore
      const result = resolveCtx.reply.connect;
      self._connectResponse(result);
      // @ts-ignore 
      if (resolveCtx.next) {
        // @ts-ignore
        resolveCtx.next();
      }
    }, rejectCtx => {
      self._connectError(rejectCtx.error);
      if (rejectCtx.next) {
        rejectCtx.next();
      }
    });
    return connectCommand;
  }

  private _startReconnecting() {
    if (!this._isConnecting()) {
      return;
    }
    if (this._reconnecting) {
      return;
    }
    if (this._transportClosed === false) {
      return;
    }

    this._reconnecting = true;
    const self = this;
    const emptyToken = this._secret_key === '';
    const needTokenRefresh = this._refreshRequired || (emptyToken && this._config.getToken !== null);
    if (!needTokenRefresh) {
      if (!self._isConnecting()) {
        return;
      }

      if (this._apiUrl) {
        this._fetchCertificate().then(function (data: any) {
          const channelMp = {}
          for (const e of data) {
            channelMp[e.name] = e
          }
          if (!self._isConnecting()) {
            return;
          }

          self._data = channelMp;
          self._initializeTransport();
        })
      } else {
        self._initializeTransport();
      }

      return;
    }

    this._getToken().then(function (secret_key: string) {
      if (!self._isConnecting()) {
        return;
      }
      if (secret_key == null || secret_key == undefined) {
        self._failUnauthorized();
        return;
      }
      self._secret_key = secret_key;
      if (self._config.getData) {
        self._config.getData().then(function (data: any) {
          if (!self._isConnecting()) {
            return;
          }
          self._data = data;
          self._initializeTransport();
        })
      } else {
        self._initializeTransport();
      }
    }).catch(function (e) {
      if (!self._isConnecting()) {
        return;
      }
      if (e instanceof UnauthorizedError) {
        self._failUnauthorized();
        return;
      }
      self.emit('error', {
        'type': 'connectToken',
        'error': {
          code: errorCodes.clientConnectToken,
          message: e !== undefined ? e.toString() : ''
        }
      });
      const delay = self._getReconnectDelay();
      self._reconnecting = false;
      self._reconnectTimeout = setTimeout(() => {
        self._startReconnecting();
      }, delay);
    });
  }

  private _connectError(err: any) {
    if (this.state !== State.Connecting) {
      return;
    }
    if (err.code === 109) { 
      this._refreshRequired = true;
    }
    if (err.code < 100 || err.temporary === true || err.code === 109) {
      this.emit('error', {
        'type': 'connect',
        'error': err
      });
      this._reconnecting = false;
      this._disconnect(err.code, err.message, true);
    } else {
      this._disconnect(err.code, err.message, false);
    }
  }

  private _scheduleReconnect() {
    if (!this._isConnecting()) {
      return;
    }
    let delay = this._getReconnectDelay();
    this._reconnectTimeout = setTimeout(() => {
      this._startReconnecting();
    }, delay);
  }

  private _constructConnectCommand(): any {
    const req: any = {};

    if (this._secret_key) {
      req.token = this._secret_key;
    }
    if (this._data) {
      req.data = this._data;
    }
    const subs = {};
    let hasSubs = false;
    if (hasSubs) {
      req.subs = subs;
    }
    return {
      connect: req
    };
  }

  private _methodCall(): any {
    if (this._isConnected()) {
      return Promise.resolve();
    }
    return new Promise((res, rej) => {
      const timeout = setTimeout(function () {
        rej({ code: errorCodes.timeout, message: 'timeout' });
      }, this._config.timeout);
      this._promises[this._nextPromiseId()] = {
        timeout: timeout,
        resolve: res,
        reject: rej
      };
    });
  }

  private _callPromise(cmd: any, resultCB: any): any {
    return new Promise((resolve, reject) => {
      this._call(cmd, false).then(resolveCtx => {
        // @ts-ignore
        resolve(resultCB(resolveCtx.reply));
        // @ts-ignore 
        if (resolveCtx.next) {
          // @ts-ignore 
          resolveCtx.next();
        }
      }, rejectCtx => {
        reject(rejectCtx.error);
        if (rejectCtx.next) {
          rejectCtx.next();
        }
      });
    });
  }

  private _dataReceived(data) {
    if (this._serverPing > 0) {
      this._waitServerPing();
    }
    const replies = this._codec.decodeReplies(data);
    this._dispatchPromise = this._dispatchPromise.then(() => {
      let finishDispatch;
      this._dispatchPromise = new Promise(resolve => {
        finishDispatch = resolve;
      });
      this._dispatchSynchronized(replies, finishDispatch);
    });
  }

  private _dispatchSynchronized(replies: any[], finishDispatch: any) {
    let p: Promise<unknown> = Promise.resolve();
    for (const i in replies) {
      if (replies.hasOwnProperty(i)) {
        p = p.then(() => {
          return this._dispatchReply(replies[i]);
        });
      }
    }
    p = p.then(() => {
      finishDispatch();
    });
  }

  private _dispatchReply(reply: any) {
    let next: any;
    const p = new Promise(resolve => {
      next = resolve;
    });

    if (reply === undefined || reply === null) {
      next();
      return p;
    }

    const id = reply.id;

    if (id && id > 0) {
      this._handleReply(reply, next);
    } else {
      if (!reply.push) {
        this._handleServerPing(next);
      } else {
        this._handlePush(reply.push, next);
      }
    }

    return p;
  }

  private _call(cmd: any, skipSending: boolean) {
    return new Promise((resolve, reject) => {
      cmd.id = this._nextCommandId();
      this._registerCall(cmd.id, resolve, reject);
      if (!skipSending) {
        this._addCommand(cmd);
      }
    });
  }

  private _startConnecting() {
    if (this._setState(State.Connecting)) {
      this.emit('connecting', { code: connectingCodes.connectCalled, reason: 'connect called' });
    }
    this._client = null;
    this._startReconnecting();
  }

  private _disconnect(code: number, reason: string, reconnect: boolean) {
    if (this._isDisconnected()) {
      return;
    }
    const previousState = this.state;
    const ctx = {
      code: code,
      reason: reason
    };

    let needEvent = false;

    if (reconnect) {
      needEvent = this._setState(State.Connecting);
    } else {
      needEvent = this._setState(State.Disconnected);
      this._rejectPromises({ code: errorCodes.clientDisconnected, message: 'disconnected' });
    }

    this._clearOutgoingRequests();

    if (previousState === State.Connecting) {
      this._clearReconnectTimeout();
    }
    if (previousState === State.Connected) {
      this._clearConnectedState();
    }

    if (needEvent) {
      if (this._isConnecting()) {
        this.emit('connecting', ctx);
      } else {
        this.emit('disconnected', ctx);
      }
    }

    if (this._transport) {
      const transport = this._transport;
      this._transport = null;
      transport.close(); 
      this._transportClosed = true;
      this._nextTransportId();
    }
    this._scheduleReconnect();
  }

  private _failUnauthorized() {
    this._disconnect(disconnectedCodes.unauthorized, 'unauthorized', false);
  }

  private _getToken(): Promise<string> {
    if (!this._config.getToken) {
      this.emit('error', {
        type: 'configuration',
        error: {
          code: errorCodes.badConfiguration,
          message: 'secret_key expired but no getToken function set in the configuration'
        }
      });
      throw new UnauthorizedError('');
    }
    return this._config.getToken({});
  }

  private _refresh() {
    const clientId = this._client;
    const self = this;
    this._getToken().then(function (secret_key) {
      if (clientId !== self._client) {
        return;
      }
      if (!secret_key) {
        self._failUnauthorized();
        return;
      }
      self._secret_key = secret_key;

      if (!self._isConnected()) {
        return;
      }

      const cmd = {
        refresh: { secret_key: self._secret_key }
      };

      self._call(cmd, false).then(resolveCtx => {
        // @ts-ignore 
        const result = resolveCtx.reply.refresh;
        self._refreshResponse(result);
        // @ts-ignore 
        if (resolveCtx.next) {
          // @ts-ignore 
          resolveCtx.next();
        }
      }, rejectCtx => {
        self._refreshError(rejectCtx.error);
        if (rejectCtx.next) {
          rejectCtx.next();
        }
      });
    }).catch(function (e) {
      if (!self._isConnected()) {
        return;
      }
      if (e instanceof UnauthorizedError) {
        self._failUnauthorized();
        return;
      }
      self.emit('error', {
        type: 'refreshToken',
        error: {
          code: errorCodes.clientRefreshToken,
          message: e !== undefined ? e.toString() : ''
        }
      });
      self._refreshTimeout = setTimeout(() => self._refresh(), self._getRefreshRetryDelay());
    });
  }

  private _refreshError(err: any) {
    if (err.code < 100 || err.temporary === true) {
      this.emit('error', {
        type: 'refresh',
        error: err
      });
      this._refreshTimeout = setTimeout(() => this._refresh(), this._getRefreshRetryDelay());
    } else {
      this._disconnect(err.code, err.message, false);
    }
  }

  private _getRefreshRetryDelay() {
    return backoff(0, 5000, 10000);
  }

  private _refreshResponse(result: any) {
    if (this._refreshTimeout) {
      clearTimeout(this._refreshTimeout);
      this._refreshTimeout = null;
    }
    if (result.expires) {
      this._client = result.client;
      this._refreshTimeout = setTimeout(() => this._refresh(), ttlMilliseconds(result.ttl));
    }
  }

  private _removeSubscription(sub: Subscription | null) {
    if (sub === null) {
      return;
    }
    delete this._subs[sub.channel];
  }

  protected _unsubscribe(sub: Subscription) {
    if (!this._isConnected()) {
      return;
    }
    const req = {
      channel: sub.channel
    };
    const cmd = { unsubscribe: req };

    const self = this;

    this._call(cmd, false).then(resolveCtx => {
      // @ts-ignore 
      if (resolveCtx.next) {
        // @ts-ignore 
        resolveCtx.next();
      }
    }, rejectCtx => {
      if (rejectCtx.next) {
        rejectCtx.next();
      }
      self._disconnect(connectingCodes.unsubscribeError, 'unsubscribe error', true);
    });
  }

  private _getSub(channel: string) {
    const sub = this._subs[channel];
    if (!sub) {
      return null;
    }
    return sub;
  }

  private _isServerSub(channel: string) {
    return this._serverSubs[channel] !== undefined;
  }

  private _sendSubscribeCommands(optimistic: boolean, skipSending: boolean): any[] {
    const commands: any[] = [];
    for (const channel in this._subs) {
      if (!this._subs.hasOwnProperty(channel)) {
        continue;
      }
      const sub = this._subs[channel];
      // @ts-ignore
      if (sub._inflight === true) {
        continue;
      }
      if (sub.state === SubscriptionState.Subscribing) {
        // @ts-ignore
        const cmd = sub._subscribe(optimistic, skipSending);
        if (cmd) {
          commands.push(cmd);
        }
      }
    }
    return commands;
  }

  private _connectResponse(result: any) {
    this._reconnectAttempts = 0;
    this._refreshRequired = false;

    if (this._isConnected()) {
      return;
    }

    this._client = result.client;
    this._setState(State.Connected);

    if (this._refreshTimeout) {
      clearTimeout(this._refreshTimeout);
    }
    if (result.expires) {
      this._refreshTimeout = setTimeout(() => this._refresh(), ttlMilliseconds(result.ttl));
    }

    this._sendSubscribeCommands(false, false);

    const ctx: any = {
      client: result.client,
      transport: this._transport?.subName()
    };
    if (result.data) {
      ctx.data = result.data;
    }

    this.emit('connected', ctx);

    this._resolvePromises();

    this._processServerSubs(result.subs || {});

    if (result.ping && result.ping > 0) {
      this._serverPing = result.ping * 1000;
      this._waitServerPing();
    } else {
      this._serverPing = 0;
    }
  }

  private _processServerSubs(subs: Record<string, any>) {
    for (const channel in subs) {
      if (!subs.hasOwnProperty(channel)) {
        continue;
      }
      const sub = subs[channel];
      this._serverSubs[channel] = {
        'offset': sub.offset,
        'epoch': sub.epoch,
        'recoverable': sub.recoverable || false
      };
      const subCtx = this._getSubscribeContext(channel, sub);
      this.emit('subscribed', subCtx);
    }

    for (const channel in subs) {
      if (!subs.hasOwnProperty(channel)) {
        continue;
      }
      const sub = subs[channel];
      if (sub.recovered) {
        const pubs = sub.publications;
        if (pubs && pubs.length > 0) {
          for (const i in pubs) {
            if (pubs.hasOwnProperty(i)) {
              this._handlePublication(channel, pubs[i]);
            }
          }
        }
      }
    }

    for (const channel in this._serverSubs) {
      if (!this._serverSubs.hasOwnProperty(channel)) {
        continue;
      }
      if (!subs[channel]) {
        this.emit('unsubscribed', { channel: channel });
        delete this._serverSubs[channel];
      }
    }
  }

  private _clearRefreshTimeout() {
    if (this._refreshTimeout !== null) {
      clearTimeout(this._refreshTimeout);
      this._refreshTimeout = null;
    }
  }

  private _clearReconnectTimeout() {
    if (this._reconnectTimeout !== null) {
      clearTimeout(this._reconnectTimeout);
      this._reconnectTimeout = null;
    }
  }

  private _clearServerPingTimeout() {
    if (this._serverPingTimeout !== null) {
      clearTimeout(this._serverPingTimeout);
      this._serverPingTimeout = null;
    }
  }

  private _waitServerPing() {
    if (this._config.maxServerPingDelay === 0) {
      return;
    }
    if (!this._isConnected()) {
      return;
    }
    this._clearServerPingTimeout();
    this._serverPingTimeout = setTimeout(() => {
      if (!this._isConnected()) {
        return;
      }
      this._disconnect(connectingCodes.noPing, 'no ping', true);
    }, this._serverPing + this._config.maxServerPingDelay);
  }

  private _getSubscribeContext(channel: string, result: any): SubscribedContext {
    const ctx: any = {
      channel: channel,
    };
    if (result.data) {
      ctx.data = result.data;
    }
    return ctx;
  }

  private _handleReply(reply: any, next: any) {
    const id = reply.id;
    if (!(id in this._callbacks)) {
      next();
      return;
    }
    const callbacks = this._callbacks[id];
    clearTimeout(this._callbacks[id].timeout);
    delete this._callbacks[id];

    if (!errorExists(reply)) {
      const callback = callbacks.callback;
      if (!callback) {
        return;
      }
      callback({ reply, next });
    } else {
      const errback = callbacks.errback;
      if (!errback) {
        next();
        return;
      }
      const error = reply.error;
      errback({ error, next });
    }
  }

  private _handleJoin(channel: string, join: any) {
    const sub = this._getSub(channel);
    if (!sub) {
      if (this._isServerSub(channel)) {
        const ctx = { channel: channel, info: this._getJoinLeaveContext(join.info) };
        this.emit('join', ctx);
      }
      return;
    }
    // @ts-ignore 
    sub._handleJoin(join);
  }

  private _handleLeave(channel: string, leave: any) {
    const sub = this._getSub(channel);
    if (!sub) {
      if (this._isServerSub(channel)) {
        const ctx = { channel: channel, info: this._getJoinLeaveContext(leave.info) };
        this.emit('leave', ctx);
      }
      return;
    }
    // @ts-ignore
    sub._handleLeave(leave);
  }

  private _handleUnsubscribe(channel: string, unsubscribe: any) {
    const sub = this._getSub(channel);
    if (!sub) {
      if (this._isServerSub(channel)) {
        delete this._serverSubs[channel];
        this.emit('unsubscribed', { channel: channel });
      }
      return;
    }
    if (unsubscribe.code < 2500) {
      // @ts-ignore 
      sub._setUnsubscribed(unsubscribe.code, unsubscribe.reason, false);
    } else {
      // @ts-ignore
      sub._setSubscribing(unsubscribe.code, unsubscribe.reason);
    }
  }

  private _handleSubscribe(channel: string, sub: any) {
    this._serverSubs[channel] = {
    };
    this.emit('subscribed', this._getSubscribeContext(channel, sub));
  }

  private _handleDisconnect(disconnect: any) {
    const code = disconnect.code;
    let reconnect = true;
    if ((code >= 3500 && code < 4000) || (code >= 4500 && code < 5000)) {
      reconnect = false;
    }

    console.log("Handle disconnect")
    this._disconnect(code, disconnect.reason, reconnect);
  }

  private _getPublicationContext(channel: string, pub: any) {
    let ctx: any = {
      channel: channel,
    };
    const cmdTask = pub.data as CmdTask
    ctx = {
      ...ctx,
      ...cmdTask
    }
    if (pub.info) {
      ctx.info = this._getJoinLeaveContext(pub.info);
    }
    return ctx;
  }

  private _getJoinLeaveContext(clientInfo: any) {
    const info: any = {
      client: clientInfo.client,
      user: clientInfo.user
    };
    if (clientInfo.conn_info) {
      info.connInfo = clientInfo.conn_info;
    }
    if (clientInfo.chan_info) {
      info.chanInfo = clientInfo.chan_info;
    }
    return info;
  }

  private _handlePublication(channel: string, pub: any) {
    const sub = this._getSub(channel);
    console.log("PUB---", pub)
    if (!sub) {
      if (this._isServerSub(channel)) {
        const ctx = this._getPublicationContext(channel, pub);
        if (ctx.cmd === InputTaskCmd) {
          const reply :any = {
            cmd: ReplyInputTaskCmd,
            deliver_id: ctx.deliver_id,
            run_on: channel,
            status: SuccessReceiveInputTaskCmd,
            send_at: getTimeNow(),
            workflow_id : pub.data.workflow_id
          }
          if (ctx.task) {
            reply.task_id =  ctx.task.id
            reply.task_name = ctx.task.name
          }
          ctx.data = ctx.message
          delete ctx.message
          this.publish(channel, reply,reply.workflow_id)
        }
        this.emit('publication', ctx);
      }
      return; 
    }
    // @ts-ignore
    sub._handlePublication(pub);
  }

  private _handleMessage(message: any) {
    this.emit('message', { data: message.data });
  }

  private _handleServerPing(next: any) {
    next();
  }

  private _handlePush(data: any, next: any) {
    const channel = data.channel;
    if (data.pub) {
      this._handlePublication(channel, data.pub);
    } else if (data.message) {
      this._handleMessage(data.message);
    } else if (data.join) {
      this._handleJoin(channel, data.join);
    } else if (data.leave) {
      this._handleLeave(channel, data.leave);
    } else if (data.unsubscribe) {
      this._handleUnsubscribe(channel, data.unsubscribe);
    } else if (data.subscribe) {
      this._handleSubscribe(channel, data.subscribe);
    } else if (data.disconnect) {
      this._handleDisconnect(data.disconnect);
    }
    next();
  }

  private _createErrorObject(code: number, message: string, temporary?: boolean) {
    const errObject: any = {
      code: code,
      message: message
    };
    if (temporary) {
      errObject.temporary = true;
    }
    return errObject;
  }

  private _registerCall(id: number, callback: any, errback: any) {
    this._callbacks[id] = {
      callback: callback,
      errback: errback,
      timeout: null
    };
    this._callbacks[id].timeout = setTimeout(() => {
      delete this._callbacks[id];
      if (isFunction(errback)) {
        errback({ error: this._createErrorObject(errorCodes.timeout, 'timeout') });
      }
    }, this._config.timeout);
  }

  private _addCommand(command: any) {
    this._transportSendCommands([command]);
  }

  private _nextPromiseId() {
    return ++this._promiseId;
  }

  private _nextTransportId() {
    return ++this._transportId;
  }

  private _resolvePromises() {
    for (const id in this._promises) {
      if (!this._promises.hasOwnProperty(id)) {
        continue;
      }
      if (this._promises[id].timeout) {
        clearTimeout(this._promises[id].timeout);
      }
      this._promises[id].resolve();
      delete this._promises[id];
    }
  }

  private _rejectPromises(err: any) {
    for (const id in this._promises) {
      if (!this._promises.hasOwnProperty(id)) {
        continue;
      }
      if (this._promises[id].timeout) {
        clearTimeout(this._promises[id].timeout);
      }
      this._promises[id].reject(err);
      delete this._promises[id];
    }
  }
}

Kairos.SubscriptionState = SubscriptionState;
Kairos.State = State
Kairos.UnauthorizedError = UnauthorizedError;
