use std::sync::Arc;

use tokio::sync::Mutex;

use crate::error::Error;
use crate::protocol::{decode_payload, encode_payload, msg_type, Envelope};
use crate::transport::http::HttpTransport;
use crate::transport::ws::WsTransport;
use crate::types::{ExecOptions, ExecResult, FileEvent, FileInfo, SandboxInfo, SandboxStatus};

/// Handle returned by watch/event subscriptions. Dropping it cancels the subscription.
pub struct WatchHandle {
    ws: Arc<WsTransport>,
    msg_type: u8,
    handler_id: u64,
}

impl WatchHandle {
    /// Explicitly cancel the subscription.
    pub async fn cancel(self) {
        self.ws.off(self.msg_type, self.handler_id).await;
    }
}

pub struct Sandbox {
    pub id: String,
    pub info: SandboxInfo,
    http: Arc<HttpTransport>,
    ws: Mutex<Option<Arc<WsTransport>>>,
    status: Mutex<SandboxStatus>,
}

impl Sandbox {
    pub(crate) fn new(info: SandboxInfo, http: Arc<HttpTransport>) -> Self {
        let status = info.status.clone();
        let id = info.id.clone();
        Self {
            id,
            info,
            http,
            ws: Mutex::new(None),
            status: Mutex::new(status),
        }
    }

    pub async fn status(&self) -> SandboxStatus {
        self.status.lock().await.clone()
    }

    /// Get the preview URL for a specific port.
    pub fn get_preview_url(&self, port: u16) -> Option<&String> {
        self.info.preview_urls.get(&port.to_string())
    }

    /// Ensure WebSocket connection is established.
    async fn ensure_ws(&self) -> Result<Arc<WsTransport>, Error> {
        let mut guard = self.ws.lock().await;
        if let Some(ref ws) = *guard {
            return Ok(ws.clone());
        }

        let ws_url = self.http.ws_url(&self.id);
        let token = self
            .http
            .get_token()
            .await
            .ok_or_else(|| Error::Auth("No token available".to_string()))?;

        let ws = WsTransport::connect(&ws_url, &token).await?;
        *guard = Some(ws.clone());
        Ok(ws)
    }

    /// Execute a command via REST and wait for completion.
    pub async fn exec(
        &self,
        command: &str,
        options: Option<ExecOptions>,
    ) -> Result<ExecResult, Error> {
        let opts = options.unwrap_or_default();
        self.http.exec(&self.id, command, &opts).await
    }

    /// Read a file as raw bytes via WebSocket.
    pub async fn read_file(&self, path: &str) -> Result<Vec<u8>, Error> {
        let ws = self.ensure_ws().await?;

        #[derive(serde::Serialize)]
        struct Req<'a> {
            path: &'a str,
        }

        let payload = encode_payload(&Req { path })?;
        let resp = ws
            .request(msg_type::FS_READ, 0, payload, 30_000)
            .await?;
        Ok(resp.payload)
    }

    /// Read a file as a UTF-8 string via WebSocket.
    pub async fn read_text_file(&self, path: &str) -> Result<String, Error> {
        let data = self.read_file(path).await?;
        String::from_utf8(data)
            .map_err(|e| Error::Protocol(format!("Invalid UTF-8: {e}")))
    }

    /// Write a file via WebSocket.
    pub async fn write_file(&self, path: &str, content: &[u8]) -> Result<(), Error> {
        let ws = self.ensure_ws().await?;

        #[derive(serde::Serialize)]
        struct Req<'a> {
            path: &'a str,
            #[serde(with = "serde_bytes")]
            content: &'a [u8],
        }

        let payload = encode_payload(&Req { path, content })?;
        ws.request(msg_type::FS_WRITE, 0, payload, 30_000).await?;
        Ok(())
    }

    /// List directory entries via WebSocket.
    pub async fn list_dir(&self, path: &str) -> Result<Vec<FileInfo>, Error> {
        let ws = self.ensure_ws().await?;

        #[derive(serde::Serialize)]
        struct Req<'a> {
            path: &'a str,
        }

        let payload = encode_payload(&Req { path })?;
        let resp = ws
            .request(msg_type::FS_LIST, 0, payload, 30_000)
            .await?;
        decode_payload(&resp.payload)
    }

    /// Remove a file or directory via WebSocket.
    pub async fn remove_file(&self, path: &str, recursive: bool) -> Result<(), Error> {
        let ws = self.ensure_ws().await?;

        #[derive(serde::Serialize)]
        struct Req<'a> {
            path: &'a str,
            recursive: bool,
        }

        let payload = encode_payload(&Req { path, recursive })?;
        ws.request(msg_type::FS_REMOVE, 0, payload, 30_000)
            .await?;
        Ok(())
    }

    /// Watch a path for file changes. Returns a WatchHandle to cancel the subscription.
    pub async fn watch_files(
        &self,
        path: &str,
        callback: impl Fn(FileEvent) + Send + Sync + 'static,
    ) -> Result<WatchHandle, Error> {
        let ws = self.ensure_ws().await?;

        let handler_id = ws
            .on(msg_type::FS_EVENT, move |env: &Envelope| {
                if let Ok(event) = decode_payload::<FileEvent>(&env.payload) {
                    callback(event);
                }
            })
            .await;

        // Send watch request
        #[derive(serde::Serialize)]
        struct Req<'a> {
            path: &'a str,
        }

        let payload = encode_payload(&Req { path })?;
        ws.send(&Envelope {
            msg_type: msg_type::FS_WATCH,
            channel: 0,
            id: 0,
            payload,
        })
        .await?;

        Ok(WatchHandle {
            ws,
            msg_type: msg_type::FS_EVENT,
            handler_id,
        })
    }

    /// Register a callback for port-open events. Returns a WatchHandle to cancel.
    pub async fn on_port_open(
        &self,
        callback: impl Fn(u16) + Send + Sync + 'static,
    ) -> Result<WatchHandle, Error> {
        let ws = self.ensure_ws().await?;

        #[derive(serde::Deserialize)]
        struct PortPayload {
            port: u16,
        }

        let handler_id = ws
            .on(msg_type::PORT_OPEN, move |env: &Envelope| {
                if let Ok(data) = decode_payload::<PortPayload>(&env.payload) {
                    callback(data.port);
                }
            })
            .await;

        Ok(WatchHandle {
            ws,
            msg_type: msg_type::PORT_OPEN,
            handler_id,
        })
    }

    /// Send a keep-alive signal via REST.
    pub async fn keep_alive(&self) -> Result<(), Error> {
        self.http.keep_alive(&self.id).await
    }

    /// Destroy the sandbox: close WS and send DELETE via REST.
    pub async fn destroy(&self) -> Result<(), Error> {
        {
            let mut guard = self.ws.lock().await;
            if let Some(ws) = guard.take() {
                ws.close().await;
            }
        }
        self.http.delete_sandbox(&self.id).await?;
        *self.status.lock().await = SandboxStatus::Stopped;
        Ok(())
    }
}
