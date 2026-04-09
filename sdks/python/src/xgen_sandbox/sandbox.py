from __future__ import annotations

import asyncio
import time
import struct
from typing import AsyncIterator, Callable

from .protocol.codec import MsgType, Envelope, encode_payload, decode_payload
from .transport.http import HttpTransport
from .transport.ws import WsTransport
from .types import (
    SandboxInfo,
    SandboxStatus,
    ExecOptions,
    ExecResult,
    ExecEvent,
    FileInfo,
    FileEvent,
    Disposable,
)


class Sandbox:
    def __init__(self, info: SandboxInfo, http: HttpTransport) -> None:
        self.id = info.id
        self.info = info
        self._http = http
        self._ws: WsTransport | None = None
        self._status: SandboxStatus = info.status

    @property
    def status(self) -> SandboxStatus:
        return self._status

    @property
    def preview_urls(self) -> dict[int, str]:
        return self.info.preview_urls

    def get_preview_url(self, port: int) -> str | None:
        return self.info.preview_urls.get(port)

    # -- WebSocket --

    async def _ensure_ws(self) -> WsTransport:
        if self._ws is not None:
            return self._ws
        ws_url = self._http.get_ws_url(self.id)
        token = self._http.get_token()
        if token is None:
            raise RuntimeError("No auth token available")
        self._ws = WsTransport(ws_url, token)
        await self._ws.connect()
        return self._ws

    # -- Exec (REST) --

    async def exec(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        cwd: str | None = None,
        timeout: int | None = None,
    ) -> ExecResult:
        return await self._http.exec_command(
            self.id,
            command=command,
            args=args,
            env=env,
            cwd=cwd,
            timeout_seconds=timeout,
        )

    # -- Exec Stream (WebSocket) --

    async def exec_stream(
        self,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        cwd: str | None = None,
    ) -> AsyncIterator[ExecEvent]:
        """Stream execution events (stdout, stderr, exit) as an async iterator."""
        ws = await self._ensure_ws()
        channel = int(time.time() * 1000) & 0xFFFFFFFF
        queue: asyncio.Queue[ExecEvent | None] = asyncio.Queue()
        cleanups: list[Callable[[], None]] = []

        def on_stdout(env: Envelope) -> None:
            if env.channel == channel:
                queue.put_nowait(ExecEvent(type="stdout", data=env.payload.decode("utf-8", errors="replace")))

        def on_stderr(env: Envelope) -> None:
            if env.channel == channel:
                queue.put_nowait(ExecEvent(type="stderr", data=env.payload.decode("utf-8", errors="replace")))

        def on_exit(env: Envelope) -> None:
            if env.channel == channel:
                data = decode_payload(env.payload)
                queue.put_nowait(ExecEvent(type="exit", exit_code=data.get("exitCode", -1)))
                queue.put_nowait(None)  # sentinel

        def on_error(env: Envelope) -> None:
            if env.channel == channel:
                queue.put_nowait(None)

        cleanups.append(ws.on(MsgType.ExecStdout, on_stdout))
        cleanups.append(ws.on(MsgType.ExecStderr, on_stderr))
        cleanups.append(ws.on(MsgType.ExecExit, on_exit))
        cleanups.append(ws.on(MsgType.Error, on_error))

        try:
            payload = encode_payload({
                "command": command,
                "args": args or [],
                "env": env or {},
                "cwd": cwd or "",
                "tty": False,
            })
            await ws.send_async(Envelope(
                type=MsgType.ExecStart, channel=channel, id=0, payload=payload,
            ))

            while True:
                event = await queue.get()
                if event is None:
                    break
                yield event
        finally:
            for cleanup in cleanups:
                cleanup()

    # -- Terminal (WebSocket) --

    async def open_terminal(
        self,
        cols: int = 80,
        rows: int = 24,
        env: dict[str, str] | None = None,
        cwd: str | None = None,
    ) -> Terminal:
        """Open an interactive terminal session."""
        ws = await self._ensure_ws()
        channel = int(time.time() * 1000) & 0xFFFFFFFF

        payload = encode_payload({
            "command": "/bin/bash",
            "args": [],
            "env": env or {},
            "cwd": cwd or "",
            "tty": True,
            "cols": cols,
            "rows": rows,
        })
        await ws.send_async(Envelope(
            type=MsgType.ExecStart, channel=channel, id=0, payload=payload,
        ))

        return Terminal(ws, channel)

    # -- Filesystem (WebSocket) --

    async def read_file(self, path: str) -> bytes:
        ws = await self._ensure_ws()
        payload = encode_payload({"path": path})
        resp = await ws.request(MsgType.FsRead, 0, payload)
        return resp.payload

    async def read_text_file(self, path: str) -> str:
        data = await self.read_file(path)
        return data.decode("utf-8")

    async def write_file(self, path: str, content: bytes | str) -> None:
        ws = await self._ensure_ws()
        if isinstance(content, str):
            content = content.encode("utf-8")
        payload = encode_payload({"path": path, "content": content})
        await ws.request(MsgType.FsWrite, 0, payload)

    async def list_dir(self, path: str) -> list[FileInfo]:
        ws = await self._ensure_ws()
        payload = encode_payload({"path": path})
        resp = await ws.request(MsgType.FsList, 0, payload)
        items = decode_payload(resp.payload)
        return [
            FileInfo(
                name=item["name"],
                size=item["size"],
                is_dir=item["isDir"],
                mod_time=item["modTime"],
            )
            for item in items
        ]

    async def remove_file(self, path: str, recursive: bool = False) -> None:
        ws = await self._ensure_ws()
        payload = encode_payload({"path": path, "recursive": recursive})
        await ws.request(MsgType.FsRemove, 0, payload)

    # -- File watching (WebSocket event subscription) --

    def watch_files(
        self, path: str, callback: Callable[[FileEvent], None]
    ) -> Disposable:
        import asyncio

        disposed = False
        event_cleanup: Callable[[], None] | None = None

        async def _setup() -> None:
            nonlocal event_cleanup
            if disposed:
                return
            ws = await self._ensure_ws()

            def _on_event(env) -> None:
                event_data = decode_payload(env.payload)
                callback(FileEvent(path=event_data["path"], type=event_data["type"]))

            event_cleanup = ws.on(MsgType.FsEvent, _on_event)

            payload = encode_payload({"path": path})
            await ws.send_async(
                Envelope(type=MsgType.FsWatch, channel=0, id=0, payload=payload)
            )

        asyncio.ensure_future(_setup())

        def _dispose() -> None:
            nonlocal disposed, event_cleanup
            disposed = True
            if event_cleanup:
                event_cleanup()
                event_cleanup = None

        return Disposable(_dispose)

    # -- Port events (WebSocket event subscription) --

    def on_port_open(self, callback: Callable[[int], None]) -> Disposable:
        import asyncio

        disposed = False
        port_cleanup: Callable[[], None] | None = None

        async def _setup() -> None:
            nonlocal port_cleanup
            if disposed:
                return
            ws = await self._ensure_ws()

            def _on_port(env) -> None:
                data = decode_payload(env.payload)
                callback(data["port"])

            port_cleanup = ws.on(MsgType.PortOpen, _on_port)

        asyncio.ensure_future(_setup())

        def _dispose() -> None:
            nonlocal disposed, port_cleanup
            disposed = True
            if port_cleanup:
                port_cleanup()
                port_cleanup = None

        return Disposable(_dispose)

    # -- Lifecycle --

    async def keep_alive(self) -> None:
        await self._http.keep_alive(self.id)

    async def destroy(self) -> None:
        if self._ws:
            self._ws.close()
            self._ws = None
        await self._http.delete_sandbox(self.id)
        self._status = "stopped"


