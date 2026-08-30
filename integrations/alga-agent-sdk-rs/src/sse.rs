use crate::dedup::MessageDedup;
use crate::error::{is_auth_error, AlgaError};
use crate::models::*;
use async_trait::async_trait;
use futures_util::StreamExt;
use reqwest::header::{HeaderMap, HeaderValue, ACCEPT, AUTHORIZATION, USER_AGENT};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::time::Duration;
use tokio::task::JoinHandle;
use tokio::time::sleep;

const LOCK_EMOJI: char = '\u{1F512}';

#[async_trait]
pub trait EventHandler: Send + Sync {
    async fn on_connected(&self, _event: ConnectedEvent) {}
    async fn on_message(&self, _event: MessageEvent) {}
    async fn on_typing(&self, _event: TypingEvent) {}
    async fn on_investigation_resume(&self, _event: InvestigationSignalEvent) {}
    async fn on_peer_finding(&self, _event: PeerFindingEvent) {}
    async fn on_peer_ask(&self, _event: PeerAskEvent) {}
    async fn on_peer_reply(&self, _event: PeerReplyEvent) {}
    async fn on_summarize_incident(&self, _event: SummarizeIncidentEvent) {}
    async fn on_alert_auto_resolved(&self, _event: AlertAutoResolvedEvent) {}
    async fn on_incident_comms_stale(&self, _event: IncidentCommsStaleEvent) {}
    async fn on_unknown_event(&self, _event_type: &str, _data: &str) {}
}

pub struct SSEClient {
    http_base: String,
    token: String,
    dedup: Arc<MessageDedup>,
    handler: Arc<dyn EventHandler>,
    http_client: reqwest::Client,
    heartbeat_interval: Duration,
    user_agent: String,
    shutdown: Arc<AtomicBool>,
    fatal_error: Arc<Mutex<Option<AlgaError>>>,
    sse_task: Option<JoinHandle<()>>,
    heartbeat_task: Option<JoinHandle<()>>,
}

impl SSEClient {
    pub fn new(
        http_base: String,
        token: String,
        dedup: Arc<MessageDedup>,
        handler: Arc<dyn EventHandler>,
        heartbeat_interval: Duration,
        user_agent: String,
        fatal_error: Arc<Mutex<Option<AlgaError>>>,
    ) -> Result<Self, AlgaError> {
        let http_client = reqwest::Client::builder()
            .build()
            .map_err(|e| AlgaError::Connection(e.to_string()))?;
        Ok(Self {
            http_base,
            token,
            dedup,
            handler,
            http_client,
            heartbeat_interval,
            user_agent,
            shutdown: Arc::new(AtomicBool::new(false)),
            fatal_error,
            sse_task: None,
            heartbeat_task: None,
        })
    }

    pub fn start(&mut self) -> Result<(), AlgaError> {
        if self.token.is_empty() {
            return Err(AlgaError::InvalidToken);
        }

        let shutdown = self.shutdown.clone();
        let fatal_error = self.fatal_error.clone();
        let http_base = self.http_base.clone();
        let token = self.token.clone();
        let user_agent = self.user_agent.clone();
        let http_client = self.http_client.clone();
        let dedup = self.dedup.clone();
        let handler = self.handler.clone();

        self.sse_task = Some(tokio::spawn(async move {
            let mut backoff = Duration::from_secs(2);
            let max_backoff = Duration::from_secs(60);

            loop {
                if shutdown.load(Ordering::Relaxed) {
                    break;
                }

                let (connected, err) = connect_and_serve(
                    &http_base,
                    &token,
                    &user_agent,
                    &http_client,
                    &dedup,
                    &handler,
                    &shutdown,
                )
                .await;

                if connected {
                    backoff = Duration::from_secs(2);
                }

                if let Some(e) = err {
                    if is_auth_error(&e) {
                        *fatal_error.lock().expect("fatal_error mutex poisoned") = Some(e);
                        break;
                    }

                    let delay = if let Some(ra) = e.retry_after() {
                        ra
                    } else {
                        let jitter = 0.9 + random_f64() * 0.2;
                        let d = Duration::from_millis(
                            ((backoff.as_millis() as f64) * jitter) as u64,
                        );
                        backoff = (backoff * 2).min(max_backoff);
                        d
                    };

                    tokio::select! {
                        _ = sleep(delay) => {}
                        _ = wait_shutdown(&shutdown) => break,
                    }
                }

                if shutdown.load(Ordering::Relaxed) {
                    break;
                }
            }
        }));

        let hb_shutdown = self.shutdown.clone();
        let hb_fatal = self.fatal_error.clone();
        let hb_http_base = self.http_base.clone();
        let hb_token = self.token.clone();
        let hb_user_agent = self.user_agent.clone();
        let hb_http_client = self.http_client.clone();
        let hb_interval = self.heartbeat_interval;

        self.heartbeat_task = Some(tokio::spawn(async move {
            loop {
                tokio::select! {
                    _ = sleep(hb_interval) => {}
                    _ = wait_shutdown(&hb_shutdown) => break,
                }
                if hb_shutdown.load(Ordering::Relaxed) {
                    break;
                }
                match post_heartbeat(&hb_http_base, &hb_token, &hb_user_agent, &hb_http_client)
                    .await
                {
                    Ok(()) => {}
                    Err(e) if is_auth_error(&e) => {
                        *hb_fatal.lock().expect("fatal_error mutex poisoned") = Some(e);
                        hb_shutdown.store(true, Ordering::Relaxed);
                        break;
                    }
                    Err(_) => {}
                }
            }
        }));

        Ok(())
    }

