use crate::dedup::MessageDedup;
use crate::error::AlgaError;
use crate::models::*;
use async_trait::async_trait;
use futures_util::StreamExt;
use reqwest::header::{HeaderMap, HeaderValue, ACCEPT, AUTHORIZATION, CACHE_CONTROL, CONNECTION};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Arc;
use std::time::Duration;
use tokio::task::JoinHandle;
use tokio::time::sleep;

#[async_trait]
pub trait EventHandler: Send + Sync {
    async fn on_connected(&self, event: ConnectedEvent);
    async fn on_message(&self, event: MessageEvent);
    async fn on_typing(&self, event: TypingEvent);
    async fn on_investigation_cancel(&self, event: InvestigationSignalEvent);
    async fn on_investigation_pause(&self, event: InvestigationSignalEvent);
    async fn on_investigation_resume(&self, event: InvestigationSignalEvent);
    async fn on_peer_finding(&self, event: PeerFindingEvent);
    async fn on_peer_ask(&self, event: PeerAskEvent);
    async fn on_peer_reply(&self, event: PeerReplyEvent);
    async fn on_agent_presence(&self, event: AgentPresenceEvent);
}

pub struct SSEClient {
    http_base: String,
    token: String,
    dedup: Arc<MessageDedup>,
    shutdown: Arc<AtomicBool>,
    handler: Arc<dyn EventHandler>,
    sse_task: Option<JoinHandle<()>>,
    heartbeat_task: Option<JoinHandle<()>>,
}

impl SSEClient {
    pub fn new(
        http_base: String,
        token: String,
        dedup: Arc<MessageDedup>,
        handler: Arc<dyn EventHandler>,
    ) -> Self {
        Self {
            http_base,
            token,
            dedup,
            shutdown: Arc::new(AtomicBool::new(false)),
            handler,
            sse_task: None,
            heartbeat_task: None,
        }
    }

    pub fn start(&mut self) -> Result<(), AlgaError> {
        if self.token.is_empty() {
            return Err(AlgaError::InvalidToken);
        }
        let shutdown = self.shutdown.clone();
        let http_base = self.http_base.clone();
        let token = self.token.clone();
        let dedup = self.dedup.clone();
        let handler = self.handler.clone();

        self.sse_task = Some(tokio::spawn(async move {
            let mut backoff_secs: u64 = 1;
            let max_backoff_secs: u64 = 60;
            let mut success_count: u32 = 0;
            const SUCCESS_THRESHOLD: u32 = 3;

            loop {
                if shutdown.load(Ordering::Relaxed) {
                    break;
                }

                match sse_loop(&http_base, &token, &dedup, &handler, &shutdown).await {
                    Ok(()) => {
                        if shutdown.load(Ordering::Relaxed) {
                            break;
                        }
                        success_count += 1;
                        if success_count >= SUCCESS_THRESHOLD {
                            backoff_secs = 1;
                            success_count = 0;
                        }
                    }
                    Err(AlgaError::Auth { .. }) => break,
                    Err(e) => {
                        if shutdown.load(Ordering::Relaxed) {
                            break;
                        }
                        // Honor Retry-After when the server gave us one (e.g. 429);
                        // otherwise exponential backoff.
                        let delay = e.retry_after().unwrap_or_else(|| {
                            backoff_secs = (backoff_secs * 2).min(max_backoff_secs);
                            Duration::from_secs(backoff_secs)
                        });
                        tokio::select! {
                            _ = sleep(delay) => {}
                            _ = shutdown_signal(&shutdown) => break,
                        }
                    }
                }
            }
        }));

        let hb_shutdown = self.shutdown.clone();
        let hb_http_base = self.http_base.clone();
        let hb_token = self.token.clone();
        self.heartbeat_task = Some(tokio::spawn(async move {
            loop {
                if hb_shutdown.load(Ordering::Relaxed) {
                    break;
                }
                tokio::select! {
                    _ = sleep(Duration::from_secs(30)) => {}
                    _ = shutdown_signal(&hb_shutdown) => break,
                }
                if hb_shutdown.load(Ordering::Relaxed) {
                    break;
                }
                let url = format!("{}/api/v1/agent/heartbeat", hb_http_base);
                let resp = reqwest::Client::new()
                    .post(&url)
                    .header(AUTHORIZATION, format!("Bearer {}", hb_token))
                    .send()
                    .await;
                if let Ok(r) = resp {
                    let status = r.status().as_u16();
                    // Stop heartbeating on auth failure; the SSE loop will also
                    // exit and the caller can observe the auth error there.
                    if status == 401 || status == 403 {
                        break;
                    }
                }
            }
        }));

        Ok(())
    }

    pub fn stop(&self) {
        self.shutdown.store(true, Ordering::Relaxed);
    }

    /// Returns true once both background tasks have been asked to stop. This is
    /// a coarse-grained polling check, not a guarantee that the tasks have fully
    /// unwound; use [`SSEClient::join`] to await full teardown.
    pub fn is_shutdown(&self) -> bool {
        self.shutdown.load(Ordering::Relaxed)
    }

    /// Await full teardown of both background tasks. Must be called after
    /// `stop()` (typically via `AlgaClient::disconnect()`).
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
        // Abort to guarantee the tasks do not outlive the client when the
        // caller never awaits `join`.
        if let Some(t) = self.sse_task.take() {
            t.abort();
        }
        if let Some(t) = self.heartbeat_task.take() {
            t.abort();
        }
    }
}

async fn shutdown_signal(flag: &Arc<AtomicBool>) {
    loop {
        if flag.load(Ordering::Relaxed) {
            return;
        }
        sleep(Duration::from_millis(250)).await;
    }
}

