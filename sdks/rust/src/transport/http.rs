use reqwest::Client;
use tokio::sync::Mutex;

use crate::error::Error;
use crate::types::{
    AuthRequest, AuthResponse, CreateSandboxOptions, ExecApiRequest, ExecApiResponse,
    ExecOptions, ExecResult, SandboxInfo,
};

pub struct HttpTransport {
    base_url: String,
    api_key: String,
    client: Client,
    token: Mutex<Option<TokenState>>,
}

struct TokenState {
    token: String,
    expires_at: chrono::DateTime<chrono::Utc>,
}

impl HttpTransport {
    pub fn new(agent_url: &str, api_key: &str) -> Self {
        let base_url = agent_url.trim_end_matches('/').to_string();
        Self {
            base_url,
            api_key: api_key.to_string(),
            client: Client::new(),
            token: Mutex::new(None),
        }
    }

    async fn ensure_token(&self) -> Result<String, Error> {
        let mut guard = self.token.lock().await;
        if let Some(ref state) = *guard {
            let now = chrono::Utc::now();
            if now < state.expires_at - chrono::Duration::seconds(60) {
                return Ok(state.token.clone());
            }
        }

        let resp = self
            .client
            .post(format!("{}/api/v1/auth/token", self.base_url))
            .json(&AuthRequest {
                api_key: self.api_key.clone(),
            })
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Auth(format!("{status} {text}")));
        }

        let auth: AuthResponse = resp.json().await?;
        let expires_at = chrono::DateTime::parse_from_rfc3339(&auth.expires_at)
            .map(|dt| dt.with_timezone(&chrono::Utc))
            .unwrap_or_else(|_| chrono::Utc::now() + chrono::Duration::hours(1));

        let token = auth.token.clone();
        *guard = Some(TokenState {
            token: auth.token,
            expires_at,
        });
        Ok(token)
    }

    async fn auth_headers(&self) -> Result<String, Error> {
        let token = self.ensure_token().await?;
        Ok(format!("Bearer {token}"))
    }

    pub async fn create_sandbox(&self, options: &CreateSandboxOptions) -> Result<SandboxInfo, Error> {
        let auth = self.auth_headers().await?;

        #[derive(serde::Serialize)]
        struct Body<'a> {
            template: &'a str,
            #[serde(skip_serializing_if = "Option::is_none")]
            timeout_seconds: Option<u64>,
            #[serde(skip_serializing_if = "Option::is_none")]
            resources: &'a Option<crate::types::Resources>,
            #[serde(skip_serializing_if = "Option::is_none")]
            env: &'a Option<std::collections::HashMap<String, String>>,
            #[serde(skip_serializing_if = "Option::is_none")]
            ports: &'a Option<Vec<u16>>,
            #[serde(skip_serializing_if = "Option::is_none")]
            gui: Option<bool>,
            #[serde(skip_serializing_if = "Option::is_none")]
            metadata: &'a Option<std::collections::HashMap<String, String>>,
        }

        let body = Body {
            template: options.template.as_deref().unwrap_or("base"),
            timeout_seconds: options.timeout_seconds,
            resources: &options.resources,
            env: &options.env,
            ports: &options.ports,
            gui: options.gui,
            metadata: &options.metadata,
        };

        let resp = self
            .client
            .post(format!("{}/api/v1/sandboxes", self.base_url))
            .header("Authorization", &auth)
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status,
                message: text,
            });
        }

        Ok(resp.json().await?)
    }

    pub async fn get_sandbox(&self, id: &str) -> Result<SandboxInfo, Error> {
        let auth = self.auth_headers().await?;
        let resp = self
            .client
            .get(format!("{}/api/v1/sandboxes/{id}", self.base_url))
            .header("Authorization", &auth)
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status,
                message: text,
            });
        }

        Ok(resp.json().await?)
    }

    pub async fn list_sandboxes(&self) -> Result<Vec<SandboxInfo>, Error> {
        let auth = self.auth_headers().await?;
        let resp = self
            .client
            .get(format!("{}/api/v1/sandboxes", self.base_url))
            .header("Authorization", &auth)
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status,
                message: text,
            });
        }

        Ok(resp.json().await?)
    }

    pub async fn delete_sandbox(&self, id: &str) -> Result<(), Error> {
        let auth = self.auth_headers().await?;
        let resp = self
            .client
            .delete(format!("{}/api/v1/sandboxes/{id}", self.base_url))
            .header("Authorization", &auth)
            .send()
            .await?;

        let status = resp.status();
        if !status.is_success() && status.as_u16() != 204 {
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status: status.as_u16(),
                message: text,
            });
        }

        Ok(())
    }

    pub async fn keep_alive(&self, id: &str) -> Result<(), Error> {
        let auth = self.auth_headers().await?;
        let resp = self
            .client
            .post(format!("{}/api/v1/sandboxes/{id}/keepalive", self.base_url))
            .header("Authorization", &auth)
            .send()
            .await?;

        let status = resp.status();
        if !status.is_success() && status.as_u16() != 204 {
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status: status.as_u16(),
                message: text,
            });
        }

        Ok(())
    }

    pub async fn exec(
        &self,
        sandbox_id: &str,
        command: &str,
        options: &ExecOptions,
    ) -> Result<ExecResult, Error> {
        let auth = self.auth_headers().await?;

        let parts: Vec<&str> = command.split_whitespace().collect();
        let program = parts.first().copied().unwrap_or(command);
        let mut args: Vec<String> = parts[1..].iter().map(|s| s.to_string()).collect();
        args.extend(options.args.clone());

        let body = ExecApiRequest {
            command: program.to_string(),
            args,
            env: options.env.clone(),
            cwd: options.cwd.clone(),
            timeout_seconds: options.timeout,
        };

        let resp = self
            .client
            .post(format!(
                "{}/api/v1/sandboxes/{sandbox_id}/exec",
                self.base_url
            ))
            .header("Authorization", &auth)
            .header("Content-Type", "application/json")
            .json(&body)
            .send()
            .await?;

        if !resp.status().is_success() {
            let status = resp.status().as_u16();
            let text = resp.text().await.unwrap_or_default();
            return Err(Error::Api {
                status,
                message: text,
            });
        }

        let api_result: ExecApiResponse = resp.json().await?;
        Ok(ExecResult {
            exit_code: api_result.exit_code,
            stdout: api_result.stdout,
            stderr: api_result.stderr,
        })
    }

    pub fn ws_url(&self, id: &str) -> String {
        let ws_base = self
            .base_url
            .replacen("https://", "wss://", 1)
            .replacen("http://", "ws://", 1);
        format!("{ws_base}/api/v1/sandboxes/{id}/ws")
    }

    pub async fn get_token(&self) -> Option<String> {
        let guard = self.token.lock().await;
        guard.as_ref().map(|s| s.token.clone())
    }
}