class Terminal:
    """Interactive terminal session over WebSocket."""

    def __init__(self, ws: WsTransport, channel: int) -> None:
        self._ws = ws
        self._channel = channel
        self._cleanups: list[Callable[[], None]] = []

    def write(self, data: str) -> None:
        """Send data to terminal stdin."""
        text_bytes = data.encode("utf-8")
        payload = struct.pack(">I", 0) + text_bytes
        self._ws.send(Envelope(
            type=MsgType.ExecStdin, channel=self._channel, id=0, payload=payload,
        ))

    def on_data(self, callback: Callable[[str], None]) -> Disposable:
        """Listen for terminal output."""
        channel = self._channel

        def handler(env: Envelope) -> None:
            if env.channel == channel:
                callback(env.payload.decode("utf-8", errors="replace"))

        cleanup = self._ws.on(MsgType.ExecStdout, handler)
        self._cleanups.append(cleanup)
        return Disposable(cleanup)

    def resize(self, cols: int, rows: int) -> None:
        """Resize the terminal."""
        payload = encode_payload({"cols": cols, "rows": rows})
        self._ws.send(Envelope(
            type=MsgType.ExecResize, channel=self._channel, id=0, payload=payload,
        ))

    def close(self) -> None:
        """Close the terminal session."""
        for cleanup in self._cleanups:
            cleanup()
        self._cleanups.clear()
