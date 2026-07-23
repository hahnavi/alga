package metrics

import "expvar"

// Webhook alert async publish outcomes (one increment per Grafana webhook POST).
var (
	WebhookAlertPublishQueued        = expvar.NewInt("alga_webhook_alert_publish_queued_total")
	WebhookAlertPublishSyncFallback  = expvar.NewInt("alga_webhook_alert_publish_sync_fallback_total")
	WebhookAlertPublishSyncProcessed = expvar.NewInt("alga_webhook_alert_publish_sync_processed_total")
)
