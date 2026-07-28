use crate::commands::InvestigationCommand;
use crate::dedup::MessageDedup;
use crate::error::AlgaError;
use crate::models::*;
use crate::sse::{EventHandler, SSEClient};
use reqwest::header::{HeaderMap, HeaderName, HeaderValue, AUTHORIZATION, CONTENT_TYPE, USER_AGENT};
use std::sync::{Arc, Mutex};
use std::time::Duration;

const DEFAULT_USER_AGENT: &str = "alga-agent-sdk-rs";
const AGENT_MESSAGES_PATH: &str = "/api/v1/agent/messages";
const IDEMPOTENCY_KEY: HeaderName = HeaderName::from_static("idempotency-key");
const MAX_ERROR_MESSAGE_BYTES: usize = 4 * 1024;

pub struct AlgaClientOptions {
    pub heartbeat_interval: Duration,
    pub max_rest_retries: usize,
    pub user_agent: String,
    pub dedup: Option<Arc<MessageDedup>>,
}

impl Default for AlgaClientOptions {
    fn default() -> Self {
        Self {
            heartbeat_interval: Duration::from_secs(30),
            max_rest_retries: 2,
            user_agent: DEFAULT_USER_AGENT.to_string(),
            dedup: None,
        }
    }
}

pub struct AlgaClient {
    server_url: String,
    token: String,
    http_client: reqwest::Client,
    user_agent: String,
    dedup: Arc<MessageDedup>,
    max_rest_retries: usize,
    heartbeat_interval: Duration,
    fatal_error: Arc<Mutex<Option<AlgaError>>>,
    sse: Option<SSEClient>,
}

impl AlgaClient {
    pub fn new(server_url: &str, token: &str) -> Result<Self, AlgaError> {
        Self::with_options(server_url, token, AlgaClientOptions::default())
    }

    pub fn with_options(
        server_url: &str,
        token: &str,
        options: AlgaClientOptions,
    ) -> Result<Self, AlgaError> {
        let http_client = reqwest::Client::builder()
            .timeout(Duration::from_secs(30))
            .build()
            .map_err(|e| AlgaError::Connection(e.to_string()))?;
        let heartbeat_interval = if options.heartbeat_interval < Duration::from_secs(1) {
            Duration::from_secs(1)
        } else {
            options.heartbeat_interval
        };
        let dedup = options
            .dedup
            .unwrap_or_else(|| Arc::new(MessageDedup::new(1000, Duration::from_secs(300))));
        Ok(Self {
            server_url: server_url.trim_end_matches('/').to_string(),
            token: token.to_string(),
            http_client,
            user_agent: options.user_agent,
            dedup,
            max_rest_retries: options.max_rest_retries,
            heartbeat_interval,
            fatal_error: Arc::new(Mutex::new(None)),
            sse: None,
        })
    }

    pub fn server_url(&self) -> &str {
        &self.server_url
    }

    pub fn connect(&mut self, handler: Arc<dyn EventHandler>) -> Result<(), AlgaError> {
        let mut sse = SSEClient::new(
            self.server_url.clone(),
            self.token.clone(),
            self.dedup.clone(),
            handler,
            self.heartbeat_interval,
            self.user_agent.clone(),
            self.fatal_error.clone(),
        )?;
        sse.start()?;
        self.sse = Some(sse);
        Ok(())
    }

    pub fn disconnect(&mut self) {
        if let Some(sse) = self.sse.take() {
            sse.stop();
        }
    }

    pub async fn join(&mut self) {
        if let Some(sse) = self.sse.as_mut() {
            sse.join().await;
        }
    }

    pub fn take_fatal_error(&self) -> Option<AlgaError> {
        self.fatal_error
            .lock()
            .expect("fatal_error mutex poisoned")
            .take()
    }

    async fn do_json<T: serde::de::DeserializeOwned + Default>(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<Vec<u8>>,
        params: Option<&[(&str, &str)]>,
        idempotency_key: Option<&str>,
    ) -> Result<T, AlgaError> {
        let bytes = self
            .execute(method, path, body, params, idempotency_key)
            .await?;
        if bytes.is_empty() {
            return Ok(T::default());
        }
        unwrap_envelope(&bytes).map_err(AlgaError::Json)
    }

