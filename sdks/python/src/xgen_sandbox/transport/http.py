from __future__ import annotations

import time

import httpx

from ..types import CreateSandboxOptions, ExecResult, SandboxInfo


class XgenApiError(RuntimeError):
    """Structured API error with v2 fields when the server provides them."""

    def __init__(self, status_code: int, body: dict | None, fallback: str) -> None:
        body = body or {}
        self.status_code = status_code
        self.code = body.get("code")
        self.retryable = body.get("retryable")
        self.details = body.get("details")
        self.request_id = body.get("request_id")
        message = body.get("message") or body.get("error") or fallback
        super().__init__(message)


def _parse_sandbox_info(data: dict) -> SandboxInfo:
    preview_urls = data.get("preview_urls") or {}
    # Keys may come as strings from JSON; convert to int.
    preview_urls = {int(k): v for k, v in preview_urls.items()}
    created_at_ms = data.get("created_at_ms")
    expires_at_ms = data.get("expires_at_ms")
    return SandboxInfo(
        id=data["id"],
        status=data["status"],
        template=data.get("template", ""),
        ws_url=data.get("ws_url", ""),
        preview_urls=preview_urls,
        vnc_url=data.get("vnc_url"),
        created_at=data.get("created_at", ""),
        expires_at=data.get("expires_at", ""),
        created_at_ms=created_at_ms,
        expires_at_ms=expires_at_ms,
        metadata=data.get("metadata"),
        capabilities=data.get("capabilities"),
        from_warm_pool=data.get("from_warm_pool"),
    )


class HttpTransport:
    def __init__(self, agent_url: str, api_key: str, api_version: str = "v2") -> None:
        self._base_url = agent_url.rstrip("/")
        self._api_key = api_key
        if api_version not in ("v1", "v2"):
            raise ValueError("api_version must be 'v1' or 'v2'")
        self._api_version = api_version
        self._token: str | None = None
        self._token_expires_at: float = 0
        self._client = httpx.AsyncClient()

    def _path(self, suffix: str) -> str:
        return f"/api/{self._api_version}{suffix}"

    async def _raise_api_error(self, resp: httpx.Response, fallback: str) -> None:
        try:
            body = resp.json()
        except Exception:
            body = {"message": resp.text}
        raise XgenApiError(resp.status_code, body, fallback)

    async def close(self) -> None:
        await self._client.aclose()

    # -- auth --

    async def _ensure_token(self) -> str:
        if self._token and time.time() < self._token_expires_at - 60:
            return self._token

        resp = await self._client.post(
            f"{self._base_url}{self._path('/auth/token')}",
            json={"api_key": self._api_key},
        )
        if not resp.is_success:
            await self._raise_api_error(resp, "Auth failed")
        data = resp.json()
        self._token = data["token"]
        if "expires_at_ms" in data:
            self._token_expires_at = data["expires_at_ms"] / 1000
        else:
            from datetime import datetime

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
        if self._api_version == "v2":
            if options.timeout_ms is not None:
                body["timeout_ms"] = options.timeout_ms
            elif options.timeout_seconds is not None:
                body["timeout_ms"] = options.timeout_seconds * 1000
        elif options.timeout_seconds is not None:
            body["timeout_seconds"] = options.timeout_seconds
        elif options.timeout_ms is not None:
            body["timeout_seconds"] = (options.timeout_ms + 999) // 1000
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
            f"{self._base_url}{self._path('/sandboxes')}",
            headers=await self._headers(),
            json=body,
        )
        if not resp.is_success:
            await self._raise_api_error(resp, "Create sandbox failed")
        return _parse_sandbox_info(resp.json())

    async def get_sandbox(self, sandbox_id: str) -> SandboxInfo:
        resp = await self._client.get(
            f"{self._base_url}{self._path(f'/sandboxes/{sandbox_id}')}",
            headers=await self._headers(),
        )
        if not resp.is_success:
            await self._raise_api_error(resp, f"Get sandbox failed: sandbox '{sandbox_id}' not found")
        return _parse_sandbox_info(resp.json())

    async def list_sandboxes(self) -> list[SandboxInfo]:
        resp = await self._client.get(
            f"{self._base_url}{self._path('/sandboxes')}",
            headers=await self._headers(),
        )
        if not resp.is_success:
            await self._raise_api_error(resp, "List sandboxes failed")
        return [_parse_sandbox_info(d) for d in resp.json()]

    async def delete_sandbox(self, sandbox_id: str) -> None:
        resp = await self._client.delete(
            f"{self._base_url}{self._path(f'/sandboxes/{sandbox_id}')}",
            headers=await self._headers(),
        )
        if not resp.is_success and resp.status_code != 204:
            await self._raise_api_error(resp, f"Delete sandbox '{sandbox_id}' failed")

    async def keep_alive(self, sandbox_id: str) -> None:
        resp = await self._client.post(
            f"{self._base_url}{self._path(f'/sandboxes/{sandbox_id}/keepalive')}",
            headers=await self._headers(),
        )
        if not resp.is_success and resp.status_code != 204:
            await self._raise_api_error(resp, f"Keepalive for sandbox '{sandbox_id}' failed")

    async def exec_command(
        self,
        sandbox_id: str,
        command: str,
        args: list[str] | None = None,
        env: dict[str, str] | None = None,
        cwd: str | None = None,
        timeout_seconds: int | None = None,
        max_output_bytes: int | None = None,
        max_stdout_bytes: int | None = None,
        max_stderr_bytes: int | None = None,
        artifact_path: str | None = None,
    ) -> ExecResult:
        body: dict = {"command": "sh", "args": ["-c", command, *(args or [])]}
        if env is not None:
            body["env"] = env
        if cwd is not None:
            body["cwd"] = cwd
        if timeout_seconds is not None:
            if self._api_version == "v2":
                body["timeout_ms"] = timeout_seconds * 1000
            else:
                body["timeout_seconds"] = timeout_seconds
        if max_output_bytes is not None:
            body["max_output_bytes"] = max_output_bytes
        if max_stdout_bytes is not None:
            body["max_stdout_bytes"] = max_stdout_bytes
        if max_stderr_bytes is not None:
            body["max_stderr_bytes"] = max_stderr_bytes
        if artifact_path is not None:
            body["artifact_path"] = artifact_path

        resp = await self._client.post(
            f"{self._base_url}{self._path(f'/sandboxes/{sandbox_id}/exec')}",
            headers=await self._headers(),
            json=body,
            timeout=max(30, (timeout_seconds or 30) + 5),
        )
        if not resp.is_success:
            await self._raise_api_error(resp, f"Exec failed on sandbox '{sandbox_id}'")
        data = resp.json()
        return ExecResult(
            exit_code=data["exit_code"],
            stdout=data.get("stdout", ""),
            stderr=data.get("stderr", ""),
            truncated=data.get("truncated", False),
            stdout_truncated=data.get("stdout_truncated", False),
            stderr_truncated=data.get("stderr_truncated", False),
            truncation_marker=data.get("truncation_marker"),
            artifact_path=data.get("artifact_path"),
        )

    def get_ws_url(self, sandbox_id: str) -> str:
        ws_base = self._base_url.replace("https://", "wss://").replace(
            "http://", "ws://"
        )
        return f"{ws_base}{self._path(f'/sandboxes/{sandbox_id}/ws')}"
