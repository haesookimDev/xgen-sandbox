from __future__ import annotations

from typing import Callable

from .protocol.codec import MsgType, Envelope, encode_payload, decode_payload
from .transport.http import HttpTransport
from .transport.ws import WsTransport
from .types import (
    SandboxInfo,
    SandboxStatus,
    ExecOptions,
    ExecResult,
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
