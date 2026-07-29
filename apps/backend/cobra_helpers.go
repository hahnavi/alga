package main

import (
	"log"
	"time"

	"alga/config"
	"alga/db"
	"alga/store"
)

func timePtr(t time.Time) *time.Time {
	return &t
}

func loadConfig() *config.Config {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}
	return cfg
}

func connectWebhookTokenStore(cfg *config.Config) store.WebhookTokenStore {
	cli, err := db.New(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	stores, err := store.NewStores(cli, 24*time.Hour, 0)
	if err != nil {
		cli.Close()
		log.Fatalf("Failed to init Postgres stores: %v", err)
	}
	return stores.WebhookToken
}

func connectAlertStore(cfg *config.Config) store.Store {
	cli, err := db.New(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	stores, err := store.NewStores(cli, 24*time.Hour, 0)
	if err != nil {
		cli.Close()
		log.Fatalf("Failed to init Postgres stores: %v", err)
	}
	return stores.Alert
}

type userStoreWithClose struct {
	store.UserStore
	close func()
}

func (u userStoreWithClose) Close() { u.close() }

func connectUserStore(cfg *config.Config) userStoreWithClose {
	cli, err := db.New(cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("Failed to connect to Postgres: %v", err)
	}
	stores, err := store.NewStores(cli, 24*time.Hour, 0)
	if err != nil {
		cli.Close()
		log.Fatalf("Failed to init Postgres stores: %v", err)
	}
	return userStoreWithClose{UserStore: stores.User, close: cli.Close}
}
