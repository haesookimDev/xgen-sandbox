from __future__ import annotations

import time

import httpx

from ..types import CreateSandboxOptions, ExecResult, SandboxInfo


def _parse_sandbox_info(data: dict) -> SandboxInfo:
    preview_urls = data.get("preview_urls") or {}
    # Keys may come as strings from JSON; convert to int.
    preview_urls = {int(k): v for k, v in preview_urls.items()}
    return SandboxInfo(
        id=data["id"],
        status=data["status"],
        template=data.get("template", ""),
        ws_url=data.get("ws_url", ""),
        preview_urls=preview_urls,
        vnc_url=data.get("vnc_url"),
        created_at=data.get("created_at", ""),
        expires_at=data.get("expires_at", ""),
        metadata=data.get("metadata"),
        capabilities=data.get("capabilities"),
    )


class HttpTransport:
    def __init__(self, agent_url: str, api_key: str) -> None:
        self._base_url = agent_url.rstrip("/")
        self._api_key = api_key
        self._token: str | None = None
        self._token_expires_at: float = 0
        self._client = httpx.AsyncClient()

    async def close(self) -> None:
        await self._client.aclose()

    # -- auth --

    async def _ensure_token(self) -> str:
        if self._token and time.time() < self._token_expires_at - 60:
            return self._token

        resp = await self._client.post(
            f"{self._base_url}/api/v1/auth/token",
            json={"api_key": self._api_key},
        )
        resp.raise_for_status()
        data = resp.json()
        self._token = data["token"]
        # expires_at is an ISO timestamp
        from datetime import datetime, timezone

        expires_dt = datetime.fromisoformat(data["expires_at"].replace("Z", "+00:00"))
        self._token_expires_at = expires_dt.timestamp()
        return self._token  # type: ignore[return-value]

    async def _headers(self) -> dict[str, str]:
        token = await self._ensure_token()
        return {
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
        }

    def get_token(self) -> str | None:
        return self._token

    # -- sandbox CRUD --

    async def create_sandbox(self, options: CreateSandboxOptions) -> SandboxInfo:
        body: dict = {"template": options.template}
        if options.timeout_seconds is not None:
            body["timeout_seconds"] = options.timeout_seconds
        if options.env is not None:
            body["env"] = options.env
        if options.ports is not None:
            body["ports"] = options.ports
        if options.gui is not None:
            body["gui"] = options.gui
        if options.metadata is not None:
            body["metadata"] = options.metadata
        if options.capabilities is not None:
            body["capabilities"] = options.capabilities

        resp = await self._client.post(
            f"{self._base_url}/api/v1/sandboxes",
            headers=await self._headers(),
            json=body,
        )
        if not resp.is_success:
            try:
                err = resp.json().get("error", resp.text)
            except Exception:
                err = resp.text
            raise RuntimeError(f"Create sandbox failed ({resp.status_code}): {err}")
        return _parse_sandbox_info(resp.json())

    async def get_sandbox(self, sandbox_id: str) -> SandboxInfo:
        resp = await self._client.get(
            f"{self._base_url}/api/v1/sandboxes/{sandbox_id}",
            headers=await self._headers(),
        )
        if not resp.is_success:
            raise RuntimeError(f"Get sandbox failed: sandbox '{sandbox_id}' not found ({resp.status_code})")
        return _parse_sandbox_info(resp.json())

    async def list_sandboxes(self) -> list[SandboxInfo]:
        resp = await self._client.get(
            f"{self._base_url}/api/v1/sandboxes",
            headers=await self._headers(),
        )
        if not resp.is_success:
            raise RuntimeError(f"List sandboxes failed: {resp.status_code}")
        return [_parse_sandbox_info(d) for d in resp.json()]

    async def delete_sandbox(self, sandbox_id: str) -> None:
        resp = await self._client.delete(
            f"{self._base_url}/api/v1/sandboxes/{sandbox_id}",
            headers=await self._headers(),
        )
        if not resp.is_success and resp.status_code != 204:
            raise RuntimeError(f"Delete sandbox '{sandbox_id}' failed ({resp.status_code})")

    async def keep_alive(self, sandbox_id: str) -> None:
        resp = await self._client.post(
            f"{self._base_url}/api/v1/sandboxes/{sandbox_id}/keepalive",
            headers=await self._headers(),
        )
        if not resp.is_success and resp.status_code != 204:
            raise RuntimeError(f"Keepalive for sandbox '{sandbox_id}' failed ({resp.status_code})")

    async def exec_command(
        self,
        sandbox_id: str,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        cwd: str | None = None,
        timeout_seconds: int | None = None,
    ) -> ExecResult:
        body: dict = {"command": "sh", "args": ["-c", command, *(args or [])]}
        if env is not None:
            body["env"] = env
        if cwd is not None:
            body["cwd"] = cwd
        if timeout_seconds is not None:
            body["timeout_seconds"] = timeout_seconds

        resp = await self._client.post(
            f"{self._base_url}/api/v1/sandboxes/{sandbox_id}/exec",
            headers=await self._headers(),
            json=body,
            timeout=max(30, (timeout_seconds or 30) + 5),
        )
        if not resp.is_success:
            raise RuntimeError(f"Exec failed on sandbox '{sandbox_id}' ({resp.status_code}): {resp.text}")
        data = resp.json()
        return ExecResult(
            exit_code=data["exit_code"],
            stdout=data.get("stdout", ""),
            stderr=data.get("stderr", ""),
        )

    def get_ws_url(self, sandbox_id: str) -> str:
        ws_base = self._base_url.replace("https://", "wss://").replace(
            "http://", "ws://"
        )
        return f"{ws_base}/api/v1/sandboxes/{sandbox_id}/ws"
