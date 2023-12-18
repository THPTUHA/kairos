export type EventMap = {
  [key: string]: (...args: any[]) => void
}

export interface TypedEventEmitter<Events extends EventMap> {
  addListener<E extends keyof Events>(event: E, listener: Events[E]): this
  on<E extends keyof Events>(event: E, listener: Events[E]): this
  once<E extends keyof Events>(event: E, listener: Events[E]): this
  prependListener<E extends keyof Events>(event: E, listener: Events[E]): this
  prependOnceListener<E extends keyof Events>(event: E, listener: Events[E]): this
  off<E extends keyof Events>(event: E, listener: Events[E]): this
  removeAllListeners<E extends keyof Events>(event?: E): this
  removeListener<E extends keyof Events>(event: E, listener: Events[E]): this
  emit<E extends keyof Events>(event: E, ...args: Parameters<Events[E]>): boolean
  rawListeners<E extends keyof Events>(event: E): Events[E][]
  listeners<E extends keyof Events>(event: E): Events[E][]
}

export type ClientEvents = {
  state: (ctx: StateContext) => void;
  connecting: (ctx: ConnectingContext) => void;
  connected: (ctx: ConnectedContext) => void;
  disconnected: (ctx: DisconnectedContext) => void;
  message: (ctx: MessageContext) => void;
  error: (ctx: ErrorContext) => void;
  subscribed: (ctx: ServerSubscribedContext) => void;
  subscribing: (ctx: ServerSubscribingContext) => void;
  unsubscribed: (ctx: ServerUnsubscribedContext) => void;
  publication: (ctx: ServerPublicationContext) => void;
  join: (ctx: ServerJoinContext) => void;
  leave: (ctx: ServerLeaveContext) => void;
}

export enum State {
  Disconnected = "disconnected",
  Connecting = "connecting",
  Connected = "connected"
}

export type SubscriptionEvents = {
  state: (ctx: SubscriptionStateContext) => void;
  subscribing: (ctx: SubscribingContext) => void;
  subscribed: (ctx: SubscribedContext) => void;
  unsubscribed: (ctx: UnsubscribedContext) => void;

  publication: (ctx: PublicationContext) => void;
  join: (ctx: JoinContext) => void;
  leave: (ctx: LeaveContext) => void;
  error: (ctx: SubscriptionErrorContext) => void;
}

export enum SubscriptionState {
  Unsubscribed = "unsubscribed",
  Subscribing = "subscribing",
  Subscribed = "subscribed"
}

export type TransportName = 'websocket';

export interface TransportEndpoint {
  transport: TransportName;
  endpoint: string;
}

export interface Options {
  secret_key: string;
  api_key: string;
  getToken: null | ((ctx: ConnectionTokenContext) => Promise<string>);
  data: any | null;
  getData: null | (() => Promise<any>);
  minReconnectDelay: number;
  maxReconnectDelay: number;
  timeout: number;
  maxServerPingDelay: number;
  websocket: any | null;
  readableStream: any | null;
  eventsource: any | null;
  networkEventTarget: EventTarget | null;
}

export interface StateContext {
  newState: State;
  oldState: State;
}

export interface ConnectedContext {
  client: string;
  transport: string;
  data?: any;
}

export interface ErrorContext {
  type: string;
  error: Error;
  transport?: string;
}

export interface Error {
  code: number;
  message: string;
}

export interface ConnectingContext {
  code: number;
  reason: string;
}

export interface DisconnectedContext {
  code: number;
  reason: string;
}

export interface MessageContext {
  data: any;
}

export interface PublicationContext {
  channel: string;
  data: any;
  info?: ClientInfo;
  offset?: number;
  tags?: Record<string, string>;
}

export interface ClientInfo {
  client: string;
  user: string;
  connInfo?: any;
  chanInfo?: any;
}

export interface JoinContext {
  channel: string;
  info: ClientInfo;
}

export interface LeaveContext {
  channel: string;
  info: ClientInfo;
}

export interface SubscriptionStateContext {
  channel: string;
  newState: SubscriptionState;
  oldState: SubscriptionState;
}

export interface ServerSubscribedContext {
  channel: string;
  data?: any;
}

export interface SubscribedContext {
  channel: string;
  data?: any;
}

export interface SubscriptionErrorContext {
  channel: string;
  type: string;
  error: Error;
}

export interface UnsubscribedContext {
  channel: string;
  code: number;
  reason: string;
}

export interface ServerPublicationContext {
  channel: string;
  data: any;
  info?: ClientInfo;
  offset?: number;
  tags?: Record<string, string>;
}

export interface ServerJoinContext {
  channel: string;
  info: ClientInfo;
}

export interface ServerLeaveContext {
  channel: string;
  info: ClientInfo;
}

export interface ServerUnsubscribedContext {
  channel: string;
}

export interface SubscribingContext {
  channel: string;
  code: number;
  reason: string;
}

export interface ServerSubscribingContext {
  channel: string;
}

export interface ConnectionTokenContext {
}

export interface SubscriptionTokenContext {
  channel: string;
}

export interface SubscriptionDataContext {
  channel: string;
}

export interface PublishResult {
}

export interface PresenceResult {
  clients: Record<string, ClientInfo>;
}

export interface PresenceStatsResult {
  numClients: number;
  numUsers: number;
}

export interface SubscriptionOptions {
  token: string;
  getToken: null | ((ctx: SubscriptionTokenContext) => Promise<string>);
  data: any | null;
  getData: null | ((ctx: SubscriptionDataContext) => Promise<any>);
  minResubscribeDelay: number;
  maxResubscribeDelay: number;
}

export interface CmdTask {
  cmd: number;
  task?: Task;
  deliver_id: number;
  channel: string;
  status: number;
  from?: string;
  send_at: number;
}

export interface CmdReplyTask {
  cmd: number;
  user_id?: number;
  task_id?: number;
  task_name?: string;
  deliver_id?: number;
  run_on: string;
  status?: number;
  workflow_id?: number;
  message?: string;
  content?: any;
  result?: Result;
  send_at: number;
}

export interface Result {
  success: boolean;
  output: string;
  attempt: number;
  startd_at: number;
  finished_at: number;
}


export interface Task {
  id?: number;
  name?: string;
  deps?: string[];
  schedule?: string;
  timezone?: string;
  clients?: string[];
  retries?: number;
  executor?: string;
  payload?: string;
  expires_at?: string;
  input?: string;
  output?: string;
  metadata?: Record<string, string>;
  workflow_id: number;
  user_define_vars?: Record<string, string>;
  dynamic_vars?: Record<string, boolean>;
  execute?: () => void;
}