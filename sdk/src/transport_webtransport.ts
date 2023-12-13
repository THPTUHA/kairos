export class WebtransportTransport {
  private _transport: any;
  private _stream: any;
  private _writer: any;
  private endpoint: string;
  private options: any;
  _utf8decoder: TextDecoder;

  constructor(endpoint: string, options: any) {
    this.endpoint = endpoint;
    this.options = options;
    this._transport = null;
    this._stream = null;
    this._writer = null;
    this._utf8decoder = new TextDecoder();
  }

  name() {
    return 'webtransport';
  }

  subName() {
    return 'webtransport';
  }

  emulation() {
    return false;
  }

  supported() {
    return this.options.webtransport !== undefined && this.options.webtransport !== null;
  }

  async initialize(callbacks: any) {
    let url: any;
    if (globalThis && globalThis.document && globalThis.document.baseURI) {
      url = new URL(this.endpoint, globalThis.document.baseURI);
    } else {
      url = new URL(this.endpoint);
    }
    const eventTarget = new EventTarget();

    this._transport = new this.options.webtransport(url.toString());
    this._transport.closed.then(() => {
      callbacks.onClose({
        code: 4,
        reason: 'connection closed'
      });
    }).catch(() => {
      callbacks.onClose({
        code: 4,
        reason: 'connection closed'
      });
    });
    try {
      await this._transport.ready;
    } catch {
      this.close();
      return;
    }
    let stream: any;
    try {
      stream = await this._transport.createBidirectionalStream();
    } catch {
      this.close();
      return;
    }
    this._stream = stream;
    this._writer = this._stream.writable.getWriter();

    eventTarget.addEventListener('close', () => {
      callbacks.onClose({
        code: 4,
        reason: 'connection closed'
      });
    });

    eventTarget.addEventListener('message', (e: any) => {
      callbacks.onMessage(e.data);
    });

    this._startReading(eventTarget);

    callbacks.onOpen();
  }

  async _startReading(eventTarget: any) {
    const reader = this._stream.readable.getReader();
    let jsonStreamBuf = '';
    let jsonStreamPos = 0;
    try {
      while (true) {
        const { done, value } = await reader.read();
        if (value.length > 0) {
          jsonStreamBuf += this._utf8decoder.decode(value);
            while (jsonStreamPos < jsonStreamBuf.length) {
              if (jsonStreamBuf[jsonStreamPos] === '\n') {
                const line = jsonStreamBuf.substring(0, jsonStreamPos);
                eventTarget.dispatchEvent(new MessageEvent('message', { data: line }));
                jsonStreamBuf = jsonStreamBuf.substring(jsonStreamPos + 1);
                jsonStreamPos = 0;
              } else {
                ++jsonStreamPos;
              }
            }
        }
        if (done) {
          break;
        }
      }
    } catch {
      eventTarget.dispatchEvent(new Event('close'));
    }
  }

  async close() {
    try {
      if (this._writer) {
        await this._writer.close();
      }
      this._transport.close();
    } catch (e) {
    }
  }

  async send(data: any) {
    let binary: Uint8Array;
    binary = new TextEncoder().encode(data + '\n');
    try {
      await this._writer.write(binary);
    } catch (e) {
      this.close();
    }
  }
}
