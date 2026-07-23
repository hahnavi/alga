package types

// GrafanaAlertingPayload represents the JSON payload sent by Grafana Alerting webhook
type GrafanaAlertingPayload struct {
	Receiver          string            `json:"receiver"`
	Status            string            `json:"status"` // "firing" or "resolved"
	OrgID             int               `json:"orgId"`
	Alerts            []Alert           `json:"alerts"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
	Version           string            `json:"version"`
	GroupKey          string            `json:"groupKey"`
	TruncatedAlerts   int               `json:"truncatedAlerts"`
	Title             string            `json:"title"`
	State             string            `json:"state"` // "alerting" or "ok"
	Message           string            `json:"message"`
}

// Alert represents a single alert in the Grafana payload
type Alert struct {
	Status       string            `json:"status"`                 // "firing" or "resolved"
	Acknowledged bool              `json:"acknowledged,omitempty"` // set when syncing from store (not from Grafana)
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     string            `json:"startsAt"` // ISO 8601 timestamp
	EndsAt       string            `json:"endsAt"`   // ISO 8601 timestamp
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
	SilenceURL   string            `json:"silenceURL"`
	DashboardURL string            `json:"dashboardURL"`
	PanelURL     string            `json:"panelURL"`
	Values       map[string]any    `json:"values"`
	ValueString  string            `json:"valueString"`
}

// MattermostAttachment represents a Slack-compatible message attachment
type MattermostAttachment struct {
	Fallback   string            `json:"fallback"`
	Color      string            `json:"color"`
	Pretext    string            `json:"pretext,omitempty"`
	AuthorName string            `json:"author_name,omitempty"`
	AuthorLink string            `json:"author_link,omitempty"`
	AuthorIcon string            `json:"author_icon,omitempty"`
	Title      string            `json:"title,omitempty"`
	TitleLink  string            `json:"title_link,omitempty"`
	Text       string            `json:"text,omitempty"`
	Fields     []AttachmentField `json:"fields,omitempty"`
	ImageURL   string            `json:"image_url,omitempty"`
	ThumbURL   string            `json:"thumb_url,omitempty"`
	Footer     string            `json:"footer,omitempty"`
	FooterIcon string            `json:"footer_icon,omitempty"`
}

// AttachmentField represents a field in a message attachment
type AttachmentField struct {
	Title string `json:"title"`
	Value any    `json:"value"`
	Short bool   `json:"short"`
}
