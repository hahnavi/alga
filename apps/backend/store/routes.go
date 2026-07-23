package store

import (
	"context"
	"fmt"
	"time"

	"alga/config"

	"alga/ent"
	entschema "alga/ent/schema"
)

type RouteRulesStore interface {
	Get() ([]config.RouteConfig, error)
	Save(routes []config.RouteConfig) error
}

type pgRouteRulesStore struct {
	pgStoreBase
}

func newPGRouteRulesStore(client *ent.Client) RouteRulesStore {
	return &pgRouteRulesStore{pgStoreBase{client: client}}
}

func (s *pgRouteRulesStore) Get() ([]config.RouteConfig, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	r, err := s.client.RouteRules.Get(ctx, singletonUUID())
	if err != nil {
		if ent.IsNotFound(err) {
			return []config.RouteConfig{}, nil
		}
		return nil, fmt.Errorf("failed to query route rules: %w", err)
	}

	if r.Routes == nil {
		return []config.RouteConfig{}, nil
	}

	var out []config.RouteConfig
	for _, rc := range r.Routes {
		out = append(out, routeConfigFromSchema(rc))
	}
	if out == nil {
		out = []config.RouteConfig{}
	}
	return out, nil
}

func (s *pgRouteRulesStore) Save(routes []config.RouteConfig) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var schemaRoutes []entschema.RouteConfig
	for _, r := range routes {
		schemaRoutes = append(schemaRoutes, routeConfigToSchema(r))
	}

	sid := singletonUUID()
	existing, err := s.client.RouteRules.Get(ctx, sid)
	if err != nil && !ent.IsNotFound(err) {
		return fmt.Errorf("failed to check existing routes: %w", err)
	}

	if existing != nil {
		_, err = s.client.RouteRules.UpdateOneID(sid).
			SetRoutes(schemaRoutes).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
	} else {
		_, err = s.client.RouteRules.Create().
			SetID(sid).
			SetRoutes(schemaRoutes).
			SetUpdatedAt(time.Now().UTC()).
			Save(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to save route rules: %w", err)
	}
	return nil
}

func routeConfigToSchema(rc config.RouteConfig) entschema.RouteConfig {
	var targets []entschema.RouteTarget
	for _, t := range rc.Targets {
		targets = append(targets, entschema.RouteTarget{
			Provider: t.Provider,
			Channel:  t.Channel,
		})
	}
	return entschema.RouteConfig{
		MatchMode:  rc.MatchMode,
		Conditions: routeConditionsToSchema(rc.Conditions),
		Targets:    targets,
		Silenced:   rc.Silenced,
	}
}

func routeConfigFromSchema(rc entschema.RouteConfig) config.RouteConfig {
	var targets []config.RouteTarget
	for _, t := range rc.Targets {
		targets = append(targets, config.RouteTarget{
			Provider: t.Provider,
			Channel:  t.Channel,
		})
	}
	return config.RouteConfig{
		MatchMode:  rc.MatchMode,
		Conditions: routeConditionsFromSchema(rc.Conditions),
		Targets:    targets,
		Silenced:   rc.Silenced,
	}
}
