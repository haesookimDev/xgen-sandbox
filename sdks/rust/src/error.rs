#[derive(Debug, thiserror::Error)]
pub enum Error {
    #[error("HTTP error: {0}")]
    Http(#[from] reqwest::Error),

    #[error("WebSocket error: {0}")]
    WebSocket(#[from] tokio_tungstenite::tungstenite::Error),

    #[error("Protocol error: {0}")]
    Protocol(String),

    #[error("Auth failed: {0}")]
    Auth(String),

    #[error("API error: {status} {message}")]
    Api { status: u16, message: String },

    #[error("Timeout: {0}")]
    Timeout(String),

    #[error("Not connected")]
    NotConnected,

    #[error("Connection closed")]
    ConnectionClosed,
}
