use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};

/// A bounded, TTL-based dedup cache for SSE message IDs.
///
/// Eviction happens before insertion: expired entries are removed first, and if
/// the cache is still at capacity the oldest entries are trimmed down to 90% of
/// `max`. This guarantees a just-accepted ID is never evicted by its own
/// insertion (which would let an immediate replay be treated as new).
pub struct MessageDedup {
    seen: Mutex<HashMap<String, Instant>>,
    max: usize,
    ttl: Duration,
}

impl MessageDedup {
    pub fn new(max_entries: usize, ttl: Duration) -> Self {
        let max = max_entries.max(1);
        Self {
            seen: Mutex::new(HashMap::new()),
            max,
            ttl,
        }
    }

    /// Reports whether `message_id` has been observed within the TTL window.
    /// The first observation records the ID and returns false.
    pub fn is_duplicate(&self, message_id: &str) -> bool {
        let mut seen = self.seen.lock().expect("dedup mutex poisoned");
        let now = Instant::now();

        // Evict expired entries.
        seen.retain(|_, ts| now.duration_since(*ts) < self.ttl);

        // Trim by count BEFORE inserting so the new entry is never the victim
        // of its own capacity eviction.
        if seen.len() >= self.max {
            let target = (self.max * 9) / 10;
            Self::trim_to(&mut seen, target);
        }

        if seen.contains_key(message_id) {
            // Refresh the timestamp so the window reflects recent replay.
            seen.insert(message_id.to_string(), now);
            return true;
        }

        seen.insert(message_id.to_string(), now);
        false
    }

    pub fn clear(&self) {
        self.seen.lock().expect("dedup mutex poisoned").clear();
    }

    fn trim_to(seen: &mut HashMap<String, Instant>, target: usize) {
        let target = target.min(seen.len());
        while seen.len() > target {
            // Remove the single oldest entry; O(n) per removal is acceptable for
            // the bounded trim-from-max pass (10% of capacity at most).
            let oldest = seen
                .iter()
                .min_by_key(|(_, ts)| *ts)
                .map(|(k, _)| k.clone());
            match oldest {
                Some(key) => {
                    seen.remove(&key);
                }
                None => break,
            }
        }
    }
}
