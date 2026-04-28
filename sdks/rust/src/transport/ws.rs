use std::collections::HashMap;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;

use futures_util::{SinkExt, StreamExt};
use tokio::sync::{oneshot, Mutex, RwLock};
use tokio_tungstenite::tungstenite::Message;

use crate::error::Error;
use crate::protocol::{decode_envelope, encode_envelope, msg_type, Envelope};

type MessageHandler = Arc<dyn Fn(&Envelope) + Send + Sync>;

struct PendingRequest {
    tx: oneshot::Sender<Result<Envelope, Error>>,
}

pub struct WsTransport {
    write_tx: Mutex<
        Option<
            futures_util::stream::SplitSink<
                tokio_tungstenite::WebSocketStream<
                    tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>,
                >,
                Message,
            >,
        >,
    >,
    handlers: Arc<RwLock<HashMap<u8, Vec<(u64, MessageHandler)>>>>,
    pending: Arc<Mutex<HashMap<u32, PendingRequest>>>,
    next_id: AtomicU32,
    next_handler_id: AtomicU32,
    read_task: Mutex<Option<tokio::task::JoinHandle<()>>>,
}

impl WsTransport {
    pub async fn connect(url: &str, token: &str) -> Result<Arc<Self>, Error> {
        let encoded_token: String =
            url::form_urlencoded::byte_serialize(token.as_bytes()).collect();
        let full_url = format!("{url}?token={encoded_token}");
        let (ws_stream, _) = tokio_tungstenite::connect_async(&full_url).await?;
        let (write, read) = ws_stream.split();

        let handlers: Arc<RwLock<HashMap<u8, Vec<(u64, MessageHandler)>>>> =
            Arc::new(RwLock::new(HashMap::new()));
        let pending: Arc<Mutex<HashMap<u32, PendingRequest>>> =
            Arc::new(Mutex::new(HashMap::new()));

        let transport = Arc::new(Self {
            write_tx: Mutex::new(Some(write)),
            handlers: handlers.clone(),
            pending: pending.clone(),
            next_id: AtomicU32::new(1),
            next_handler_id: AtomicU32::new(1),
            read_task: Mutex::new(None),
        });

        let transport_weak = Arc::downgrade(&transport);
        let read_task = tokio::spawn(async move {
            let mut read = read;
            while let Some(msg) = read.next().await {
                let msg = match msg {
                    Ok(m) => m,
                    Err(_) => break,
                };

                let data = match msg {
                    Message::Binary(data) => data,
                    Message::Close(_) => break,
                    _ => continue,
                };

                let envelope = match decode_envelope(&data) {
                    Ok(e) => e,
                    Err(_) => continue,
                };

                // Handle ping
                if envelope.msg_type == msg_type::PING {
                    if let Some(transport) = transport_weak.upgrade() {
                        let pong = Envelope {
                            msg_type: msg_type::PONG,
                            channel: 0,
                            id: envelope.id,
                            payload: vec![],
                        };
                        let _ = transport.send_envelope(&pong).await;
                    }
                    continue;
                }

                // Check pending requests
                if envelope.id > 0 {
                    let mut pending_guard = pending.lock().await;
                    if let Some(req) = pending_guard.remove(&envelope.id) {
                        if envelope.msg_type == msg_type::ERROR {
                            let _ = req
                                .tx
                                .send(Err(Error::Protocol("Server error".to_string())));
                        } else {
                            let _ = req.tx.send(Ok(envelope));
                        }
                        continue;
                    }
                }

                // Dispatch to type handlers
                let handlers_guard = handlers.read().await;
                if let Some(type_handlers) = handlers_guard.get(&envelope.msg_type) {
                    for (_, handler) in type_handlers {
                        handler(&envelope);
                    }
                }
            }

            // Connection closed: reject all pending
            let mut pending_guard = pending.lock().await;
            for (_, req) in pending_guard.drain() {
                let _ = req.tx.send(Err(Error::ConnectionClosed));
            }
        });

        *transport.read_task.lock().await = Some(read_task);

        Ok(transport)
    }

    async fn send_envelope(&self, envelope: &Envelope) -> Result<(), Error> {
        let data = encode_envelope(envelope);
        let mut guard = self.write_tx.lock().await;
        let writer = guard.as_mut().ok_or(Error::NotConnected)?;
        writer
            .send(Message::Binary(data.into()))
            .await
            .map_err(Error::WebSocket)
    }

    pub async fn send(&self, envelope: &Envelope) -> Result<(), Error> {
        self.send_envelope(envelope).await
    }

    pub async fn request(
        &self,
        msg_type: u8,
        channel: u32,
        payload: Vec<u8>,
        timeout_ms: u64,
    ) -> Result<Envelope, Error> {
        let id = self.next_id.fetch_add(1, Ordering::SeqCst);

        let (tx, rx) = oneshot::channel();
        {
            let mut pending = self.pending.lock().await;
            pending.insert(id, PendingRequest { tx });
        }

        let envelope = Envelope {
            msg_type,
            channel,
            id,
            payload,
        };
        self.send_envelope(&envelope).await?;

        let result = tokio::time::timeout(std::time::Duration::from_millis(timeout_ms), rx).await;

        match result {
            Ok(Ok(r)) => r,
            Ok(Err(_)) => {
                // Sender dropped (connection closed)
                Err(Error::ConnectionClosed)
            }
            Err(_) => {
                // Timeout: remove pending request
                let mut pending = self.pending.lock().await;
                pending.remove(&id);
                Err(Error::Timeout(format!("Request timeout (id={id})")))
            }
        }
    }

    /// Register a handler for a specific message type. Returns a handler ID for removal.
    pub async fn on(
        &self,
        msg_type: u8,
        handler: impl Fn(&Envelope) + Send + Sync + 'static,
    ) -> u64 {
        let handler_id = self.next_handler_id.fetch_add(1, Ordering::SeqCst) as u64;
        let mut handlers = self.handlers.write().await;
        handlers
            .entry(msg_type)
            .or_default()
            .push((handler_id, Arc::new(handler)));
        handler_id
    }

    /// Remove a handler by its ID.
    pub async fn off(&self, msg_type: u8, handler_id: u64) {
        let mut handlers = self.handlers.write().await;
        if let Some(list) = handlers.get_mut(&msg_type) {
            list.retain(|(id, _)| *id != handler_id);
        }
    }

    pub async fn close(&self) {
        // Close the write side
        let mut guard = self.write_tx.lock().await;
        if let Some(mut writer) = guard.take() {
            let _ = writer.send(Message::Close(None)).await;
        }

        // Abort the read task
        let mut task = self.read_task.lock().await;
        if let Some(handle) = task.take() {
            handle.abort();
        }
    }
}
