use std::sync::Arc;

use crate::error::Error;
use crate::sandbox::Sandbox;
use crate::transport::http::HttpTransport;
use crate::types::{CreateSandboxOptions, SandboxInfo, SandboxStatus};

pub struct XgenClient {
    http: Arc<HttpTransport>,
}

#[derive(Debug, Clone, Default)]
pub struct ClientOptions {
    pub api_version: Option<String>,
}

impl XgenClient {
    /// Create a new client with the given API key and agent URL.
    pub fn new(api_key: &str, agent_url: &str) -> Self {
        Self::new_with_options(api_key, agent_url, ClientOptions::default())
    }

    /// Create a new client with explicit options. API v2 is the default.
    pub fn new_with_options(api_key: &str, agent_url: &str, options: ClientOptions) -> Self {
        let api_version = options.api_version.as_deref().unwrap_or("v2");
        Self {
            http: Arc::new(HttpTransport::new_with_version(agent_url, api_key, api_version)),
        }
    }

    /// Create a new sandbox and return a Sandbox instance.
    /// Waits for the sandbox to reach "running" status.
    pub async fn create_sandbox(
        &self,
        options: CreateSandboxOptions,
    ) -> Result<Sandbox, Error> {
        let info = self.http.create_sandbox(&options).await?;

        if info.status != SandboxStatus::Running {
            self.wait_for_running(&info.id, 60_000).await?;
            let updated = self.http.get_sandbox(&info.id).await?;
            return Ok(Sandbox::new(updated, self.http.clone()));
        }

        Ok(Sandbox::new(info, self.http.clone()))
    }

    /// Get an existing sandbox by ID, returning a Sandbox handle.
    pub async fn get_sandbox(&self, id: &str) -> Result<Sandbox, Error> {
        let info = self.http.get_sandbox(id).await?;
        Ok(Sandbox::new(info, self.http.clone()))
    }

    /// List all sandboxes.
    pub async fn list_sandboxes(&self) -> Result<Vec<SandboxInfo>, Error> {
        self.http.list_sandboxes().await
    }

    async fn wait_for_running(&self, id: &str, timeout_ms: u64) -> Result<(), Error> {
        let start = tokio::time::Instant::now();
        let timeout = std::time::Duration::from_millis(timeout_ms);

        loop {
            let info = self.http.get_sandbox(id).await?;
            match info.status {
                SandboxStatus::Running => return Ok(()),
                SandboxStatus::Error | SandboxStatus::Stopped => {
                    return Err(Error::Api {
                        status: 0,
                        message: format!("Sandbox {id} entered {:?} state", info.status),
                    });
                }
                _ => {}
            }

            if start.elapsed() >= timeout {
                return Err(Error::Timeout(format!(
                    "Sandbox {id} did not become ready within {timeout_ms}ms"
                )));
            }

            tokio::time::sleep(std::time::Duration::from_secs(1)).await;
        }
    }
}