    async fn do_void(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<Vec<u8>>,
        params: Option<&[(&str, &str)]>,
        idempotency_key: Option<&str>,
    ) -> Result<(), AlgaError> {
        self.execute(method, path, body, params, idempotency_key)
            .await?;
        Ok(())
    }

    async fn execute(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<Vec<u8>>,
        params: Option<&[(&str, &str)]>,
        idempotency_key: Option<&str>,
    ) -> Result<Vec<u8>, AlgaError> {
        let is_messages = path == AGENT_MESSAGES_PATH;
        let mutating = method != reqwest::Method::GET && method != reqwest::Method::HEAD;

        let mut key = idempotency_key.map(|s| s.to_string());
        if mutating && key.is_none() && is_messages {
            key = Some(new_idempotency_key());
        }

        let attempts = if mutating && key.is_none() {
            0
        } else {
            self.max_rest_retries
        };

        let mut last_err: Option<AlgaError> = None;

        for attempt in 0..=attempts {
            let request = self.build_request(
                method.clone(),
                path,
                body.as_deref(),
                params,
                key.as_deref(),
            )?;

            let response = match self.http_client.execute(request).await {
                Ok(r) => r,
                Err(e) => {
                    let err = AlgaError::Connection(e.to_string());
                    if !err.is_retryable() || attempt == attempts {
                        return Err(err);
                    }
                    last_err = Some(err);
                    let backoff = backoff_for(attempt, None);
                    tokio::time::sleep(backoff).await;
                    continue;
                }
            };

            let status = response.status();
            if status.as_u16() == 401 || status.as_u16() == 403 {
                let body_text = response.text().await.unwrap_or_default();
                return Err(AlgaError::Auth {
                    status_code: status.as_u16(),
                    message: truncate(body_text, MAX_ERROR_MESSAGE_BYTES),
                });
            }

            if !status.is_success() {
                let retry_after = parse_retry_after(response.headers());
                let body_text = response.text().await.unwrap_or_default();
                let api_err = AlgaError::Api {
                    status_code: status.as_u16(),
                    message: truncate(body_text, MAX_ERROR_MESSAGE_BYTES),
                    retry_after,
                };
                if !api_err.is_retryable() || attempt == attempts {
                    return Err(api_err);
                }
                last_err = Some(api_err);
                let backoff = backoff_for(attempt, retry_after);
                tokio::time::sleep(backoff).await;
                continue;
            }

            let bytes = response
                .bytes()
                .await
                .map_err(|e| AlgaError::Connection(e.to_string()))?;
            return Ok(bytes.to_vec());
        }

        Err(last_err.unwrap_or_else(|| AlgaError::Connection("exhausted retries".into())))
    }

    fn build_request(
        &self,
        method: reqwest::Method,
        path: &str,
        body: Option<&[u8]>,
        params: Option<&[(&str, &str)]>,
        idempotency_key: Option<&str>,
    ) -> Result<reqwest::Request, AlgaError> {
        let url = format!("{}{}", self.server_url, path);
        let mut req = self.http_client.request(method, &url);

        let mut headers = HeaderMap::new();
        let auth_value = HeaderValue::from_str(&format!("Bearer {}", self.token))
            .map_err(|_| AlgaError::InvalidToken)?;
        headers.insert(AUTHORIZATION, auth_value);
        let ua_value = HeaderValue::from_str(&self.user_agent)
            .unwrap_or_else(|_| HeaderValue::from_static(DEFAULT_USER_AGENT));
        headers.insert(USER_AGENT, ua_value);
        if body.is_some() {
            headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
        }
        if let Some(k) = idempotency_key {
            let kv = HeaderValue::from_str(k).map_err(|_| AlgaError::InvalidToken)?;
            headers.insert(IDEMPOTENCY_KEY, kv);
        }
        req = req.headers(headers);

        if let Some(p) = params {
            let filtered: Vec<(&str, &str)> = p
                .iter()
                .filter(|(_, v)| !v.is_empty())
                .copied()
                .collect();
            if !filtered.is_empty() {
                req = req.query(&filtered);
            }
        }

        if let Some(b) = body {
            req = req.body(b.to_vec());
        }

        req.build().map_err(|e| AlgaError::Connection(e.to_string()))
    }

