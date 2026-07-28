use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{Duration, Instant};

pub struct MessageDedup {
    seen: Mutex<HashMap<String, Instant>>,
    max: usize,
    ttl: Duration,
}

impl MessageDedup {
    pub fn new(max_entries: usize, ttl: Duration) -> Self {
        let max = max_entries.max(1);
        let ttl = if ttl.is_zero() { Duration::from_secs(300) } else { ttl };
        Self {
            seen: Mutex::new(HashMap::new()),
            max,
            ttl,
        }
    }

    pub fn is_duplicate(&self, message_id: &str) -> bool {
        let mut seen = self.seen.lock().expect("dedup mutex poisoned");
        let now = Instant::now();

        seen.retain(|_, ts| now.duration_since(*ts) < self.ttl);

        if seen.len() >= self.max {
            let target = (self.max * 9) / 10;
            Self::trim_to(&mut seen, target);
        }

        if seen.contains_key(message_id) {
            seen.insert(message_id.to_string(), now);
            return true;
        }

        seen.insert(message_id.to_string(), now);
        false
    }

    pub fn clear(&self) {
        self.seen.lock().expect("dedup mutex poisoned").clear();
    }

    pub fn size(&self) -> usize {
        self.seen.lock().expect("dedup mutex poisoned").len()
    }

    fn trim_to(seen: &mut HashMap<String, Instant>, target: usize) {
        let target = target.min(seen.len());
        while seen.len() > target {
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
