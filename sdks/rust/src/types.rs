use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum SandboxStatus {
    Starting,
    Running,
    Stopping,
    Stopped,
    Error,
}

#[derive(Debug, Clone, Default, Serialize)]
pub struct CreateSandboxOptions {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub template: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_seconds: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_ms: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub resources: Option<Resources>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub env: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub ports: Option<Vec<u16>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub gui: Option<bool>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub capabilities: Option<Vec<String>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Resources {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cpu: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub memory: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub disk: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct SandboxInfo {
    pub id: String,
    pub status: SandboxStatus,
    pub template: String,
    pub ws_url: String,
    #[serde(default)]
    pub preview_urls: HashMap<String, String>,
    pub vnc_url: Option<String>,
    #[serde(default)]
    pub created_at: String,
    #[serde(default)]
    pub expires_at: String,
    pub created_at_ms: Option<i64>,
    pub expires_at_ms: Option<i64>,
    pub metadata: Option<HashMap<String, String>>,
    pub capabilities: Option<Vec<String>>,
    pub from_warm_pool: Option<bool>,
}

#[derive(Debug, Clone, Deserialize)]
pub struct StructuredError {
    pub code: Option<String>,
    pub message: Option<String>,
    pub details: Option<serde_json::Value>,
    pub request_id: Option<String>,
    pub retryable: Option<bool>,
}

#[derive(Debug, Clone, Default)]
pub struct ExecOptions {
    pub args: Vec<String>,
    pub env: Option<HashMap<String, String>>,
    pub cwd: Option<String>,
    pub timeout: Option<u64>,
}

#[derive(Debug, Clone)]
pub struct ExecResult {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Debug, Clone, Deserialize)]
pub struct FileInfo {
    pub name: String,
    pub size: u64,
    #[serde(rename = "isDir")]
    pub is_dir: bool,
    #[serde(rename = "modTime")]
    pub mod_time: u64,
}

#[derive(Debug, Clone, Deserialize)]
pub struct FileEvent {
    pub path: String,
    #[serde(rename = "type")]
    pub event_type: String,
}

#[derive(Deserialize)]
pub(crate) struct AuthResponse {
    pub token: String,
    #[serde(default)]
    pub expires_at: String,
    pub expires_at_ms: Option<i64>,
}

#[derive(Serialize)]
pub(crate) struct AuthRequest {
    pub api_key: String,
}

#[derive(Deserialize)]
pub(crate) struct ExecApiResponse {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
}

#[derive(Serialize)]
pub(crate) struct ExecApiRequest {
    pub command: String,
    #[serde(skip_serializing_if = "Vec::is_empty")]
    pub args: Vec<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub env: Option<HashMap<String, String>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub cwd: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_seconds: Option<u64>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub timeout_ms: Option<u64>,
}
