from __future__ import annotations

import struct
from dataclasses import dataclass
from typing import Any

import msgpack


HEADER_SIZE = 9
_HEADER_FMT = ">BII"  # type(1 byte) + channel(4 bytes BE) + id(4 bytes BE)


class MsgType:
    Ping = 0x01
    Pong = 0x02
    Error = 0x03
    Ack = 0x04
    ExecStart = 0x20
    ExecStdin = 0x21
    ExecStdout = 0x22
    ExecStderr = 0x23
    ExecExit = 0x24
    ExecSignal = 0x25
    ExecResize = 0x26
    FsRead = 0x30
    FsWrite = 0x31
    FsList = 0x32
    FsRemove = 0x33
    FsWatch = 0x34
    FsEvent = 0x35
    PortOpen = 0x40
    PortClose = 0x41
    SandboxReady = 0x50
    SandboxError = 0x51
    SandboxStats = 0x52


@dataclass
class Envelope:
    type: int
    channel: int
    id: int
    payload: bytes


def encode_envelope(env: Envelope) -> bytes:
    header = struct.pack(_HEADER_FMT, env.type, env.channel, env.id)
    return header + env.payload


def decode_envelope(data: bytes | bytearray | memoryview) -> Envelope:
    if len(data) < HEADER_SIZE:
        raise ValueError(f"Message too short: {len(data)} bytes")
    msg_type, channel, msg_id = struct.unpack_from(_HEADER_FMT, data)
    payload = bytes(data[HEADER_SIZE:])
    return Envelope(type=msg_type, channel=channel, id=msg_id, payload=payload)


def encode_payload(value: Any) -> bytes:
    return msgpack.packb(value, use_bin_type=True)


def decode_payload(data: bytes) -> Any:
    return msgpack.unpackb(data, raw=False)


def create_envelope(
    msg_type: int,
    channel: int,
    msg_id: int,
    payload: Any | None = None,
) -> bytes:
    payload_bytes = encode_payload(payload) if payload is not None else b""
    return encode_envelope(
        Envelope(type=msg_type, channel=channel, id=msg_id, payload=payload_bytes)
    )
