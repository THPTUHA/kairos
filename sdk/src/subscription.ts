import EventEmitter from 'events';
import { Kairos, UnauthorizedError } from './kairos';
import { errorCodes, unsubscribedCodes, subscribingCodes, connectingCodes } from './codes';
import {
  PresenceResult, PresenceStatsResult,
  PublishResult, State, SubscriptionEvents, SubscriptionOptions,
  SubscriptionState, SubscriptionTokenContext, TypedEventEmitter,
  SubscriptionDataContext
} from './types';
import { ttlMilliseconds, backoff } from './utils';

export class Subscription extends (EventEmitter as new () => TypedEventEmitter<SubscriptionEvents>) {
  channel: string;
  state: SubscriptionState;

  private _kairos: Kairos;
  private _promises: Record<number, any>;
  private _resubscribeTimeout?: null | ReturnType<typeof setTimeout> = null;
  private _refreshTimeout?: null | ReturnType<typeof setTimeout> = null;
  private _getToken: null | ((ctx: SubscriptionTokenContext) => Promise<string>);
  private _minResubscribeDelay: number;
  private _maxResubscribeDelay: number;
  private _recover: boolean;
  private _offset: number | null;
  private _epoch: string | null;
  private _resubscribeAttempts: number;
  private _promiseId: number;

  private _token: string;
  private _data: any | null;
  private _getData: null | ((ctx: SubscriptionDataContext) => Promise<any>);
  private _joinLeave: boolean;
  private _inflight: boolean;

  constructor(kairos: Kairos, channel: string, options?: Partial<SubscriptionOptions>) {
    super();
    this.channel = channel;
    this.state = SubscriptionState.Unsubscribed;
    this._kairos = kairos;
    this._token = '';
    this._getToken = null;
    this._data = null;
    this._getData = null;
    this._recover = false;
    this._offset = null;
    this._epoch = null;
    this._joinLeave = false;
    this._minResubscribeDelay = 500;
    this._maxResubscribeDelay = 20000;
    this._resubscribeTimeout = null;
    this._resubscribeAttempts = 0;
    this._promises = {};
    this._promiseId = 0;
    this._inflight = false;
    this._refreshTimeout = null;
    this._setOptions(options);
    this.on('error', function () { Function.prototype(); });
  }

