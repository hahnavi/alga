use crate::commands::InvestigationCommand;
use crate::dedup::MessageDedup;
use crate::error::AlgaError;
use crate::models::*;
use crate::sse::{EventHandler, SSEClient};
use reqwest::header::{HeaderMap, HeaderValue, AUTHORIZATION, CONTENT_TYPE};
use std::sync::Arc;
use std::time::Duration;

const MAX_RESPONSE_BODY: usize = 32 * 1024 * 1024;

pub struct AlgaClient {
    server_url: String,
    token: String,
    http_client: reqwest::Client,
    sse: Option<SSEClient>,
    dedup: Arc<MessageDedup>,
}

impl AlgaClient {
    pub fn new(server_url: &str, token: &str) -> Self {
        let dedup = Arc::new(MessageDedup::new(10000, Duration::from_secs(300)));
        Self {
            server_url: server_url.trim_end_matches('/').to_string(),
            token: token.to_string(),
            http_client: reqwest::Client::builder()
                .timeout(Duration::from_secs(30))
                .build()
                .expect("reqwest client with default TLS must build"),
            sse: None,
            dedup,
        }
    }

    pub fn connect(&mut self, handler: Arc<dyn EventHandler>) -> Result<(), AlgaError> {
        let mut sse = SSEClient::new(
            self.server_url.clone(),
            self.token.clone(),
            self.dedup.clone(),
            handler,
        );
        sse.start()?;
        self.sse = Some(sse);
        Ok(())
    }

    pub fn disconnect(&mut self) {
        if let Some(sse) = self.sse.take() {
            sse.stop();
        }
    }

    fn auth_headers(&self) -> Result<HeaderMap, AlgaError> {
        let mut headers = HeaderMap::new();
        let value = HeaderValue::from_str(&format!("Bearer {}", self.token))
            .map_err(|_| AlgaError::InvalidToken)?;
        headers.insert(AUTHORIZATION, value);
        Ok(headers)
    }

    fn json_headers(&self) -> Result<HeaderMap, AlgaError> {
        let mut headers = self.auth_headers()?;
        headers.insert(CONTENT_TYPE, HeaderValue::from_static("application/json"));
        Ok(headers)
    }

    fn agent_url(&self, path: &str) -> String {
        format!("{}{}", self.server_url, path)
    }

    async fn map_response<T: serde::de::DeserializeOwned>(
        &self,
        response: reqwest::Response,
    ) -> Result<T, AlgaError> {
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
        response.json::<T>().await.map_err(AlgaError::from)
    }

    async fn map_empty_response(&self, response: reqwest::Response) -> Result<(), AlgaError> {
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
        Ok(())
    }

