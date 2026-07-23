package platform

// BuildOwnerChatID formats the owner-scoped chat ID shared by the agent and
// operator thread/event paths (e.g. "alert_<n>", "incident_investigation_<id>").
func BuildOwnerChatID(ownerType, ownerID string) string {
	return ownerType + "_" + ownerID
}