    pub async fn list_alerts(
        &self,
        params: &[(&str, &str)],
    ) -> Result<Vec<Alert>, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/alerts",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn get_alert(&self, fingerprint: &str) -> Result<Alert, AlgaError> {
        let path = format!("/api/v1/agent/alerts/{}", encode_path(fingerprint));
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn resolve_alert(&self, fingerprint: &str) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/alerts/{}/resolve", encode_path(fingerprint));
        self.do_void(reqwest::Method::POST, &path, None, None, None)
            .await
    }

    pub async fn reopen_alert(&self, fingerprint: &str) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/alerts/{}/reopen", encode_path(fingerprint));
        self.do_void(reqwest::Method::POST, &path, None, None, None)
            .await
    }

    pub async fn list_incident_tasks(
        &self,
        incident_number: i64,
        params: &[(&str, &str)],
    ) -> Result<Vec<CoordinationTask>, AlgaError> {
        let path = format!("/api/v1/agent/incidents/{}/tasks", incident_number);
        self.do_json(reqwest::Method::GET, &path, None, Some(params), None)
            .await
    }

    pub async fn get_incident(
        &self,
        incident_number: i64,
    ) -> Result<IncidentContext, AlgaError> {
        let path = format!("/api/v1/agent/incidents/{}", incident_number);
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn get_incident_timeline(
        &self,
        incident_number: i64,
    ) -> Result<Vec<serde_json::Value>, AlgaError> {
        let path = format!("/api/v1/agent/incidents/{}/timeline", incident_number);
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn add_incident_timeline(
        &self,
        incident_number: i64,
        message: &str,
        event_type: &str,
    ) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/incidents/{}/timeline", incident_number);
        let payload = serde_json::json!({
            "message": message,
            "event_type": event_type,
        });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::POST, &path, Some(body), None, None)
            .await
    }

    pub async fn update_incident_summary(
        &self,
        incident_number: i64,
        summary: &str,
    ) -> Result<Incident, AlgaError> {
        let path = format!("/api/v1/agent/incidents/{}", incident_number);
        let payload = serde_json::json!({ "summary": summary });
        let body = serde_json::to_vec(&payload)?;
        self.do_json(reqwest::Method::PATCH, &path, Some(body), None, None)
            .await
    }

    pub async fn send_message(
        &self,
        chat_id: &str,
        text: &str,
        mentions: Option<&[&str]>,
    ) -> Result<SendMessageResponse, AlgaError> {
        self.send_message_with_key(chat_id, text, mentions, "").await
    }

    pub async fn send_message_with_key(
        &self,
        chat_id: &str,
        text: &str,
        mentions: Option<&[&str]>,
        idempotency_key: &str,
    ) -> Result<SendMessageResponse, AlgaError> {
        let mut payload = serde_json::Map::new();
        payload.insert("chat_id".into(), serde_json::Value::String(chat_id.into()));
        payload.insert("kind".into(), serde_json::Value::String("text".into()));
        payload.insert("text".into(), serde_json::Value::String(text.into()));
        let mentions_val = match mentions {
            Some(m) => serde_json::Value::Array(
                m.iter()
                    .map(|s| serde_json::Value::String((*s).to_string()))
                    .collect(),
            ),
            None => serde_json::Value::Array(vec![]),
        };
        payload.insert("mentions".into(), mentions_val);
        let body = serde_json::to_vec(&serde_json::Value::Object(payload))?;
        let key = if idempotency_key.is_empty() {
            None
        } else {
            Some(idempotency_key)
        };
        self.do_json(reqwest::Method::POST, AGENT_MESSAGES_PATH, Some(body), None, key)
            .await
    }

    pub async fn send_command(
        &self,
        chat_id: &str,
        cmd: InvestigationCommand,
    ) -> Result<CommandResponse, AlgaError> {
        self.send_command_with_key(chat_id, cmd, "").await
    }

    pub async fn send_command_with_key(
        &self,
        chat_id: &str,
        cmd: InvestigationCommand,
        idempotency_key: &str,
    ) -> Result<CommandResponse, AlgaError> {
        let payload = serde_json::json!({
            "chat_id": chat_id,
            "kind": "inv_tool",
            "command": cmd,
        });
        let body = serde_json::to_vec(&payload)?;
        let key = if idempotency_key.is_empty() {
            None
        } else {
            Some(idempotency_key)
        };
        self.do_json(reqwest::Method::POST, AGENT_MESSAGES_PATH, Some(body), None, key)
            .await
    }

    pub async fn send_incident_summary(
        &self,
        incident_number: i64,
        text: &str,
    ) -> Result<(), AlgaError> {
        let payload = serde_json::json!({
            "chat_id": format!("incident_coord_{}", incident_number),
            "kind": "incident_summary",
            "text": text,
        });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::POST, AGENT_MESSAGES_PATH, Some(body), None, None)
            .await
    }

    pub async fn send_draft(
        &self,
        chat_id: &str,
        draft_id: &str,
        text: &str,
    ) -> Result<(), AlgaError> {
        let payload = serde_json::json!({
            "chat_id": chat_id,
            "draft_id": draft_id,
            "text": text,
        });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::POST, "/api/v1/agent/drafts", Some(body), None, None)
            .await
    }

    pub async fn edit_message(
        &self,
        message_id: &str,
        chat_id: &str,
        text: &str,
    ) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/messages/{}", encode_path(message_id));
        let payload = serde_json::json!({
            "chat_id": chat_id,
            "kind": "text",
            "text": text,
        });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::PUT, &path, Some(body), None, None)
            .await
    }

    pub async fn delete_message(
        &self,
        message_id: &str,
        chat_id: &str,
    ) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/messages/{}", encode_path(message_id));
        let payload = serde_json::json!({ "chat_id": chat_id });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::DELETE, &path, Some(body), None, None)
            .await
    }

    pub async fn send_typing(&self, chat_id: &str, active: bool) -> Result<(), AlgaError> {
        let payload = serde_json::json!({ "chat_id": chat_id, "active": active });
        let body = serde_json::to_vec(&payload)?;
        self.do_void(reqwest::Method::POST, "/api/v1/agent/typing", Some(body), None, None)
            .await
    }

    pub async fn send_heartbeat(&self) -> Result<(), AlgaError> {
        self.do_void(reqwest::Method::POST, "/api/v1/agent/heartbeat", None, None, None)
            .await
    }

    pub async fn list_knowledge(
        &self,
        params: &[(&str, &str)],
    ) -> Result<KnowledgeListResponse, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/knowledge",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn get_knowledge(&self, id: &str) -> Result<KnowledgeNote, AlgaError> {
        let path = format!("/api/v1/agent/knowledge/{}", encode_path(id));
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn create_knowledge(
        &self,
        params: serde_json::Value,
    ) -> Result<KnowledgeNote, AlgaError> {
        let body = serde_json::to_vec(&params)?;
        self.do_json(
            reqwest::Method::POST,
            "/api/v1/agent/knowledge",
            Some(body),
            None,
            None,
        )
        .await
    }

    pub async fn list_memories(
        &self,
        params: &[(&str, &str)],
    ) -> Result<MemoryListResponse, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/memories",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn create_memory(&self, params: serde_json::Value) -> Result<Memory, AlgaError> {
        let body = serde_json::to_vec(&params)?;
        self.do_json(
            reqwest::Method::POST,
            "/api/v1/agent/memories",
            Some(body),
            None,
            None,
        )
        .await
    }

    pub async fn get_memory(&self, id: &str) -> Result<Memory, AlgaError> {
        let path = format!("/api/v1/agent/memories/{}", encode_path(id));
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn delete_memory(&self, id: &str) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/memories/{}", encode_path(id));
        self.do_void(reqwest::Method::DELETE, &path, None, None, None)
            .await
    }

    pub async fn list_peer_asks(
        &self,
        params: &[(&str, &str)],
    ) -> Result<PeerAskListResponse, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/peer-ask",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn create_peer_ask(&self, params: serde_json::Value) -> Result<PeerAsk, AlgaError> {
        let body = serde_json::to_vec(&params)?;
        self.do_json(
            reqwest::Method::POST,
            "/api/v1/agent/peer-ask",
            Some(body),
            None,
            None,
        )
        .await
    }

    pub async fn get_peer_ask(&self, id: &str) -> Result<PeerAsk, AlgaError> {
        let path = format!("/api/v1/agent/peer-ask/{}", encode_path(id));
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }

    pub async fn reply_peer_ask(
        &self,
        id: &str,
        reply: &str,
    ) -> Result<PeerAsk, AlgaError> {
        let path = format!("/api/v1/agent/peer-ask/{}/reply", encode_path(id));
        let payload = serde_json::json!({ "reply": reply });
        let body = serde_json::to_vec(&payload)?;
        self.do_json(reqwest::Method::POST, &path, Some(body), None, None)
            .await
    }

    pub async fn cancel_peer_ask(&self, id: &str) -> Result<(), AlgaError> {
        let path = format!("/api/v1/agent/peer-ask/{}/cancel", encode_path(id));
        self.do_void(reqwest::Method::POST, &path, None, None, None)
            .await
    }

    pub async fn list_services(
        &self,
        params: &[(&str, &str)],
    ) -> Result<ServiceListResponse, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/services",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn who_is_on_call(&self) -> Result<Vec<OnCallEntry>, AlgaError> {
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/on-call/current",
            None,
            None,
            None,
        )
        .await
    }

    pub async fn get_playbooks(
        &self,
        alert_fingerprint: &str,
    ) -> Result<Vec<Playbook>, AlgaError> {
        let params: &[(&str, &str)] = &[("alert_fingerprint", alert_fingerprint)];
        self.do_json(
            reqwest::Method::GET,
            "/api/v1/agent/playbooks",
            None,
            Some(params),
            None,
        )
        .await
    }

    pub async fn get_secret(&self, secret_id: &str) -> Result<SecretValue, AlgaError> {
        let path = format!("/api/v1/agent/secrets/{}", encode_path(secret_id));
        self.do_json(reqwest::Method::GET, &path, None, None, None)
            .await
    }
}

