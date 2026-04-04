use crate::error::Error;

pub const HEADER_SIZE: usize = 9;

#[allow(dead_code)]
pub mod msg_type {
    pub const PING: u8 = 0x01;
    pub const PONG: u8 = 0x02;
    pub const ERROR: u8 = 0x03;
    pub const ACK: u8 = 0x04;

    pub const EXEC_START: u8 = 0x20;
    pub const EXEC_STDIN: u8 = 0x21;
    pub const EXEC_STDOUT: u8 = 0x22;
    pub const EXEC_STDERR: u8 = 0x23;
    pub const EXEC_EXIT: u8 = 0x24;
    pub const EXEC_SIGNAL: u8 = 0x25;
    pub const EXEC_RESIZE: u8 = 0x26;

    pub const FS_READ: u8 = 0x30;
    pub const FS_WRITE: u8 = 0x31;
    pub const FS_LIST: u8 = 0x32;
    pub const FS_REMOVE: u8 = 0x33;
    pub const FS_WATCH: u8 = 0x34;
    pub const FS_EVENT: u8 = 0x35;

    pub const PORT_OPEN: u8 = 0x40;
    pub const PORT_CLOSE: u8 = 0x41;

    pub const SANDBOX_READY: u8 = 0x50;
    pub const SANDBOX_ERROR: u8 = 0x51;
    pub const SANDBOX_STATS: u8 = 0x52;
}

#[derive(Debug, Clone)]
pub struct Envelope {
    pub msg_type: u8,
    pub channel: u32,
    pub id: u32,
    pub payload: Vec<u8>,
}

pub fn encode_envelope(env: &Envelope) -> Vec<u8> {
    let mut buf = Vec::with_capacity(HEADER_SIZE + env.payload.len());
    buf.push(env.msg_type);
    buf.extend_from_slice(&env.channel.to_be_bytes());
    buf.extend_from_slice(&env.id.to_be_bytes());
    buf.extend_from_slice(&env.payload);
    buf
}

pub fn decode_envelope(data: &[u8]) -> Result<Envelope, Error> {
    if data.len() < HEADER_SIZE {
        return Err(Error::Protocol(format!(
            "Message too short: {} bytes",
            data.len()
        )));
    }
    let msg_type = data[0];
    let channel = u32::from_be_bytes([data[1], data[2], data[3], data[4]]);
    let id = u32::from_be_bytes([data[5], data[6], data[7], data[8]]);
    let payload = data[HEADER_SIZE..].to_vec();
    Ok(Envelope {
        msg_type,
        channel,
        id,
        payload,
    })
}

pub fn encode_payload<T: serde::Serialize>(value: &T) -> Result<Vec<u8>, Error> {
    rmp_serde::to_vec(value).map_err(|e| Error::Protocol(format!("msgpack encode error: {e}")))
}

pub fn decode_payload<T: serde::de::DeserializeOwned>(data: &[u8]) -> Result<T, Error> {
    rmp_serde::from_slice(data).map_err(|e| Error::Protocol(format!("msgpack decode error: {e}")))
}
