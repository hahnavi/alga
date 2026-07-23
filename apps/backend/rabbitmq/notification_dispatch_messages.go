package rabbitmq

const MaxNotificationDispatchRetries = 4

var notificationDispatchRetryQueues = map[int]string{
	1: QueueNotificationDispatchRetry1,
	2: QueueNotificationDispatchRetry2,
	3: QueueNotificationDispatchRetry3,
	4: QueueNotificationDispatchRetry4,
}

type NotificationDispatchMessage struct {
	EventEnvelope
	UserID           string         `json:"user_id"`
	IncidentNumber   int64          `json:"incident_number,omitempty"`
	NotificationType string         `json:"notification_type"`
	Title            string         `json:"title"`
	Message          string         `json:"message"`
	ResourceType     string         `json:"resource_type,omitempty"`
	ResourceID       string         `json:"resource_id,omitempty"`
	Channels         []string       `json:"channels,omitempty"`
	Level            int            `json:"level,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
	RetryCount       int            `json:"retry_count"`
	NotificationID   string         `json:"notification_id,omitempty"`
}
