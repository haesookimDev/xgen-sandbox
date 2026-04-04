from __future__ import annotations

import asyncio

from .transport.http import HttpTransport
from .sandbox import Sandbox
from .types import CreateSandboxOptions, SandboxInfo


class XgenClient:
    def __init__(self, api_key: str, agent_url: str) -> None:
        self._http = HttpTransport(agent_url, api_key)

    async def create_sandbox(
        self,
        template: str = "base",
        timeout_seconds: int | None = None,
        env: dict[str, str] | None = None,
        ports: list[int] | None = None,
        gui: bool | None = None,
        metadata: dict[str, str] | None = None,
    ) -> Sandbox:
        options = CreateSandboxOptions(
            template=template,
            timeout_seconds=timeout_seconds,
            env=env,
            ports=ports,
            gui=gui,
            metadata=metadata,
        )
        info = await self._http.create_sandbox(options)
        if info.status != "running":
            await self._wait_for_running(info.id, 60_000)
            info = await self._http.get_sandbox(info.id)
        return Sandbox(info, self._http)

    async def get_sandbox(self, sandbox_id: str) -> Sandbox:
        info = await self._http.get_sandbox(sandbox_id)
        return Sandbox(info, self._http)

    async def list_sandboxes(self) -> list[SandboxInfo]:
        return await self._http.list_sandboxes()

    async def close(self) -> None:
        await self._http.close()

    async def _wait_for_running(self, sandbox_id: str, timeout_ms: int) -> None:
        deadline = asyncio.get_event_loop().time() + timeout_ms / 1000
        while asyncio.get_event_loop().time() < deadline:
            info = await self._http.get_sandbox(sandbox_id)
            if info.status == "running":
                return
            if info.status in ("error", "stopped"):
                raise RuntimeError(
                    f"Sandbox {sandbox_id} entered {info.status} state"
                )
            await asyncio.sleep(1)
        raise TimeoutError(
            f"Sandbox {sandbox_id} did not become ready within {timeout_ms}ms"
        )

    async def __aenter__(self) -> XgenClient:
        return self

    async def __aexit__(self, *exc) -> None:
        await self.close()
