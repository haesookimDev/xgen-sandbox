from __future__ import annotations

from dataclasses import dataclass, field
from typing import Literal


SandboxStatus = Literal["starting", "running", "stopping", "stopped", "error"]
"""Lifecycle status of a sandbox."""


@dataclass
class SandboxInfo:
    """Runtime information about a sandbox instance."""

    id: str
    """Unique sandbox identifier."""
    status: SandboxStatus
    """Current lifecycle status."""
    template: str
    """Runtime template used to create this sandbox."""
    ws_url: str
    """WebSocket URL for real-time communication with the sandbox sidecar."""
    preview_urls: dict[int, str] = field(default_factory=dict)
    """Map of exposed port numbers to their public preview URLs."""
    vnc_url: str | None = None
    """VNC URL for GUI sandboxes. Only present when ``gui=True``."""
    created_at: str = ""
    """ISO 8601 timestamp of when the sandbox was created."""
    expires_at: str = ""
    """ISO 8601 timestamp of when the sandbox will expire."""
    created_at_ms: int | None = None
    """Unix epoch milliseconds when the sandbox was created. Preferred for API v2."""
    expires_at_ms: int | None = None
    """Unix epoch milliseconds when the sandbox will expire. Preferred for API v2."""
    metadata: dict[str, str] | None = None
    """User-defined metadata."""
    capabilities: list[str] | None = None
    """Active runtime capabilities for this sandbox."""
    from_warm_pool: bool | None = None
    """True when this sandbox was claimed from the warm pool. API v2 only."""


@dataclass
class CreateSandboxOptions:
    """Options for creating a new sandbox."""

    template: str = "base"
    """Runtime template (e.g. ``"base"``, ``"nodejs"``, ``"python"``, ``"gui"``)."""
    timeout_seconds: int | None = None
    """Sandbox timeout in seconds. Automatically destroyed after this duration."""
    timeout_ms: int | None = None
    """Sandbox timeout in milliseconds. Preferred for API v2."""
    env: dict[str, str] | None = None
    """Environment variables injected into the sandbox runtime."""
    ports: list[int] | None = None
    """Ports to expose via preview URLs."""
    gui: bool | None = None
    """Enable GUI (VNC) desktop environment."""
    metadata: dict[str, str] | None = None
    """Arbitrary key-value metadata attached to the sandbox."""
    capabilities: list[str] | None = None
    """Runtime capabilities: ``"sudo"``, ``"git-ssh"``, ``"browser"``."""


@dataclass
class ExecOptions:
    """Options for command execution."""

    args: list[str] | None = None
    """Additional arguments appended after the command."""
    env: dict[str, str] | None = None
    """Environment variables for the command."""
    cwd: str | None = None
    """Working directory. Defaults to ``/home/sandbox/workspace``."""
    timeout: int | None = None
    """Timeout in seconds. Defaults to 30."""


@dataclass
class ExecResult:
    """Result of a synchronous command execution."""

    exit_code: int
    """Process exit code. 0 indicates success."""
    stdout: str
    """Captured standard output."""
    stderr: str
    """Captured standard error."""


@dataclass
class StructuredError:
    """Stable machine-readable error returned by API v2."""

    code: str
    message: str
    retryable: bool
    details: dict | None = None
    request_id: str | None = None
    sandbox_id: str | None = None
    command_id: str | None = None


@dataclass
class ExecEvent:
    """A streaming event emitted during command execution."""

    type: Literal["stdout", "stderr", "exit"]
    """Event type."""
    data: str | None = None
    """Output data. Present for ``stdout`` and ``stderr`` events."""
    exit_code: int | None = None
    """Process exit code. Present for ``exit`` events."""


@dataclass
class FileInfo:
    """Metadata about a file or directory entry."""

    name: str
    """File or directory name (not the full path)."""
    size: int
    """File size in bytes."""
    is_dir: bool
    """True if this entry is a directory."""
    mod_time: int
    """Last modification time as a Unix timestamp (seconds)."""


@dataclass
class FileEvent:
    """A file system change event."""

    path: str
    """Path of the changed file relative to the workspace root."""
    type: Literal["created", "modified", "deleted"]
    """Type of change that occurred."""


class Disposable:
    """A handle that can be disposed to unsubscribe from events or release resources.

    Example::

        watcher = sandbox.watch_files(".", my_callback)
        # ... later ...
        watcher.dispose()  # Stop watching
    """

    def __init__(self, dispose_fn: callable) -> None:
        self._dispose_fn = dispose_fn
        self._disposed = False

    def dispose(self) -> None:
        """Stop listening and release associated resources."""
        if not self._disposed:
            self._disposed = True
            self._dispose_fn()
