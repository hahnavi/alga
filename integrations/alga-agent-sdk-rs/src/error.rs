use std::fmt;
use std::time::Duration;

#[derive(Debug)]
pub enum AlgaError {
    Auth { status_code: u16, message: String },
    Api {
        status_code: u16,
        message: String,
        retry_after: Option<Duration>,
    },
    Connection(String),
    Request(reqwest::Error),
    Json(serde_json::Error),
    InvalidToken,
}

impl fmt::Display for AlgaError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            AlgaError::Auth {
                status_code,
                message,
            } => write!(f, "auth error {}: {}", status_code, message),
            AlgaError::Api {
                status_code,
                message,
                retry_after,
            } => match retry_after {
                Some(d) => write!(
                    f,
                    "api error {}: {} (retry after {:.0}s)",
                    status_code,
                    message,
                    d.as_secs_f64()
                ),
                None => write!(f, "api error {}: {}", status_code, message),
            },
            AlgaError::Connection(s) => write!(f, "connection error: {}", s),
            AlgaError::Request(e) => write!(f, "request error: {}", e),
            AlgaError::Json(e) => write!(f, "json error: {}", e),
            AlgaError::InvalidToken => write!(
                f,
                "invalid bearer token: contains bytes illegal in an HTTP header value"
            ),
        }
    }
}

impl std::error::Error for AlgaError {}

impl From<reqwest::Error> for AlgaError {
    fn from(e: reqwest::Error) -> Self {
        AlgaError::Request(e)
    }
}

impl From<serde_json::Error> for AlgaError {
    fn from(e: serde_json::Error) -> Self {
        AlgaError::Json(e)
    }
}

impl AlgaError {
    pub fn is_retryable(&self) -> bool {
        match self {
            AlgaError::Auth { .. } => false,
            AlgaError::Api { status_code, .. } => matches!(
                status_code,
                429 | 500 | 502 | 503 | 504
            ),
            AlgaError::Connection(_) => true,
            AlgaError::Request(_) => true,
            AlgaError::Json(_) => false,
            AlgaError::InvalidToken => false,
        }
    }

    pub fn retry_after(&self) -> Option<Duration> {
        match self {
            AlgaError::Api { retry_after, .. } => *retry_after,
            _ => None,
        }
    }
}

pub fn is_auth_error(err: &AlgaError) -> bool {
    matches!(err, AlgaError::Auth { .. })
}

pub fn is_retryable_error(err: &AlgaError) -> bool {
    err.is_retryable()
}
