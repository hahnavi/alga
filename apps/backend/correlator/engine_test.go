package correlator

import (
	"testing"
	"time"
)

func TestCorrListKey(t *testing.T) {
	t.Parallel()
	got := corrListKey("prod:api:HighCPU")
	if got != "alga:corr:prod:api:HighCPU" {
		t.Fatalf("corrListKey = %q, want %q", got, "alga:corr:prod:api:HighCPU")
	}
}

func TestCooldownKey(t *testing.T) {
	t.Parallel()
	got := cooldownKey("prod:api:HighCPU")
	if got != "alga:cooldown:prod:api:HighCPU" {
		t.Fatalf("cooldownKey = %q, want %q", got, "alga:cooldown:prod:api:HighCPU")
	}
}

func TestFlushLockKey(t *testing.T) {
	t.Parallel()
	got := flushLockKey("prod:api:HighCPU")
	if got != "alga:corr-lock:prod:api:HighCPU" {
		t.Fatalf("flushLockKey = %q, want %q", got, "alga:corr-lock:prod:api:HighCPU")
	}
}

func TestNewCorrelatorDefaultCooldownTTL(t *testing.T) {
	t.Parallel()
	c := NewCorrelator(nil, nil, Config{Window: 0})
	if c.cfg.CooldownTTL != 30*time.Minute {
		t.Fatalf("CooldownTTL = %v, want 30m", c.cfg.CooldownTTL)
	}
}

func TestNewCorrelatorPreservesCooldownTTL(t *testing.T) {
	t.Parallel()
	dur := 15 * time.Minute
	c := NewCorrelator(nil, nil, Config{Window: 0, CooldownTTL: dur})
	if c.cfg.CooldownTTL != dur {
		t.Fatalf("CooldownTTL = %v, want %v", c.cfg.CooldownTTL, dur)
	}
}

func TestNewCorrelatorWindowZero(t *testing.T) {
	t.Parallel()
	c := NewCorrelator(nil, nil, Config{Window: 0})
	if c.Window() != 0 {
		t.Fatalf("Window = %v, want 0", c.Window())
	}
}

func TestNewCorrelatorWindowNonZero(t *testing.T) {
	t.Parallel()
	c := NewCorrelator(nil, nil, Config{Window: 5 * time.Minute})
	if c.Window() != 5*time.Minute {
		t.Fatalf("Window = %v, want 5m", c.Window())
	}
}

func TestCorrelationKeyDaemonset(t *testing.T) {
	t.Parallel()
	key, disc := CorrelationKey(map[string]string{
		"namespace": "prod",
		"daemonset": "log-collector",
		"alertname": "PodCrashLooping",
	})
	if key != "prod:log-collector:PodCrashLooping" {
		t.Fatalf("key = %q, want %q", key, "prod:log-collector:PodCrashLooping")
	}
	if disc["daemonset"] != "log-collector" {
		t.Fatalf("disc[daemonset] = %q, want %q", disc["daemonset"], "log-collector")
	}
}

func TestCorrelationKeyJob(t *testing.T) {
	t.Parallel()
	key, _ := CorrelationKey(map[string]string{
		"namespace": "prod",
		"job":       "batch-export",
		"alertname": "JobFailed",
	})
	if key != "prod:batch-export:JobFailed" {
		t.Fatalf("key = %q, want %q", key, "prod:batch-export:JobFailed")
	}
}

func TestCorrelationKeyOnlyAlertname(t *testing.T) {
	t.Parallel()
	key, disc := CorrelationKey(map[string]string{
		"alertname": "Watchdog",
	})
	if key != "Watchdog" {
		t.Fatalf("key = %q, want %q", key, "Watchdog")
	}
	if disc["alertname"] != "Watchdog" {
		t.Fatalf("disc[alertname] = %q", disc["alertname"])
	}
}

func TestCorrelationKeyDeterministic(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"alertname": "Foo",
		"namespace": "prod",
	}
	k1, _ := CorrelationKey(labels)
	k2, _ := CorrelationKey(labels)
	if k1 != k2 {
		t.Fatalf("same labels produced different keys: %q vs %q", k1, k2)
	}
}

func TestCorrelationKeyUnkeyedDeterministic(t *testing.T) {
	t.Parallel()
	labels := map[string]string{
		"region": "us-east",
		"team":   "platform",
	}
	k1, _ := CorrelationKey(labels)
	k2, _ := CorrelationKey(labels)
	if k1 != k2 {
		t.Fatalf("unkeyed labels produced different keys: %q vs %q", k1, k2)
	}
	if k1 == "unkeyed:e3b0c44298fc1c14" {
		t.Fatalf("unkeyed hash should differ from empty-labels hash")
	}
}

func TestCorrelationKeyDifferentLabelsDifferentUnkeyed(t *testing.T) {
	t.Parallel()
	k1, _ := CorrelationKey(map[string]string{"region": "us-east"})
	k2, _ := CorrelationKey(map[string]string{"region": "us-west"})
	if k1 == k2 {
		t.Fatalf("different labels must not produce same unkeyed key")
	}
}
