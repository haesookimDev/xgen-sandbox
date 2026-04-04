from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal


SandboxStatus = Literal["starting", "running", "stopping", "stopped", "error"]


@dataclass
class SandboxInfo:
    id: str
    status: SandboxStatus
    template: str
    ws_url: str
    preview_urls: dict[int, str] = field(default_factory=dict)
    vnc_url: str | None = None
    created_at: str = ""
    expires_at: str = ""
    metadata: dict[str, str] | None = None


@dataclass
class CreateSandboxOptions:
    template: str = "base"
    timeout_seconds: int | None = None
    env: dict[str, str] | None = None
    ports: list[int] | None = None
    gui: bool | None = None
    metadata: dict[str, str] | None = None


@dataclass
class ExecOptions:
    args: list[str] | None = None
    env: dict[str, str] | None = None
    cwd: str | None = None
    timeout: int | None = None


@dataclass
class ExecResult:
    exit_code: int
    stdout: str
    stderr: str


@dataclass
class ExecEvent:
    type: Literal["stdout", "stderr", "exit"]
    data: str | None = None
    exit_code: int | None = None


@dataclass
class FileInfo:
    name: str
    size: int
    is_dir: bool
    mod_time: int


@dataclass
class FileEvent:
    path: str
    type: Literal["created", "modified", "deleted"]


class Disposable:
    """A handle that can be disposed to clean up resources."""

    def __init__(self, dispose_fn: callable) -> None:
        self._dispose_fn = dispose_fn
        self._disposed = False

    def dispose(self) -> None:
        if not self._disposed:
            self._disposed = True
            self._dispose_fn()
