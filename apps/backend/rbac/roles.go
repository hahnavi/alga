package rbac

import (
	"slices"
	"strings"
)

var rolePermissions = map[string][]Permission{
	"admin": {
		AlertsRead, AlertsWrite, AlertsDelete,
		KnowledgeRead, KnowledgeWrite, KnowledgeDelete,
		RoutesRead, RoutesWrite,
		IntegrationsRead, IntegrationsWrite, IntegrationsTest,
		UsersManage, TokensManage,
		DashboardRead, ChannelsRead, AuditRead,
		NotificationsRead, NotificationsWrite,
		MemoriesRead, MemoriesWrite, MemoriesDelete,
		SystemConfigRead, SystemConfigWrite,
		TriageRead, TriageWrite, TriageOverride,
		IncidentsRead, IncidentsWrite, IncidentsCommand, IncidentsDelete,
		ServicesRead, ServicesWrite,
		OnCallRead, OnCallWrite,
		EscalationRead, EscalationWrite,
		PostMortemsRead, PostMortemsWrite, PostMortemsDelete,
		PlaybookRead, PlaybookWrite, PlaybookDelete,
		HeartbeatsRead, HeartbeatsWrite, HeartbeatsDelete,
		StatusPagesRead, StatusPagesWrite, StatusPagesDelete,
		OIDCManage,
		CredentialSecretsManage, CredentialSecretsRead,
		AdminAccess,
	},
	"operator": {
		AlertsRead, AlertsWrite,
		KnowledgeRead, KnowledgeWrite,
		RoutesRead,
		IntegrationsRead,
		DashboardRead, ChannelsRead, AuditRead,
		NotificationsRead, NotificationsWrite,
		MemoriesRead, MemoriesWrite,
		TriageRead, TriageWrite, TriageOverride,
		IncidentsRead, IncidentsWrite, IncidentsCommand,
		ServicesRead,
		OnCallRead,
		EscalationRead,
		PostMortemsRead, PostMortemsWrite,
		PlaybookRead, PlaybookWrite,
		HeartbeatsRead, HeartbeatsWrite,
		StatusPagesRead, StatusPagesWrite,
		CredentialSecretsRead, CredentialSecretsManage,
	},
	"viewer": {
		AlertsRead,
		KnowledgeRead,
		RoutesRead,
		IntegrationsRead,
		DashboardRead, ChannelsRead,
		NotificationsRead,
		MemoriesRead,
		TriageRead,
		IncidentsRead,
		ServicesRead,
		OnCallRead,
		EscalationRead,
		PostMortemsRead,
		PlaybookRead,
		HeartbeatsRead,
		StatusPagesRead,
	},
}

var validRoles = map[string]bool{
	"admin":    true,
	"operator": true,
	"viewer":   true,
}

func ValidRole(role string) bool {
	return validRoles[role]
}

func HasAnyPermission(role string, perms ...Permission) bool {
	granted, ok := rolePermissions[role]
	if !ok {
		return false
	}
	grantedSet := make(map[Permission]bool, len(granted))
	for _, p := range granted {
		grantedSet[p] = true
	}
	for _, p := range perms {
		if grantedSet[p] {
			return true
		}
	}
	return false
}

func AllPermissions(role string) []Permission {
	perms := rolePermissions[role]
	out := make([]Permission, len(perms))
	copy(out, perms)
	slices.SortFunc(out, func(a, b Permission) int {
		return strings.Compare(string(a), string(b))
	})
	return out
}
