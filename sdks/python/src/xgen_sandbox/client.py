from __future__ import annotations

import asyncio

from .transport.http import HttpTransport
from .sandbox import Sandbox
from .types import CreateSandboxOptions, SandboxInfo


class XgenClient:
    """Client for the xgen-sandbox API.

    Manages sandbox lifecycle: creation, retrieval, and listing.
    Use as an async context manager to ensure proper cleanup.

    Example::

        async with XgenClient(api_key="my-key", agent_url="http://localhost:8080") as client:
            sandbox = await client.create_sandbox(template="nodejs")
            result = await sandbox.exec("node -v")
            print(result.stdout)
            await sandbox.destroy()

    Args:
        api_key: API key for authentication with the xgen-sandbox agent.
        agent_url: Base URL of the xgen-sandbox agent (e.g. ``"http://localhost:8080"``).
    """

    def __init__(self, api_key: str, agent_url: str, api_version: str = "v2") -> None:
        self._http = HttpTransport(agent_url, api_key, api_version)

    async def create_sandbox(
        self,
        template: str = "base",
        timeout_seconds: int | None = None,
        timeout_ms: int | None = None,
        env: dict[str, str] | None = None,
        ports: list[int] | None = None,
        gui: bool | None = None,
        metadata: dict[str, str] | None = None,
    ) -> Sandbox:
        """Create a new sandbox and wait for it to become ready.

        Args:
            template: Runtime template (e.g. ``"base"``, ``"nodejs"``, ``"python"``, ``"gui"``).
            timeout_seconds: Sandbox auto-destroy timeout in seconds.
            env: Environment variables injected into the sandbox.
            ports: Ports to expose via preview URLs.
            gui: Enable GUI (VNC) desktop environment.
            metadata: Arbitrary key-value metadata.

        Returns:
            A :class:`~xgen_sandbox.Sandbox` instance in "running" state.

        Raises:
            TimeoutError: If the sandbox does not become ready within 60 seconds.
            RuntimeError: If the sandbox enters an error or stopped state.
        """
        options = CreateSandboxOptions(
            template=template,
            timeout_seconds=timeout_seconds,
            timeout_ms=timeout_ms,
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
        """Get an existing sandbox by ID.

        Args:
            sandbox_id: The unique sandbox identifier.

        Returns:
            A :class:`~xgen_sandbox.Sandbox` instance.

        Raises:
            RuntimeError: If the sandbox does not exist.
        """
        info = await self._http.get_sandbox(sandbox_id)
        return Sandbox(info, self._http)

    async def list_sandboxes(self) -> list[SandboxInfo]:
        """List all active sandboxes.

        Returns:
            A list of :class:`~xgen_sandbox.SandboxInfo` objects.
        """
        return await self._http.list_sandboxes()

    async def close(self) -> None:
        """Close the HTTP client and release resources."""
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