async fn sse_loop(
    http_base: &str,
    token: &str,
    dedup: &Arc<MessageDedup>,
    handler: &Arc<dyn EventHandler>,
    shutdown: &Arc<AtomicBool>,
) -> Result<(), AlgaError> {
    let url = format!("{}/api/v1/agent/events", http_base);

    let client = reqwest::Client::new();
    let mut headers = HeaderMap::new();
    headers.insert(ACCEPT, HeaderValue::from_static("text/event-stream"));
    headers.insert(CACHE_CONTROL, HeaderValue::from_static("no-cache"));
    headers.insert(CONNECTION, HeaderValue::from_static("keep-alive"));
    headers.insert(
        AUTHORIZATION,
        HeaderValue::from_str(&format!("Bearer {}", token)).map_err(|_| AlgaError::InvalidToken)?,
    );

    let response = client
        .get(&url)
        .headers(headers)
        .send()
        .await
        .map_err(|e| AlgaError::Connection(e.to_string()))?;

    let status = response.status();
    if status.as_u16() == 401 || status.as_u16() == 403 {
        let body = response.text().await.unwrap_or_default();
        return Err(AlgaError::Auth {
            status_code: status.as_u16(),
            message: body,
        });
    }
    if !status.is_success() {
        let retry_after = parse_retry_after(response.headers());
        let body = response.text().await.unwrap_or_default();
        return Err(AlgaError::Api {
            status_code: status.as_u16(),
            message: body,
            retry_after,
        });
    }

    let mut stream = response.bytes_stream();
    let mut buffer = String::new();
    let mut event_type = String::new();
    let mut data_buffer = String::new();

    while let Some(chunk_result) = stream.next().await {
        if shutdown.load(Ordering::Relaxed) {
            break;
        }

        let chunk = chunk_result.map_err(|e| AlgaError::Connection(e.to_string()))?;
        buffer.push_str(&String::from_utf8_lossy(&chunk));

        while let Some(pos) = buffer.find('\n') {
            let line = buffer[..pos].trim_end_matches('\r').to_string();
            buffer = buffer[pos + 1..].to_string();

            if line.is_empty() {
                if !data_buffer.is_empty() {
                    // Per the SSE spec, a missing `event:` field means "message".
                    let ev = if event_type.is_empty() {
                        "message"
                    } else {
                        event_type.as_str()
                    };
                    dispatch_event(ev, &data_buffer, dedup, handler).await;
                    event_type.clear();
                    data_buffer.clear();
                }
                continue;
            }

            if let Some(ev) = line.strip_prefix("event:") {
                event_type = ev.trim().to_string();
            } else if let Some(data) = line.strip_prefix("data:") {
                // Per spec, strip exactly one leading space if present.
                let data = data.strip_prefix(' ').unwrap_or(data);
                if data_buffer.is_empty() {
                    data_buffer.push_str(data);
                } else {
                    data_buffer.push('\n');
                    data_buffer.push_str(data);
                }
            }
        }
    }

    Ok(())
}

async fn dispatch_event(
    event_type: &str,
    data: &str,
    dedup: &Arc<MessageDedup>,
    handler: &Arc<dyn EventHandler>,
) {
    match event_type {
        "connected" => {
            if let Ok(event) = serde_json::from_str::<ConnectedEvent>(data) {
                handler.on_connected(event).await;
            }
        }
        "message" => {
            if let Ok(event) = serde_json::from_str::<MessageEvent>(data) {
                if !event.message_id.is_empty() && dedup.is_duplicate(&event.message_id) {
                    return;
                }
                handler.on_message(event).await;
            }
        }
        "typing" => {
            if let Ok(event) = serde_json::from_str::<TypingEvent>(data) {
                handler.on_typing(event).await;
            }
        }
        "investigation_cancel" => {
            if let Ok(event) = serde_json::from_str::<InvestigationSignalEvent>(data) {
                handler.on_investigation_cancel(event).await;
            }
        }
        "investigation_pause" => {
            if let Ok(event) = serde_json::from_str::<InvestigationSignalEvent>(data) {
                handler.on_investigation_pause(event).await;
            }
        }
        "investigation_resume" => {
            if let Ok(event) = serde_json::from_str::<InvestigationSignalEvent>(data) {
                handler.on_investigation_resume(event).await;
            }
        }
        "peer_finding" => {
            if let Ok(event) = serde_json::from_str::<PeerFindingEvent>(data) {
                handler.on_peer_finding(event).await;
            }
        }
        "peer_ask" => {
            if let Ok(event) = serde_json::from_str::<PeerAskEvent>(data) {
                handler.on_peer_ask(event).await;
            }
        }
        "peer_reply" => {
            if let Ok(event) = serde_json::from_str::<PeerReplyEvent>(data) {
                handler.on_peer_reply(event).await;
            }
        }
        "agent_presence" => {
            if let Ok(event) = serde_json::from_str::<AgentPresenceEvent>(data) {
                handler.on_agent_presence(event).await;
            }
        }
        _ => {}
    }
}

/// Parse the `Retry-After` header into a duration. Supports both delta-seconds
/// and HTTP-date forms.
fn parse_retry_after(headers: &reqwest::header::HeaderMap) -> Option<Duration> {
    let raw = headers.get(reqwest::header::RETRY_AFTER)?.to_str().ok()?;
    if let Ok(secs) = raw.trim().parse::<u64>() {
        return Some(Duration::from_secs(secs));
    }
    let date = httpdate::parse_http_date(raw.trim()).ok()?;
    let delta = date.duration_since(std::time::SystemTime::now()).ok()?;
    Some(delta)
}
