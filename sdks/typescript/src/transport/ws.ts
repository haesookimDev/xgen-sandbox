import WebSocket from "ws";
import {
  type Envelope,
  decodeEnvelope,
  encodeEnvelope,
  MsgType,
} from "../protocol/codec.js";

export type MessageHandler = (envelope: Envelope) => void;

export class WsTransport {
  private ws: WebSocket | null = null;
  private url: string;
  private token: string;
  private handlers = new Map<number, MessageHandler[]>(); // type -> handlers
  private pendingRequests = new Map<
    number,
    { resolve: (env: Envelope) => void; reject: (err: Error) => void }
  >();
  private nextId = 1;
  private reconnectAttempts = 0;
  private maxReconnectAttempts = 5;
  private connected = false;
  private connectPromise: Promise<void> | null = null;
  private intentionallyClosed = false;

  constructor(url: string, token: string) {
    this.url = url;
    this.token = token;
  }

  async connect(): Promise<void> {
    if (this.connected) return;
    if (this.connectPromise) return this.connectPromise;

    this.connectPromise = new Promise<void>((resolve, reject) => {
      const wsUrl = `${this.url}?token=${encodeURIComponent(this.token)}`;
      this.ws = new WebSocket(wsUrl);
      this.ws.binaryType = "arraybuffer";

      this.ws.on("open", () => {
        this.connected = true;
        this.reconnectAttempts = 0;
        this.connectPromise = null;
        resolve();
      });

      this.ws.on("message", (data: WebSocket.Data) => {
        const bytes =
          data instanceof ArrayBuffer
            ? new Uint8Array(data)
            : new Uint8Array(
                (data as Buffer).buffer,
                (data as Buffer).byteOffset,
                (data as Buffer).byteLength
              );
        this.handleMessage(bytes);
      });

      this.ws.on("close", () => {
        this.connected = false;
        this.connectPromise = null;
        this.attemptReconnect();
      });

      this.ws.on("error", (err) => {
        this.connectPromise = null;
        if (!this.connected) {
          reject(err);
        }
      });
    });

    return this.connectPromise;
  }

  close(): void {
    this.intentionallyClosed = true;
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.connected = false;

    // Reject all pending requests
    for (const [, pending] of this.pendingRequests) {
      pending.reject(new Error("Connection closed"));
    }
    this.pendingRequests.clear();
  }

  send(envelope: Envelope): void {
    if (!this.ws || !this.connected) {
      throw new Error("Not connected");
    }
    const data = encodeEnvelope(envelope);
    this.ws.send(data);
  }

  async request(
    type: number,
    channel: number,
    payload: Uint8Array,
    timeout = 30_000
  ): Promise<Envelope> {
    const id = this.nextId++;

    return new Promise<Envelope>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pendingRequests.delete(id);
        reject(new Error(`Request timeout (id=${id})`));
      }, timeout);

      this.pendingRequests.set(id, {
        resolve: (env) => {
          clearTimeout(timer);
          resolve(env);
        },
        reject: (err) => {
          clearTimeout(timer);
          reject(err);
        },
      });

      this.send({ type, channel, id, payload });
    });
  }

  on(type: number, handler: MessageHandler): () => void {
    const existing = this.handlers.get(type) ?? [];
    existing.push(handler);
    this.handlers.set(type, existing);

    return () => {
      const handlers = this.handlers.get(type);
      if (handlers) {
        const idx = handlers.indexOf(handler);
        if (idx !== -1) handlers.splice(idx, 1);
      }
    };
  }

  private handleMessage(data: Uint8Array): void {
    let envelope: Envelope;
    try {
      envelope = decodeEnvelope(data);
    } catch {
      return;
    }

    // Handle ping/pong
    if (envelope.type === MsgType.Ping) {
      this.send({
        type: MsgType.Pong,
        channel: 0,
        id: envelope.id,
        payload: new Uint8Array(0),
      });
      return;
    }

    // Check if this is a response to a pending request
    if (envelope.id > 0) {
      const pending = this.pendingRequests.get(envelope.id);
      if (pending) {
        this.pendingRequests.delete(envelope.id);
        if (envelope.type === MsgType.Error) {
          pending.reject(new Error(`Server error`));
        } else {
          pending.resolve(envelope);
        }
        return;
      }
    }

    // Dispatch to type handlers
    const handlers = this.handlers.get(envelope.type);
    if (handlers) {
      for (const handler of handlers) {
        handler(envelope);
      }
    }
  }

  private attemptReconnect(): void {
    if (this.intentionallyClosed) return;
    if (this.reconnectAttempts >= this.maxReconnectAttempts) return;

    this.reconnectAttempts++;
    const delay = Math.min(1000 * Math.pow(2, this.reconnectAttempts - 1), 30_000);

    setTimeout(() => {
      if (this.intentionallyClosed || this.connected) return;
      this.connect().catch(() => {
        // connect() failure will trigger another close event, which retries
      });
    }, delay);
  }
}
