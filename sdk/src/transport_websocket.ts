export class WebsocketTransport {
  private _transport: any;
  private endpoint: string;
  private options: any;

  constructor(endpoint: string, options: any) {
    this.endpoint = endpoint;
    this.options = options;
    this._transport = null;
  }

  name() {
    return 'websocket';
  }

  subName() {
    return 'websocket';
  }

  supported() {
    return this.options.websocket !== undefined && this.options.websocket !== null;
  }

  initialize(callbacks: any) {
    let subProtocol = '';
    if (subProtocol !== '') {
      this._transport = new this.options.websocket(this.endpoint, subProtocol);
    } else {
      this._transport = new this.options.websocket(this.endpoint);
    }

    this._transport.onopen = () => {
      callbacks.onOpen();
    };

    this._transport.onerror = e => {
      callbacks.onError(e);
    };

    this._transport.onclose = closeEvent => {
      callbacks.onClose(closeEvent);
    };

    this._transport.onmessage = event => {
      callbacks.onMessage(event.data);
    };
  }

  close() {
    this._transport.close();
  }

  send(data: any) {
    this._transport.send(data);
  }
}