    pub fn stop(&self) {
        self.shutdown.store(true, Ordering::Relaxed);
    }

    pub fn is_shutdown(&self) -> bool {
        self.shutdown.load(Ordering::Relaxed)
    }

    pub async fn join(&mut self) {
        if let Some(t) = self.sse_task.take() {
            let _ = t.await;
        }
        if let Some(t) = self.heartbeat_task.take() {
            let _ = t.await;
        }
    }
}

impl Drop for SSEClient {
    fn drop(&mut self) {
        self.stop();
        if let Some(t) = self.sse_task.take() {
            t.abort();
        }
        if let Some(t) = self.heartbeat_task.take() {
            t.abort();
        }
    }
}

async fn wait_shutdown(flag: &Arc<AtomicBool>) {
    loop {
        if flag.load(Ordering::Relaxed) {
            return;
        }
        sleep(Duration::from_millis(250)).await;
    }
}

#[allow(clippy::too_many_arguments)]
async fn connect_and_serve(
    http_base: &str,
    token: &str,
    user_agent: &str,
    http_client: &reqwest::Client,
    dedup: &Arc<MessageDedup>,
    handler: &Arc<dyn EventHandler>,
    shutdown: &Arc<AtomicBool>,
) -> (bool, Option<AlgaError>) {
    let url = format!("{}/api/v1/agent/events", http_base);

    let mut headers = HeaderMap::new();
    headers.insert(ACCEPT, HeaderValue::from_static("text/event-stream"));
    if let Ok(v) = HeaderValue::from_str(&format!("Bearer {}", token)) {
        headers.insert(AUTHORIZATION, v);
    }
    if let Ok(v) = HeaderValue::from_str(user_agent) {
        headers.insert(USER_AGENT, v);
    }

    let response = match http_client.get(&url).headers(headers).send().await {
        Ok(r) => r,
        Err(e) => return (false, Some(AlgaError::Connection(e.to_string()))),
    };

    let status = response.status();
    if status.as_u16() == 401 || status.as_u16() == 403 {
        return (
            false,
            Some(AlgaError::Auth {
                status_code: status.as_u16(),
                message: "authentication failed".to_string(),
            }),
        );
    }
    if !status.is_success() {
        let retry_after = parse_retry_after(response.headers());
        return (
            false,
            Some(AlgaError::Api {
                status_code: status.as_u16(),
                message: "unexpected status code".to_string(),
                retry_after,
            }),
        );
    }

    let mut stream = response.bytes_stream();
    let mut buffer = String::new();
    let mut event_type = String::new();
    let mut data_buffer = String::new();

    while let Some(chunk_result) = stream.next().await {
        if shutdown.load(Ordering::Relaxed) {
            break;
        }

        let chunk = match chunk_result {
            Ok(c) => c,
            Err(e) => {
                return (true, Some(AlgaError::Connection(e.to_string())))
            }
        };
        buffer.push_str(&String::from_utf8_lossy(&chunk));

        while let Some(pos) = buffer.find('\n') {
            let line = buffer[..pos].trim_end_matches('\r').to_string();
            buffer = buffer[pos + 1..].to_string();

            if line.is_empty() {
                if !data_buffer.is_empty() {
                    let ev = if event_type.is_empty() {
                        "message".to_string()
                    } else {
                        event_type.clone()
                    };
                    dispatch_event(&ev, &data_buffer, dedup, handler).await;
                    event_type.clear();
                    data_buffer.clear();
                }
                continue;
            }

            if line.starts_with(':') {
                continue;
            }

            if let Some(rest) = line.strip_prefix("event:") {
                event_type = rest.trim().to_string();
            } else if let Some(rest) = line.strip_prefix("data:") {
                let data = rest.strip_prefix(' ').unwrap_or(rest);
                if data_buffer.is_empty() {
                    data_buffer.push_str(data);
                } else {
                    data_buffer.push('\n');
                    data_buffer.push_str(data);
                }
            }
        }
    }

    (
        true,
        Some(AlgaError::Connection("sse stream closed".to_string())),
    )
}