    pub async fn list_alerts(
        &self,
        params: ListAlertsParams,
    ) -> Result<AlertListResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/alerts");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[
                ("status", params.status.as_deref()),
                ("severity", params.severity.as_deref()),
                ("provider", params.provider.as_deref()),
                ("search", params.search.as_deref()),
            ])
            .query(&[
                ("limit", params.limit.map(|v| v.to_string())),
                ("skip", params.skip.map(|v| v.to_string())),
            ])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_alert(&self, fingerprint: &str) -> Result<Alert, AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/alerts/{}",
            encode_path(fingerprint)
        ));
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn resolve_alert(&self, fingerprint: &str) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/alerts/{}/resolve",
            encode_path(fingerprint)
        ));
        let response = self
            .http_client
            .post(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn reopen_alert(&self, fingerprint: &str) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/alerts/{}/reopen",
            encode_path(fingerprint)
        ));
        let response = self
            .http_client
            .post(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn list_investigations(
        &self,
        params: ListInvestigationsParams,
    ) -> Result<InvestigationListResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/investigations");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[
                ("status", params.status.as_deref()),
                ("severity", params.severity.as_deref()),
                ("search", params.search.as_deref()),
                ("incident_id", params.incident_id.as_deref()),
            ])
            .query(&[
                ("limit", params.limit.map(|v| v.to_string())),
                ("skip", params.skip.map(|v| v.to_string())),
            ])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_investigation(&self, id: &str) -> Result<Investigation, AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/investigations/{}", encode_path(id)));
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn post_update(
        &self,
        investigation_id: &str,
        update_type: &str,
        message: &str,
    ) -> Result<Investigation, AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/investigations/{}/updates",
            encode_path(investigation_id)
        ));
        let body = serde_json::json!({
            "message": message,
            "type": update_type,
        });
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn send_message(
        &self,
        chat_id: &str,
        text: &str,
        mentions: Option<Vec<String>>,
    ) -> Result<SendMessageResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/messages");
        let mut body = serde_json::json!({
            "chat_id": chat_id,
            "kind": "text",
            "text": text,
        });
        if let Some(m) = mentions {
            body["mentions"] = serde_json::json!(m);
        }
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn send_command(
        &self,
        chat_id: &str,
        cmd: InvestigationCommand,
    ) -> Result<CommandResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/messages");
        let body = serde_json::json!({
            "chat_id": chat_id,
            "kind": "inv_tool",
            "command": cmd,
        });
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn edit_message(
        &self,
        message_id: &str,
        chat_id: &str,
        text: &str,
    ) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/messages/{}",
            encode_path(message_id)
        ));
        let body = serde_json::json!({
            "chat_id": chat_id,
            "kind": "text",
            "text": text,
        });
        let response = self
            .http_client
            .put(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn delete_message(&self, message_id: &str, chat_id: &str) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/messages/{}",
            encode_path(message_id)
        ));
        let body = serde_json::json!({
            "chat_id": chat_id,
        });
        let response = self
            .http_client
            .delete(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn send_typing(&self, chat_id: &str, active: bool) -> Result<(), AlgaError> {
        let url = self.agent_url("/api/v1/agent/typing");
        let body = serde_json::json!({
            "chat_id": chat_id,
            "active": active,
        });
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn send_heartbeat(&self) -> Result<(), AlgaError> {
        let url = self.agent_url("/api/v1/agent/heartbeat");
        let response = self
            .http_client
            .post(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn list_knowledge(
        &self,
        params: ListKnowledgeParams,
    ) -> Result<KnowledgeListResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/knowledge");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[
                ("search", params.search.as_deref()),
                ("tags", params.tags.as_deref()),
            ])
            .query(&[
                ("limit", params.limit.map(|v| v.to_string())),
                ("skip", params.skip.map(|v| v.to_string())),
            ])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn create_knowledge(
        &self,
        params: CreateKnowledgeParams,
    ) -> Result<KnowledgeNote, AlgaError> {
        let url = self.agent_url("/api/v1/agent/knowledge");
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&params)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn list_memories(
        &self,
        params: ListMemoriesParams,
    ) -> Result<MemoryListResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/memories");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[("search", params.search.as_deref())])
            .query(&[
                ("limit", params.limit.map(|v| v.to_string())),
                ("skip", params.skip.map(|v| v.to_string())),
            ])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn create_memory(&self, params: CreateMemoryParams) -> Result<Memory, AlgaError> {
        let url = self.agent_url("/api/v1/agent/memories");
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&params)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_memory(&self, id: &str) -> Result<Memory, AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/memories/{}", encode_path(id)));
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn delete_memory(&self, id: &str) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/memories/{}", encode_path(id)));
        let response = self
            .http_client
            .delete(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn list_peer_asks(
        &self,
        params: ListPeerAsksParams,
    ) -> Result<PeerAskListResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/peer-ask");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[("status", params.status.as_deref())])
            .query(&[
                ("limit", params.limit.map(|v| v.to_string())),
                ("skip", params.skip.map(|v| v.to_string())),
            ])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn create_peer_ask(&self, params: CreatePeerAskParams) -> Result<PeerAsk, AlgaError> {
        let url = self.agent_url("/api/v1/agent/peer-ask");
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&params)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_peer_ask(&self, id: &str) -> Result<PeerAsk, AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/peer-ask/{}", encode_path(id)));
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn reply_peer_ask(&self, id: &str, reply: &str) -> Result<PeerAsk, AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/peer-ask/{}/reply", encode_path(id)));
        let body = serde_json::json!({ "reply": reply });
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&body)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn cancel_peer_ask(&self, id: &str) -> Result<PeerAsk, AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/peer-ask/{}/cancel",
            encode_path(id)
        ));
        let response = self
            .http_client
            .post(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_incident(&self, id: &str) -> Result<Incident, AlgaError> {
        let url = self.agent_url(&format!("/api/v1/agent/incidents/{}", encode_path(id)));
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn add_incident_timeline(
        &self,
        incident_id: &str,
        params: AddTimelineEntryParams,
    ) -> Result<(), AlgaError> {
        let url = self.agent_url(&format!(
            "/api/v1/agent/incidents/{}/timeline",
            encode_path(incident_id)
        ));
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&params)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn list_services(&self) -> Result<Vec<Service>, AlgaError> {
        let url = self.agent_url("/api/v1/agent/services");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn who_is_on_call(&self) -> Result<OnCallResponse, AlgaError> {
        let url = self.agent_url("/api/v1/agent/on-call/current");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn send_incident_summary(
        &self,
        incident_id: &str,
        text: &str,
    ) -> Result<(), AlgaError> {
        let url = self.agent_url("/api/v1/agent/messages");
        let payload = serde_json::json!({
            "chat_id": format!("incident_coord_{}", incident_id),
            "kind": "incident_summary",
            "text": text,
        });
        let response = self
            .http_client
            .post(&url)
            .headers(self.json_headers()?)
            .json(&payload)
            .send()
            .await?;
        self.map_empty_response(response).await
    }

    pub async fn get_capabilities(&self) -> Result<Vec<Capability>, AlgaError> {
        let url = self.agent_url("/api/v1/agent/capabilities");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn get_playbooks(
        &self,
        alert_fingerprint: Option<&str>,
    ) -> Result<Vec<Playbook>, AlgaError> {
        let url = self.agent_url("/api/v1/agent/playbooks");
        let response = self
            .http_client
            .get(&url)
            .headers(self.auth_headers()?)
            .query(&[("alert_fingerprint", alert_fingerprint)])
            .send()
            .await?;
        self.map_response(response).await
    }

    pub async fn upload_media(
        &self,
        file_name: &str,
        content_type: &str,
        data: Vec<u8>,
    ) -> Result<SendMessageResponse, AlgaError> {
        if data.len() > MAX_RESPONSE_BODY {
            return Err(AlgaError::Connection(format!(
                "media upload of {} bytes exceeds the {} byte limit",
                data.len(),
                MAX_RESPONSE_BODY
            )));
        }
        let url = self.agent_url("/api/v1/agent/media");
        let part = reqwest::multipart::Part::bytes(data)
            .file_name(file_name.to_string())
            .mime_str(content_type)
            .map_err(|e| AlgaError::Connection(e.to_string()))?;
        let form = reqwest::multipart::Form::new().part("file", part);
        let response = self
            .http_client
            .post(&url)
            .headers(self.auth_headers()?)
            .multipart(form)
            .send()
            .await?;
        self.map_response(response).await
    }
}

/// Encode a path segment per RFC 3986. Replaces reqwest's URL encoding (which
/// percent-encodes `/` inside a segment) with the component-encoding the agent
/// API expects.
fn encode_path(segment: &str) -> String {
    let mut out = String::with_capacity(segment.len());
    for b in segment.bytes() {
        // unreserved per RFC 3986
        if b.is_ascii_alphanumeric() || matches!(b, b'-' | b'_' | b'.' | b'~') {
            out.push(b as char);
        } else {
            out.push_str(&format!("%{:02X}", b));
        }
    }
    out
}

/// Parse the `Retry-After` header into a duration. Supports both delta-seconds
/// and HTTP-date forms (the latter resolved against the response `Date` header
/// when present, otherwise ignored).
fn parse_retry_after(headers: &reqwest::header::HeaderMap) -> Option<Duration> {
    let raw = headers.get(reqwest::header::RETRY_AFTER)?.to_str().ok()?;
    if let Ok(secs) = raw.trim().parse::<u64>() {
        return Some(Duration::from_secs(secs));
    }
    // HTTP-date form
    let date = httpdate::parse_http_date(raw.trim()).ok()?;
    let now = std::time::SystemTime::now();
    let delta = date.duration_since(now).ok()?;
    Some(delta)
}