fn encode_path(segment: &str) -> String {
    let mut out = String::with_capacity(segment.len());
    for b in segment.bytes() {
        if b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.' | b'~') {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{:02X}", b));
        }
    }
    out
}

fn truncate(s: String, n: usize) -> String {
    if s.len() <= n {
        return s;
    }
    let mut end = n;
    while end > 0 && !s.is_char_boundary(end) {
        end -= 1;
    }
    s[..end].to_string()
}

fn parse_retry_after(headers: &reqwest::header::HeaderMap) -> Option<Duration> {
    let raw = headers.get(reqwest::header::RETRY_AFTER)?.to_str().ok()?;
    let trimmed = raw.trim();
    if let Ok(secs) = trimmed.parse::<u64>() {
        return Some(Duration::from_secs(secs).min(Duration::from_secs(600)));
    }
    let date = httpdate::parse_http_date(trimmed).ok()?;
    let delta = date
        .duration_since(std::time::SystemTime::now())
        .ok()?;
    Some(delta.min(Duration::from_secs(600)))
}

fn unwrap_envelope<T: serde::de::DeserializeOwned>(
    body: &[u8],
) -> Result<T, serde_json::Error> {
    #[derive(serde::Deserialize)]
    struct Envelope {
        #[serde(default)]
        data: Option<serde_json::Value>,
    }
    if let Ok(env) = serde_json::from_slice::<Envelope>(body) {
        if let Some(data) = env.data {
            if !data.is_null() {
                return serde_json::from_value(data);
            }
        }
    }
    serde_json::from_slice(body)
}

fn new_idempotency_key() -> String {
    let mut buf = [0u8; 16];
    fill_random(&mut buf);
    let hex: String = buf.iter().map(|b| format!("{:02x}", b)).collect();
    format!("alga-{}", hex)
}

fn backoff_for(attempt: usize, retry_after: Option<Duration>) -> Duration {
    if let Some(ra) = retry_after {
        return ra.min(Duration::from_secs(600));
    }
    let exp = attempt.min(30) as u32;
    let base_secs = 1u64 << exp;
    let base = Duration::from_secs(base_secs).min(Duration::from_secs(30));
    let jitter_max = (base.as_millis() as u64) / 5;
    let jitter = if jitter_max == 0 {
        0
    } else {
        random_u64() % jitter_max
    };
    base + Duration::from_millis(jitter)
}

fn random_u64() -> u64 {
    let mut buf = [0u8; 8];
    fill_random(&mut buf);
    u64::from_le_bytes(buf)
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