async fn dispatch_event(
    event_type: &str,
    data: &str,
    dedup: &Arc<MessageDedup>,
    handler: &Arc<dyn EventHandler>,
) {
    let trimmed = data.trim();
    match event_type {
        "connected" => {
            if let Ok(event) = serde_json::from_str::<ConnectedEvent>(trimmed) {
                handler.on_connected(event).await;
            }
        }
        "message" => {
            if let Ok(event) = serde_json::from_str::<MessageEvent>(trimmed) {
                if !event.message_id.is_empty() && dedup.is_duplicate(&event.message_id) {
                    return;
                }
                if event.text.starts_with(LOCK_EMOJI) {
                    return;
                }
                handler.on_message(event).await;
            }
        }
        "typing" => {
            if let Ok(event) = serde_json::from_str::<TypingEvent>(trimmed) {
                handler.on_typing(event).await;
            }
        }
        "investigation_resume" => {
            if let Ok(event) = serde_json::from_str::<InvestigationSignalEvent>(trimmed) {
                handler.on_investigation_resume(event).await;
            }
        }
        "peer_finding" => {
            if let Ok(event) = serde_json::from_str::<PeerFindingEvent>(trimmed) {
                handler.on_peer_finding(event).await;
            }
        }
        "peer_ask" => {
            if let Ok(event) = serde_json::from_str::<PeerAskEvent>(trimmed) {
                handler.on_peer_ask(event).await;
            }
        }
        "peer_reply" => {
            if let Ok(event) = serde_json::from_str::<PeerReplyEvent>(trimmed) {
                handler.on_peer_reply(event).await;
            }
        }
        "summarize_incident" => {
            if let Ok(event) = serde_json::from_str::<SummarizeIncidentEvent>(trimmed) {
                handler.on_summarize_incident(event).await;
            }
        }
        "alert_auto_resolved" => {
            if let Ok(event) = serde_json::from_str::<AlertAutoResolvedEvent>(trimmed) {
                handler.on_alert_auto_resolved(event).await;
            }
        }
        "incident_comms_stale" => {
            if let Ok(event) = serde_json::from_str::<IncidentCommsStaleEvent>(trimmed) {
                handler.on_incident_comms_stale(event).await;
            }
        }
        _ => {
            handler.on_unknown_event(event_type, trimmed).await;
        }
    }
}

async fn post_heartbeat(
    http_base: &str,
    token: &str,
    user_agent: &str,
    http_client: &reqwest::Client,
) -> Result<(), AlgaError> {
    let url = format!("{}/api/v1/agent/heartbeat", http_base);
    let mut headers = HeaderMap::new();
    if let Ok(v) = HeaderValue::from_str(&format!("Bearer {}", token)) {
        headers.insert(AUTHORIZATION, v);
    }
    if let Ok(v) = HeaderValue::from_str(user_agent) {
        headers.insert(USER_AGENT, v);
    }
    let response = http_client
        .post(&url)
        .headers(headers)
        .send()
        .await
        .map_err(|e| AlgaError::Connection(e.to_string()))?;
    let status = response.status();
    if status.as_u16() == 401 || status.as_u16() == 403 {
        return Err(AlgaError::Auth {
            status_code: status.as_u16(),
            message: "heartbeat auth failed".to_string(),
        });
    }
    if !status.is_success() {
        return Err(AlgaError::Api {
            status_code: status.as_u16(),
            message: "heartbeat non-ok status".to_string(),
            retry_after: None,
        });
    }
    Ok(())
}

fn parse_retry_after(headers: &reqwest::header::HeaderMap) -> Option<Duration> {
    let raw = headers.get(reqwest::header::RETRY_AFTER)?.to_str().ok()?;
    let trimmed = raw.trim();
    if let Ok(secs) = trimmed.parse::<u64>() {
        return Some(Duration::from_secs(secs).min(Duration::from_secs(600)));
    }
    let date = httpdate::parse_http_date(trimmed).ok()?;
    let delta = date.duration_since(std::time::SystemTime::now()).ok()?;
    Some(delta.min(Duration::from_secs(600)))
}

fn random_f64() -> f64 {
    let mut buf = [0u8; 8];
    fill_random(&mut buf);
    let bits = u64::from_le_bytes(buf);
    (bits >> 11) as f64 / (1u64 << 53) as f64
}

fn fill_random(buf: &mut [u8]) {
    use std::io::Read;
    if std::fs::File::open("/dev/urandom")
        .and_then(|mut f| f.read_exact(buf))
        .is_ok()
    {
        return;
    }
    let t = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos() as u64)
        .unwrap_or(0);
    for (i, b) in buf.iter_mut().enumerate() {
        *b = ((t >> ((i * 7) % 64)) & 0xff) as u8;
    }
}