  ready(timeout?: number): Promise<void> {
    if (this.state === SubscriptionState.Unsubscribed) {
      return Promise.reject({ code: errorCodes.subscriptionUnsubscribed, message: this.state });
    }
    if (this.state === SubscriptionState.Subscribed) {
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

  subscribe() {
    if (this._isSubscribed()) {
      return;
    }
    this._resubscribeAttempts = 0;
    this._setSubscribing(subscribingCodes.subscribeCalled, 'subscribe called');
  }

  unsubscribe() {
    this._setUnsubscribed(unsubscribedCodes.unsubscribeCalled, 'unsubscribe called', true);
  }

  publish(data: any): Promise<PublishResult> {
    const self = this;
    return this._methodCall().then(function () {
      return self._kairos.publish(self.channel, data);
    });
  }

  presence(): Promise<PresenceResult> {
    const self = this;
    return this._methodCall().then(function () {
      return self._kairos.presence(self.channel);
    });
  }

  presenceStats(): Promise<PresenceStatsResult> {
    const self = this;
    return this._methodCall().then(function () {
      return self._kairos.presenceStats(self.channel);
    });
  }

  private _methodCall(): any {
    if (this._isSubscribed()) {
      return Promise.resolve();
    }
    if (this._isUnsubscribed()) {
      return Promise.reject({ code: errorCodes.subscriptionUnsubscribed, message: this.state });
    }
    return new Promise((res, rej) => {
      const timeout = setTimeout(function () {
        rej({ code: errorCodes.timeout, message: 'timeout' });
        // @ts-ignore – we are hiding some symbols from public API autocompletion.
      }, this._kairos._config.timeout);
      this._promises[this._nextPromiseId()] = {
        timeout: timeout,
        resolve: res,
        reject: rej
      };
    });
  }

  private _nextPromiseId() {
    return ++this._promiseId;
  }

  private _needRecover() {
    return this._recover === true;
  }

  private _isUnsubscribed() {
    return this.state === SubscriptionState.Unsubscribed;
  }

  private _isSubscribing() {
    return this.state === SubscriptionState.Subscribing;
  }

  private _isSubscribed() {
    return this.state === SubscriptionState.Subscribed;
  }

  private _setState(newState: SubscriptionState) {
    if (this.state !== newState) {
      const oldState = this.state;
      this.state = newState;
      this.emit('state', { newState, oldState, channel: this.channel });
      return true;
    }
    return false;
  }

  private _usesToken() {
    return this._token !== '' || this._getToken !== null;
  }

  private _clearSubscribingState() {
    this._resubscribeAttempts = 0;
    this._clearResubscribeTimeout();
  }

  private _clearSubscribedState() {
    this._clearRefreshTimeout();
  }

  private _setSubscribed(result: any) {
    if (!this._isSubscribing()) {
      return;
    }
    this._clearSubscribingState();

    if (result.recoverable) {
      this._recover = true;
      this._offset = result.offset || 0;
      this._epoch = result.epoch || '';
    }

    this._setState(SubscriptionState.Subscribed);
    // @ts-ignore
    const ctx = this._kairos._getSubscribeContext(this.channel, result);
    this.emit('subscribed', ctx);
    this._resolvePromises();

    const pubs = result.publications;
    if (pubs && pubs.length > 0) {
      for (const i in pubs) {
        if (!pubs.hasOwnProperty(i)) {
          continue;
        }
        this._handlePublication(pubs[i]);
      }
    }

    if (result.expires === true) {
      this._refreshTimeout = setTimeout(() => this._refresh(), ttlMilliseconds(result.ttl));
    }
  }

  private _setSubscribing(code: number, reason: string) {
    if (this._isSubscribing()) {
      return;
    }
    if (this._isSubscribed()) {
      this._clearSubscribedState();
    }
    if (this._setState(SubscriptionState.Subscribing)) {
      this.emit('subscribing', { channel: this.channel, code: code, reason: reason });
    }
    this._subscribe(false, false);
  }

  private _subscribe(optimistic: boolean, skipSending: boolean): any {
    if (this._kairos.state !== State.Connected && !optimistic) {
      return null;
    }

    const self = this;
    const getDataCtx = {
      channel: self.channel
    };

    if (!this._usesToken() || this._token) {
      if (self._getData) {
        self._getData(getDataCtx).then(function (data: any) {
          if (!self._isSubscribing()) {
            return;
          }
          self._data = data;
          self._sendSubscribe(self._token, false);
        })
        return null;
      } else {
        return self._sendSubscribe(self._token, skipSending);
      }
    }
    if (optimistic) {
      return null;
    }
    this._getSubscriptionToken().then(function (token) {
      if (!self._isSubscribing()) {
        return;
      }
      if (!token) {
        self._failUnauthorized();
        return;
      }
      self._token = token;
      if (self._getData) {
        self._getData(getDataCtx).then(function (data: any) {
          if (!self._isSubscribing()) {
            return;
          }
          self._data = data;
          self._sendSubscribe(token, false);
        })
      } else {
        self._sendSubscribe(token, false);
      }
    }).catch(function (e) {
      if (!self._isSubscribing()) {
        return;
      }
      if (e instanceof UnauthorizedError) {
        self._failUnauthorized();
        return;
      }
      self.emit('error', {
        type: 'subscribeToken',
        channel: self.channel,
        error: {
          code: errorCodes.subscriptionSubscribeToken,
          message: e !== undefined ? e.toString() : ''
        }
      });
      self._scheduleResubscribe();
    });
    return null;
  }

  private _sendSubscribe(token: string, skipSending: boolean): any {
    const channel = this.channel;

    const req: any = {
      channel: channel
    };

    if (token) {
      req.token = token;
    }

    if (this._data) {
      req.data = this._data;
    }

    if (this._joinLeave) {
      req.join_leave = true;
    }

    if (this._needRecover()) {
      req.recover = true;
      const offset = this._getOffset();
      if (offset) {
        req.offset = offset;
      }
      const epoch = this._getEpoch();
      if (epoch) {
        req.epoch = epoch;
      }
    }

    const cmd = { subscribe: req };


    // @ts-ignore 
    this._kairos._call(cmd, skipSending).then(resolveCtx => {
      // @ts-ignore 
      const result = resolveCtx.reply.subscribe;
      this._handleSubscribeResponse(
        result
      );
      // @ts-ignore
      if (resolveCtx.next) {
        // @ts-ignore
        resolveCtx.next();
      }
    }, rejectCtx => {
      this._handleSubscribeError(rejectCtx.error);
      if (rejectCtx.next) {
        rejectCtx.next();
      }
    });
    return cmd;
  }

  private _handleSubscribeError(error) {
    if (!this._isSubscribing()) {
      return;
    }
    if (error.code === errorCodes.timeout) {
      // @ts-ignore 
      this._kairos._disconnect(connectingCodes.subscribeTimeout, 'subscribe timeout', true);
      return;
    }
    this._subscribeError(error);
  }

  private _handleSubscribeResponse(result) {
    if (!this._isSubscribing()) {
      return;
    }
    this._setSubscribed(result);
  }

  private _setUnsubscribed(code, reason, sendUnsubscribe) {
    if (this._isUnsubscribed()) {
      return;
    }
    if (this._isSubscribed()) {
      if (sendUnsubscribe) {
        // @ts-ignore 
        this._kairos._unsubscribe(this);
      }
      this._clearSubscribedState();
    }
    if (this._isSubscribing()) {
      if (this._inflight && sendUnsubscribe) {
        // @ts-ignore 
        this._kairos._unsubscribe(this);
      }
      this._clearSubscribingState();
    }
    if (this._setState(SubscriptionState.Unsubscribed)) {
      this.emit('unsubscribed', { channel: this.channel, code: code, reason: reason });
    }
    this._rejectPromises({ code: errorCodes.subscriptionUnsubscribed, message: this.state });
  }

  private _handlePublication(pub: any) {
    // @ts-ignore 
    const ctx = this._kairos._getPublicationContext(this.channel, pub);
    this.emit('publication', ctx);
    if (pub.offset) {
      this._offset = pub.offset;
    }
  }

  protected _handleJoin(join: any) {
    // @ts-ignore 
    const info = this._kairos._getJoinLeaveContext(join.info)
    this.emit('join', { channel: this.channel, info: info });
  }

  protected _handleLeave(leave: any) {
    // @ts-ignore
    const info = this._kairos._getJoinLeaveContext(leave.info)
    this.emit('leave', { channel: this.channel, info: info });
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

  private _scheduleResubscribe() {
    const self = this;
    const delay = this._getResubscribeDelay();
    this._resubscribeTimeout = setTimeout(function () {
      if (self._isSubscribing()) {
        self._subscribe(false, false);
      }
    }, delay);
  }

  private _subscribeError(err: any) {
    if (!this._isSubscribing()) {
      return;
    }
    if (err.code < 100 || err.code === 109 || err.temporary === true) {
      // Token expired error.
      if (err.code === 109) {
        this._token = '';
      }
      const errContext = {
        channel: this.channel,
        type: 'subscribe',
        error: err
      };
      if (this._kairos.state === State.Connected) {
        this.emit('error', errContext);
      }
      this._scheduleResubscribe();
    } else {
      this._setUnsubscribed(err.code, err.message, false);
    }
  }

  private _getResubscribeDelay() {
    const delay = backoff(this._resubscribeAttempts, this._minResubscribeDelay, this._maxResubscribeDelay);
    this._resubscribeAttempts++;
    return delay;
  }

  private _setOptions(options: Partial<SubscriptionOptions> | undefined) {
    if (!options) {
      return;
    }
    if (options.data) {
      this._data = options.data;
    }
    if (options.getData) {
      this._getData = options.getData;
    }
    if (options.minResubscribeDelay !== undefined) {
      this._minResubscribeDelay = options.minResubscribeDelay;
    }
    if (options.maxResubscribeDelay !== undefined) {
      this._maxResubscribeDelay = options.maxResubscribeDelay;
    }
    if (options.token) {
      this._token = options.token;
    }
    if (options.getToken) {
      this._getToken = options.getToken;
    }
  }

  private _getOffset() {
    const offset = this._offset;
    if (offset !== null) {
      return offset;
    }
    return 0;
  }

  private _getEpoch() {
    const epoch = this._epoch;
    if (epoch !== null) {
      return epoch;
    }
    return '';
  }

  private _clearRefreshTimeout() {
    if (this._refreshTimeout !== null) {
      clearTimeout(this._refreshTimeout);
      this._refreshTimeout = null;
    }
  }

  private _clearResubscribeTimeout() {
    if (this._resubscribeTimeout !== null) {
      clearTimeout(this._resubscribeTimeout);
      this._resubscribeTimeout = null;
    }
  }

  private _getSubscriptionToken() {
    const ctx = {
      channel: this.channel
    };
    const getToken = this._getToken;
    if (getToken === null) {
      this.emit('error', {
        type: 'configuration',
        channel: this.channel,
        error: {
          code: errorCodes.badConfiguration,
          message: 'provide a function to get channel subscription token'
        }
      });
      throw new UnauthorizedError('');
    }
    return getToken(ctx);
  }

  private _refresh() {
    this._clearRefreshTimeout();
    const self = this;
    this._getSubscriptionToken().then(function (token) {
      if (!self._isSubscribed()) {
        return;
      }
      if (!token) {
        self._failUnauthorized();
        return;
      }
      self._token = token;
      const req = {
        channel: self.channel,
        token: token
      };
      const msg = {
        'sub_refresh': req
      };
      // @ts-ignore 
      self._kairos._call(msg).then(resolveCtx => {
        // @ts-ignore
        const result = resolveCtx.reply.sub_refresh;
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
      if (e instanceof UnauthorizedError) {
        self._failUnauthorized();
        return;
      }
      self.emit('error', {
        type: 'refreshToken',
        channel: self.channel,
        error: {
          code: errorCodes.subscriptionRefreshToken,
          message: e !== undefined ? e.toString() : ''
        }
      });
      self._refreshTimeout = setTimeout(() => self._refresh(), self._getRefreshRetryDelay());
    });
  }

  private _refreshResponse(result: any) {
    if (!this._isSubscribed()) {
      return;
    }
    this._clearRefreshTimeout();
    if (result.expires === true) {
      this._refreshTimeout = setTimeout(() => this._refresh(), ttlMilliseconds(result.ttl));
    }
  }

  private _refreshError(err: any) {
    if (!this._isSubscribed()) {
      return;
    }
    if (err.code < 100 || err.temporary === true) {
      this.emit('error', {
        type: 'refresh',
        channel: this.channel,
        error: err
      });
      this._refreshTimeout = setTimeout(() => this._refresh(), this._getRefreshRetryDelay());
    } else {
      this._setUnsubscribed(err.code, err.message, true);
    }
  }

  private _getRefreshRetryDelay() {
    return backoff(0, 10000, 20000);
  }

  private _failUnauthorized() {
    this._setUnsubscribed(unsubscribedCodes.unauthorized, 'unauthorized', true);
  }
}
