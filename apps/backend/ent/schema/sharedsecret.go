package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/google/uuid"
)

// SharedSecret is one credential that agents fetch by secret_id via the agent
// bearer API. For a provider of type "internal", the plaintext value is stored
// encrypted in value_encrypted. For external providers, remote_ref holds the
// backend-specific path/key that Alga resolves through the provider at read
// time. allowed_agent_ids lists the agent token IDs permitted to fetch the
// secret; a secret is always restricted to this list, and an empty list means
// no agent may fetch it until at least one is added.
type SharedSecret struct {
	ent.Schema
}

func (SharedSecret) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "shared_secrets"},
	}
}

func (SharedSecret) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).StorageKey("id"),
		field.UUID("provider_id", uuid.UUID{}),
		field.String("name").NotEmpty(),
		// secret_id is the stable identifier agents use to fetch the secret.
		field.String("secret_id").NotEmpty(),
		field.String("description").Default(""),
		// remote_ref is the backend path for external providers (e.g. a vault
		// path or AWS secret ARN). Empty for internal providers.
		field.String("remote_ref").Default(""),
		// value_encrypted holds the AEAD ciphertext for internal secrets.
		field.String("value_encrypted").Default(""),
		// value_configured mirrors whether a plaintext value is stored, so list
		// views never need to touch the ciphertext to know if it is set.
		field.Bool("value_configured").Default(false),
		// allowed_agent_ids is a JSON array of agent token IDs permitted to
		// fetch the secret. A secret is always restricted to this list; an
		// empty list means no agent may fetch it until at least one is added.
		field.JSON("allowed_agent_ids", []uuid.UUID{}).Optional(),
		field.Time("created_at").Default(timeNow),
		field.Time("updated_at").Default(timeNow).UpdateDefault(timeNow),
	}
}

func (SharedSecret) Edges() []ent.Edge {
	return nil
}

func (SharedSecret) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("secret_id").Unique(),
		index.Fields("provider_id"),
	}
}
