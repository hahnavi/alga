use std::fmt;

#[derive(Debug)]
pub enum AlgaError {
    Auth {
        status_code: u16,
        message: String,
    },
    Api {
        status_code: u16,
        message: String,
        retry_after: Option<std::time::Duration>,
    },
    Connection(String),
    Request(reqwest::Error),
    Json(serde_json::Error),
    /// The bearer token contained bytes that are illegal in an HTTP header value.
    InvalidToken,
}

impl fmt::Display for AlgaError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            AlgaError::Auth {
                status_code,
                message,
            } => {
                write!(f, "authentication failed ({}): {}", status_code, message)
            }
            AlgaError::Api {
                status_code,
                message,
                retry_after,
            } => match retry_after {
                Some(d) => write!(
                    f,
                    "api error ({}): {} (retry after {:.0}s)",
                    status_code,
                    message,
                    d.as_secs_f64()
                ),
                None => write!(f, "api error ({}): {}", status_code, message),
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
    /// Returns the retry delay the server requested via `Retry-After`, if any.
    /// Most useful for 429 responses.
    pub fn retry_after(&self) -> Option<std::time::Duration> {
        match self {
            AlgaError::Api { retry_after, .. } => *retry_after,
            _ => None,
        }
    }
}
