import { encode, decode } from "@msgpack/msgpack";

export const HEADER_SIZE = 9;

export const MsgType = {
  Ping: 0x01,
  Pong: 0x02,
  Error: 0x03,
  Ack: 0x04,
  ExecStart: 0x20,
  ExecStdin: 0x21,
  ExecStdout: 0x22,
  ExecStderr: 0x23,
  ExecExit: 0x24,
  ExecSignal: 0x25,
  ExecResize: 0x26,
  FsRead: 0x30,
  FsWrite: 0x31,
  FsList: 0x32,
  FsRemove: 0x33,
  FsWatch: 0x34,
  FsEvent: 0x35,
  PortOpen: 0x40,
  PortClose: 0x41,
  SandboxReady: 0x50,
  SandboxError: 0x51,
  SandboxStats: 0x52,
} as const;

export interface Envelope {
  type: number;
  channel: number;
  id: number;
  payload: Uint8Array;
}

export function encodeEnvelope(env: Envelope): Uint8Array {
  const buf = new Uint8Array(HEADER_SIZE + env.payload.length);
  const view = new DataView(buf.buffer);
  buf[0] = env.type;
  view.setUint32(1, env.channel, false);
  view.setUint32(5, env.id, false);
  buf.set(env.payload, HEADER_SIZE);
  return buf;
}

export function decodeEnvelope(data: Uint8Array): Envelope {
  if (data.length < HEADER_SIZE) {
    throw new Error(`Message too short: ${data.length} bytes`);
  }
  const view = new DataView(data.buffer, data.byteOffset);
  return {
    type: data[0],
    channel: view.getUint32(1, false),
    id: view.getUint32(5, false),
    payload: data.slice(HEADER_SIZE),
  };
}

export function encodePayload(value: unknown): Uint8Array {
  return encode(value) as Uint8Array;
}

export function decodePayload<T = unknown>(data: Uint8Array): T {
  return decode(data) as T;
}

export function createEnvelope(
  type: number,
  channel: number,
  id: number,
  payload?: unknown
): Uint8Array {
  const payloadBytes = payload ? encodePayload(payload) : new Uint8Array(0);
  return encodeEnvelope({ type, channel, id, payload: payloadBytes });
}
