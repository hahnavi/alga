package store

import (
	"context"
	"fmt"
	"time"

	"github.com/uptrace/bun"

	"alga/config"
	"alga/db/models"
)

type RouteRulesStore interface {
	Get() ([]config.RouteConfig, error)
	Save(routes []config.RouteConfig) error
}

type pgRouteRulesStore struct {
	pgStoreBase
}

func newPGRouteRulesStore(db *bun.DB) RouteRulesStore {
	return &pgRouteRulesStore{pgStoreBase{db: db}}
}

func (s *pgRouteRulesStore) Get() ([]config.RouteConfig, error) {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	r := new(models.RouteRules)
	err := s.db.NewSelect().Model(r).Where("id = ?", singletonUUID()).Scan(ctx)
	if err != nil {
		if isNotFound(err) {
			return []config.RouteConfig{}, nil
		}
		return nil, fmt.Errorf("failed to query route rules: %w", err)
	}

	if r.Routes == nil {
		return []config.RouteConfig{}, nil
	}

	var out []config.RouteConfig
	for _, rc := range r.Routes {
		out = append(out, routeConfigFromModels(rc))
	}
	if out == nil {
		out = []config.RouteConfig{}
	}
	return out, nil
}

func (s *pgRouteRulesStore) Save(routes []config.RouteConfig) error {
	ctx, cancel := pgctx(context.Background())
	defer cancel()

	var modelRoutes []models.RouteConfig
	for _, r := range routes {
		modelRoutes = append(modelRoutes, routeConfigToModels(r))
	}

	sid := singletonUUID()
	existing := new(models.RouteRules)
	err := s.db.NewSelect().Model(existing).Where("id = ?", sid).Scan(ctx)
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("failed to check existing routes: %w", err)
	}

	now := time.Now().UTC()
	if err == nil {
		_, err = s.db.NewUpdate().Model((*models.RouteRules)(nil)).
			Set("routes = ?", modelRoutes).
			Set("updated_at = ?", now).
			Where("id = ?", sid).
			Exec(ctx)
	} else {
		m := &models.RouteRules{
			ID:        sid,
			Routes:    modelRoutes,
			UpdatedAt: now,
		}
		_, err = s.db.NewInsert().Model(m).Exec(ctx)
	}
	if err != nil {
		return fmt.Errorf("failed to save route rules: %w", err)
	}
	return nil
}

func routeConfigToModels(rc config.RouteConfig) models.RouteConfig {
	var targets []models.RouteTarget
	for _, t := range rc.Targets {
		targets = append(targets, models.RouteTarget{
			Provider: t.Provider,
			Channel:  t.Channel,
		})
	}
	return models.RouteConfig{
		MatchMode:  rc.MatchMode,
		Conditions: routeConditionsToModels(rc.Conditions),
		Targets:    targets,
		Silenced:   rc.Silenced,
	}
}

func routeConfigFromModels(rc models.RouteConfig) config.RouteConfig {
	var targets []config.RouteTarget
	for _, t := range rc.Targets {
		targets = append(targets, config.RouteTarget{
			Provider: t.Provider,
			Channel:  t.Channel,
		})
	}
	return config.RouteConfig{
		MatchMode:  rc.MatchMode,
		Conditions: routeConditionsFromModels(rc.Conditions),
		Targets:    targets,
		Silenced:   rc.Silenced,
	}
}
