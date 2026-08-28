package api

import (
	"testing"

	"github.com/google/uuid"
)

// TestMentionedUserIDs pins mention-id filtering before a mention
// notification is published: only distinct human-user UUIDs survive, the
// sender never notifies themself, and agent mentions stay activation-only.
func TestMentionedUserIDs(t *testing.T) {
	t.Parallel()

	userA := uuid.NewString()
	userB := uuid.NewString()

	tests := []struct {
		name     string
		mentions []string
		senderID string
		want     []string
	}{
		{
			name:     "user mentions kept",
			mentions: []string{"user:" + userA, "user:" + userB},
			senderID: "",
			want:     []string{userA, userB},
		},
		{
			name:     "sender excluded",
			mentions: []string{"user:" + userA, "user:" + userB},
			senderID: userA,
			want:     []string{userB},
		},
		{
			name:     "agent mentions dropped",
			mentions: []string{"agent:" + uuid.NewString(), "user:" + userA},
			senderID: "",
			want:     []string{userA},
		},
		{
			name:     "duplicates collapsed",
			mentions: []string{"user:" + userA, "user:" + userA},
			senderID: "",
			want:     []string{userA},
		},
		{
			name:     "malformed ids ignored",
			mentions: []string{"user:not-a-uuid", "user:", "someone", "user:" + userA},
			senderID: "",
			want:     []string{userA},
		},
		{
			name:     "no usable mentions",
			mentions: nil,
			senderID: "",
			want:     nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := mentionedUserIDs(tc.mentions, tc.senderID)
			if len(got) != len(tc.want) {
				t.Fatalf("mentionedUserIDs = %v, want %v", got, tc.want)
			}
			for i := range tc.want {
				if got[i] != tc.want[i] {
					t.Fatalf("mentionedUserIDs = %v, want %v", got, tc.want)
				}
			}
		})
	}
}
