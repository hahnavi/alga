use alga_agent_sdk::MessageDedup;
use std::time::Duration;

#[test]
fn basic_dedup() {
    let d = MessageDedup::new(100, Duration::from_secs(60));
    assert!(
        !d.is_duplicate("a"),
        "first sighting should not be a duplicate"
    );
    assert!(d.is_duplicate("a"), "second sighting should be a duplicate");
    assert!(
        !d.is_duplicate("b"),
        "first sighting of a new id should not be a duplicate"
    );
}

/// Reproduces the original bug where a just-accepted ID could be evicted by its
/// own insertion when the cache was at capacity, letting an immediate replay be
/// treated as new.
#[test]
fn no_evict_on_insert_at_capacity() {
    let d = MessageDedup::new(3, Duration::from_secs(60));
    // Fill to capacity.
    d.is_duplicate("id-a");
    d.is_duplicate("id-b");
    d.is_duplicate("id-c");
    // At capacity: the next unique insert must still be recorded, and an
    // immediate replay must be detected as a duplicate (not evicted).
    assert!(
        !d.is_duplicate("id-new"),
        "first sighting should not be a duplicate"
    );
    assert!(
        d.is_duplicate("id-new"),
        "id-new was evicted by its own insertion — replay not detected"
    );
}

#[test]
fn ttl_expiry() {
    let d = MessageDedup::new(100, Duration::from_millis(20));
    d.is_duplicate("ephemeral");
    std::thread::sleep(Duration::from_millis(30));
    assert!(
        !d.is_duplicate("ephemeral"),
        "id should have expired and been re-accepted"
    );
}
