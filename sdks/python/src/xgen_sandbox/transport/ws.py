from __future__ import annotations

import asyncio
from typing import Callable
from urllib.parse import urlencode

import websockets
import websockets.asyncio.client

from ..protocol.codec import (
    MsgType,
    Envelope,
    encode_envelope,
    decode_envelope,
)

MessageHandler = Callable[[Envelope], None]


class WsTransport:
    def __init__(self, url: str, token: str, max_reconnect_attempts: int = 5) -> None:
        self._url = url
        self._token = token
        self._ws: websockets.asyncio.client.ClientConnection | None = None
        self._handlers: dict[int, list[MessageHandler]] = {}
        self._pending: dict[int, asyncio.Future[Envelope]] = {}
        self._next_id = 1
        self._connected = False
        self._recv_task: asyncio.Task | None = None
        self._intentionally_closed = False
        self._reconnect_attempts = 0
        self._max_reconnect_attempts = max_reconnect_attempts

    async def connect(self) -> None:
        if self._connected:
            return
        ws_url = f"{self._url}?{urlencode({'token': self._token})}"
        self._ws = await websockets.asyncio.client.connect(ws_url)
        self._connected = True
        self._reconnect_attempts = 0
        self._intentionally_closed = False
        self._recv_task = asyncio.get_event_loop().create_task(self._recv_loop())

    def close(self) -> None:
        self._intentionally_closed = True
        self._connected = False
        if self._recv_task and not self._recv_task.done():
            self._recv_task.cancel()
        if self._ws:
            asyncio.get_event_loop().create_task(self._ws.close())
            self._ws = None
        # Reject pending requests
        for fut in self._pending.values():
            if not fut.done():
                fut.set_exception(ConnectionError("Connection closed"))
        self._pending.clear()

    def send(self, envelope: Envelope) -> None:
        if not self._ws or not self._connected:
            raise ConnectionError("Not connected")
        data = encode_envelope(envelope)
        asyncio.get_event_loop().create_task(self._ws.send(data))

    async def send_async(self, envelope: Envelope) -> None:
        if not self._ws or not self._connected:
            raise ConnectionError("Not connected")
        data = encode_envelope(envelope)
        await self._ws.send(data)

    async def request(
        self,
        msg_type: int,
        channel: int,
        payload: bytes,
        timeout: float = 30.0,
    ) -> Envelope:
        msg_id = self._next_id
        self._next_id += 1

        loop = asyncio.get_event_loop()
        fut: asyncio.Future[Envelope] = loop.create_future()
        self._pending[msg_id] = fut

        await self.send_async(
            Envelope(type=msg_type, channel=channel, id=msg_id, payload=payload)
        )

        try:
            return await asyncio.wait_for(fut, timeout=timeout)
        except asyncio.TimeoutError:
            self._pending.pop(msg_id, None)
            raise TimeoutError(f"Request timeout (id={msg_id})")

    def on(self, msg_type: int, handler: MessageHandler) -> Callable[[], None]:
        handlers = self._handlers.setdefault(msg_type, [])
        handlers.append(handler)

        def cleanup() -> None:
            try:
                handlers.remove(handler)
            except ValueError:
                pass

        return cleanup

    # -- internal --

    async def _recv_loop(self) -> None:
        assert self._ws is not None
        try:
            async for message in self._ws:
                if isinstance(message, str):
                    continue
                self._handle_message(message)
        except websockets.exceptions.ConnectionClosed:
            pass
        except asyncio.CancelledError:
            return  # intentional cancellation, don't reconnect
        finally:
            self._connected = False
            asyncio.ensure_future(self._attempt_reconnect())

    def _handle_message(self, data: bytes) -> None:
        try:
            envelope = decode_envelope(data)
        except Exception:
            return

        # Respond to pings
        if envelope.type == MsgType.Ping:
            self.send(
                Envelope(type=MsgType.Pong, channel=0, id=envelope.id, payload=b"")
            )
            return

        # Check pending requests by id
        if envelope.id > 0:
            fut = self._pending.pop(envelope.id, None)
            if fut is not None and not fut.done():
                if envelope.type == MsgType.Error:
                    fut.set_exception(RuntimeError("Server error"))
                else:
                    fut.set_result(envelope)
                return

        # Dispatch to type handlers
        handlers = self._handlers.get(envelope.type)
        if handlers:
            for handler in list(handlers):
                handler(envelope)

    async def _attempt_reconnect(self) -> None:
        if self._intentionally_closed:
            return
        if self._reconnect_attempts >= self._max_reconnect_attempts:
            return

        self._reconnect_attempts += 1
        delay = min(2 ** (self._reconnect_attempts - 1), 30)
        await asyncio.sleep(delay)

        if self._intentionally_closed or self._connected:
            return

        try:
            await self.connect()
        except Exception:
            pass  # connect failure triggers recv_loop exit which retries
