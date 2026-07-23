package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"alga/config"
	"alga/crypto"
	"alga/pgclient"
	"alga/store"
)

var webhookTokenCmd = &cobra.Command{
	Use:   "webhook-token",
	Short: "Manage webhook authentication tokens",
}

var webhookTokenGenerateCmd = &cobra.Command{
	Use:   "generate <name>",
	Short: "Generate a new webhook token",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		webhookTokenStore := connectWebhookTokenStore(cfg)
		defer webhookTokenStore.Close()

		record, err := webhookTokenStore.CreateToken(args[0], nil)
		if err != nil {
			log.Fatalf("Failed to create webhook token: %v", err)
		}

		fmt.Printf("Webhook token created successfully:\n")
		fmt.Printf("  ID:    %s\n", record.ID.String())
		fmt.Printf("  Name:  %s\n", record.Name)
		fmt.Printf("  Token: %s (save this - it won't be shown again)\n", record.Token)
		fmt.Printf("\nUse this token in the Authorization header:\n")
		fmt.Printf("  Authorization: Bearer <token>\n")
	},
}

var webhookTokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List active webhook tokens",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		webhookTokenStore := connectWebhookTokenStore(cfg)
		defer webhookTokenStore.Close()

		tokens, err := webhookTokenStore.ListTokens()
		if err != nil {
			log.Fatalf("Failed to list webhook tokens: %v", err)
		}

		if len(tokens) == 0 {
			fmt.Println("No active webhook tokens found.")
			return
		}

		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		_, _ = fmt.Fprintln(w, "ID\tNAME\tCREATED")
		for _, t := range tokens {
			_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", t.ID.String(), t.Name, t.CreatedAt.Format(time.RFC3339))
		}
		_ = w.Flush()
	},
}

var webhookTokenRevokeCmd = &cobra.Command{
	Use:   "revoke <id>",
	Short: "Revoke a webhook token by its ID",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		webhookTokenStore := connectWebhookTokenStore(cfg)
		defer webhookTokenStore.Close()

		id, err := uuid.Parse(args[0])
		if err != nil {
			log.Fatalf("Invalid webhook token ID: %v", err)
		}

		if err := webhookTokenStore.RevokeToken(id); err != nil {
			log.Fatalf("Failed to revoke webhook token: %v", err)
		}

		fmt.Println("Webhook token revoked successfully.")
	},
}

var alertsCmd = &cobra.Command{
	Use:   "alerts",
	Short: "Query alerts from the database",
}

var dbCmd = &cobra.Command{
	Use:   "db",
	Short: "Database maintenance commands",
}

var dataCmd = &cobra.Command{
	Use:   "data",
	Short: "Data management commands",
}

var pruneCmd = &cobra.Command{
	Use:   "prune",
	Short: "Prune old resolved alerts",
	RunE:  runPrune,
}

var pruneDryRun bool
var pruneDays int

var dbMigrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Run database migrations",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := pgclient.ApplyMigrations(ctx, cfg.PostgresDSN); err != nil {
			log.Fatalf("Failed to run Postgres migrations: %v", err)
		}
		fmt.Println("Postgres migrations applied successfully.")
	},
}

var alertsQueryCmd = &cobra.Command{
	Use:   "query",
	Short: "Query alerts with flags",
	Long: `Query alerts using flag-based filters.

Examples:
  alga alerts query --status firing
  alga alerts query --status firing --limit 10 --sort -updated_at
  alga alerts query --channel backend --skip 5
  alga alerts query --search HighCPU --severity critical`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		alertStore := connectAlertStore(cfg)
		defer alertStore.Close()

		filter := map[string]any{}
		if v, _ := cmd.Flags().GetString("status"); v != "" {
			filter["status"] = v
		}
		if v, _ := cmd.Flags().GetString("channel"); v != "" {
			filter["channel"] = v
		}
		if v, _ := cmd.Flags().GetString("provider"); v != "" {
			filter["provider"] = v
		}
		if v, _ := cmd.Flags().GetString("severity"); v != "" {
			filter["severity"] = v
		}
		if v, _ := cmd.Flags().GetString("search"); v != "" {
			filter["search"] = v
		}
		if v, _ := cmd.Flags().GetString("start_date"); v != "" {
			filter["start_date"] = v
		}
		if v, _ := cmd.Flags().GetString("end_date"); v != "" {
			filter["end_date"] = v
		}

		limit, _ := cmd.Flags().GetInt64("limit")
		if limit == 0 {
			limit = 20
		}
		filter["$limit"] = limit

		skip, _ := cmd.Flags().GetInt64("skip")
		if skip > 0 {
			filter["$skip"] = skip
		}

		sortStr, _ := cmd.Flags().GetString("sort")
		if sortStr != "" {
			filter["$sort"] = sortStr
		}

		results, err := alertStore.QueryAlerts(filter)
		if err != nil {
			log.Fatalf("Query failed: %v", err)
		}

		output, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			log.Fatalf("Failed to marshal results: %v", err)
		}
		fmt.Println(string(output))
	},
}

var userCmd = &cobra.Command{
	Use:   "user",
	Short: "User management commands",
}

var userResetPasswordCmd = &cobra.Command{
	Use:   "reset-password <email>",
	Short: "Reset a user's password (for environments without email)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		userStore := connectUserStore(cfg)
		defer userStore.Close()

		user, err := userStore.GetByEmail(args[0])
		if err != nil {
			log.Fatalf("Failed to look up user: %v", err)
		}
		if user == nil {
			log.Fatalf("No user found with email %q", args[0])
		}

		newPassword, _ := cmd.Flags().GetString("password")
		if newPassword == "" {
			b := make([]byte, 24)
			if _, err := rand.Read(b); err != nil {
				log.Fatalf("Failed to generate random password: %v", err)
			}
			newPassword = base64.RawStdEncoding.EncodeToString(b)
		}

		hash, err := crypto.HashPassword(newPassword)
		if err != nil {
			log.Fatalf("Failed to hash password: %v", err)
		}

		if err := userStore.UpdateUser(user.ID, map[string]any{
			"password":              hash,
			"failed_login_attempts": 0,
			"locked_until":          nil,
		}); err != nil {
			log.Fatalf("Failed to update password: %v", err)
		}

		fmt.Printf("Password reset successfully for %s\n", user.Email)
		fmt.Printf("New password: %s\n", newPassword)
	},
}

var seedCmd = &cobra.Command{
	Use:   "seed",
	Short: "Seed sample data for demo purposes",
	Run: func(cmd *cobra.Command, args []string) {
		cfg := loadConfig()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		cli, err := pgclient.New(cfg.PostgresDSN)
		if err != nil {
			log.Fatalf("Failed to connect to Postgres: %v", err)
		}
		defer cli.Close()

		stores, err := store.NewStores(cli, 24*time.Hour, 0)
		if err != nil {
			log.Fatalf("Failed to init Postgres stores: %v", err)
		}

		alerts := []store.AlertRecord{
			{
				Fingerprint: "seed-critical-high-cpu",
				Status:      "firing",
				Labels: map[string]string{
					"alertname":  "HighCPUUsage",
					"severity":   "critical",
					"namespace":  "production",
					"pod":        "api-server-7d9f8c6b5-xk2lp",
					"deployment": "api-server",
					"instance":   "10.0.1.42:9090",
					"job":        "kubernetes-pods",
				},
				Annotations: map[string]string{
					"summary":     "API server CPU usage above 95%",
					"description": "Pod api-server-7d9f8c6b5-xk2lp has been using 97% CPU for the past 15 minutes.",
				},
				StartsAt: time.Now().UTC().Add(-15 * time.Minute),
			},
			{
				Fingerprint: "seed-warning-high-memory",
				Status:      "firing",
				Labels: map[string]string{
					"alertname":  "HighMemoryUsage",
					"severity":   "warning",
					"namespace":  "staging",
					"pod":        "worker-5b8d7f6c4-mn3pq",
					"deployment": "worker",
					"instance":   "10.0.2.18:9090",
					"job":        "kubernetes-pods",
				},
				Annotations: map[string]string{
					"summary":     "Worker memory usage above 80%",
					"description": "Pod worker-5b8d7f6c4-mn3pq memory usage is at 84%.",
				},
				StartsAt: time.Now().UTC().Add(-30 * time.Minute),
			},
			{
				Fingerprint: "seed-info-pod-restart",
				Status:      "firing",
				Labels: map[string]string{
					"alertname":  "PodRestartLoop",
					"severity":   "info",
					"namespace":  "monitoring",
					"pod":        "log-collector-6c4d3e2f1-abcde",
					"deployment": "log-collector",
					"instance":   "10.0.3.7:9090",
					"job":        "kubernetes-pods",
				},
				Annotations: map[string]string{
					"summary":     "Log collector pod restarting",
					"description": "Pod log-collector-6c4d3e2f1-abcde has restarted 3 times in the last hour.",
				},
				StartsAt: time.Now().UTC().Add(-45 * time.Minute),
			},
			{
				Fingerprint: "seed-ack-disk-space",
				Status:      "firing",
				Labels: map[string]string{
					"alertname":  "DiskSpaceLow",
					"severity":   "warning",
					"namespace":  "production",
					"pod":        "db-primary-0",
					"deployment": "database",
					"instance":   "10.0.1.100:9090",
					"job":        "kubernetes-pods",
					"device":     "/dev/sda1",
				},
				Annotations: map[string]string{
					"summary":     "Database disk space below 15%",
					"description": "Node db-primary-0 disk usage is at 88%.",
				},
				StartsAt: time.Now().UTC().Add(-2 * time.Hour),
				InitialEvent: &store.AlertEvent{
					Type:      "acked",
					Timestamp: time.Now().UTC().Add(-1 * time.Hour),
					Source:    "user",
				},
			},
			{
				Fingerprint: "seed-resolved-http-errors",
				Status:      "resolved",
				Labels: map[string]string{
					"alertname":  "HighErrorRate",
					"severity":   "info",
					"namespace":  "production",
					"pod":        "gateway-8f7e6d5c4-jklmn",
					"deployment": "gateway",
					"instance":   "10.0.1.55:9090",
					"job":        "kubernetes-pods",
				},
				Annotations: map[string]string{
					"summary":     "Gateway 5xx error rate elevated",
					"description": "Gateway 5xx error rate was 12% for 5 minutes.",
				},
				StartsAt: time.Now().UTC().Add(-4 * time.Hour),
				EndsAt:   timePtr(time.Now().UTC().Add(-3 * time.Hour)),
			},
		}

		for i, a := range alerts {
			if a.Status == "firing" && a.Acknowledged || (a.InitialEvent != nil && a.InitialEvent.Type == "acked") {
				a.Acknowledged = true
			}
			if _, err := stores.Alert.Create(a); err != nil {
				log.Fatalf("Failed to seed alert %d: %v", i+1, err)
			}
			fmt.Printf("  Created alert: %s (%s)\n", a.Labels["alertname"], a.Status)
		}

		notes := []*store.KnowledgeNote{
			{
				Kind:         store.KnowledgeKindRunbook,
				Title:        "Runbook: High CPU",
				BodyMarkdown: "## High CPU Troubleshooting\n\n1. Check running processes: `top` or `htop`\n2. Review recent deployments for CPU regressions\n3. Scale horizontally if traffic spike\n4. Check for infinite loops in application logs\n5. Review resource limits in pod spec",
				Tags:         []string{"cpu", "performance", "runbook"},
				AuthorType:   store.KnowledgeAuthorUser,
				AuthorName:   "Admin",
				Selectors: []config.RouteCondition{
					{Field: "alertname", Operator: "contains", Value: "CPU"},
				},
			},
			{
				Kind:         store.KnowledgeKindRunbook,
				Title:        "Network Troubleshooting Guide",
				BodyMarkdown: "## Network Troubleshooting\n\n1. Verify DNS resolution: `nslookup` or `dig`\n2. Check connectivity: `curl` or `telnet`\n3. Review network policies in Kubernetes\n4. Check load balancer health checks\n5. Verify firewall rules and security groups",
				Tags:         []string{"network", "connectivity", "dns"},
				AuthorType:   store.KnowledgeAuthorUser,
				AuthorName:   "Admin",
				Selectors: []config.RouteCondition{
					{Field: "alertname", Operator: "contains", Value: "Network"},
				},
			},
			{
				Kind:         store.KnowledgeKindRunbook,
				Title:        "Deployment Checklist",
				BodyMarkdown: "## Pre-Deployment Checklist\n\n- [ ] Run full test suite\n- [ ] Review changelog\n- [ ] Notify stakeholders\n- [ ] Prepare rollback plan\n- [ ] Verify resource requests/limits\n- [ ] Check config map changes\n\n## Post-Deployment\n\n- [ ] Verify health endpoints\n- [ ] Monitor error rates for 30 minutes\n- [ ] Confirm alerting rules are active",
				Tags:         []string{"deployment", "checklist", "process"},
				AuthorType:   store.KnowledgeAuthorUser,
				AuthorName:   "Admin",
			},
		}

		for i, n := range notes {
			if _, err := stores.Knowledge.Create(ctx, n); err != nil {
				log.Fatalf("Failed to seed knowledge note %d: %v", i+1, err)
			}
			fmt.Printf("  Created knowledge note: %s\n", n.Title)
		}

		fmt.Println("\nSeed completed successfully.")
	},
}

var triageCmd = &cobra.Command{
	Use:   "triage",
	Short: "Triage operations",
}

var triageStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show triage accuracy and volume stats",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("load config: %w", err)
		}
		pgClient, err := pgclient.New(cfg.PostgresDSN)
		if err != nil {
			return fmt.Errorf("connect to postgres: %w", err)
		}
		defer pgClient.Close()
		if cfg.PostgresAutoMigrate {
			if err := pgclient.ApplyMigrations(cmd.Context(), cfg.PostgresDSN); err != nil {
				return fmt.Errorf("apply migrations: %w", err)
			}
		}
		triageStore := store.NewPostgresTriageResultStore(pgClient)
		confirmed, overridden, pending, err := triageStore.CountByOutcome(cmd.Context())
		if err != nil {
			return fmt.Errorf("count by outcome: %w", err)
		}
		byDecision, _ := triageStore.CountByDecision(cmd.Context())
		accuracy := float64(0)
		if confirmed+overridden > 0 {
			accuracy = float64(confirmed) / float64(confirmed+overridden) * 100
		}
		fmt.Printf("Triage Stats:\n")
		fmt.Printf("  Total: %d (confirmed: %d, overridden: %d, pending: %d)\n", confirmed+overridden+pending, confirmed, overridden, pending)
		fmt.Printf("  Accuracy: %.1f%%\n", accuracy)
		fmt.Printf("  By decision: %v\n", byDecision)
		return nil
	},
}

func init() {
	webhookTokenCmd.AddCommand(webhookTokenGenerateCmd)
	webhookTokenCmd.AddCommand(webhookTokenListCmd)
	webhookTokenCmd.AddCommand(webhookTokenRevokeCmd)
	rootCmd.AddCommand(webhookTokenCmd)

	userResetPasswordCmd.Flags().String("password", "", "New password (generates random if omitted)")
	userCmd.AddCommand(userResetPasswordCmd)
	rootCmd.AddCommand(userCmd)

	alertsCmd.AddCommand(alertsQueryCmd)
	alertsQueryCmd.Flags().String("status", "", "Filter by status (firing, resolved)")
	alertsQueryCmd.Flags().String("channel", "", "Filter by channel")
	alertsQueryCmd.Flags().String("provider", "", "Filter by provider")
	alertsQueryCmd.Flags().String("severity", "", "Filter by severity")
	alertsQueryCmd.Flags().String("search", "", "Search in alertname and labels")
	alertsQueryCmd.Flags().String("start_date", "", "Start date (RFC3339)")
	alertsQueryCmd.Flags().String("end_date", "", "End date (RFC3339)")
	alertsQueryCmd.Flags().Int64("limit", 20, "Max results")
	alertsQueryCmd.Flags().Int64("skip", 0, "Skip results")
	alertsQueryCmd.Flags().String("sort", "-updated_at", "Sort field (prefix with - for desc)")
	rootCmd.AddCommand(alertsCmd)

	dbCmd.AddCommand(dbMigrateCmd)
	rootCmd.AddCommand(dbCmd)

	pruneCmd.Flags().BoolVar(&pruneDryRun, "dry-run", false, "Count alerts without deleting")
	pruneCmd.Flags().IntVar(&pruneDays, "days", 0, "Override retention days (0 = use config)")
	dataCmd.AddCommand(pruneCmd)
	dataCmd.AddCommand(cleanupDeletedCmd)
	rootCmd.AddCommand(dataCmd)

	rootCmd.AddCommand(seedCmd)

	triageCmd.AddCommand(triageStatsCmd)
	rootCmd.AddCommand(triageCmd)
}

func runPrune(cmd *cobra.Command, args []string) error {
	cfg := loadConfig()
	alertStore := connectAlertStore(cfg)
	defer alertStore.Close()

	days := pruneDays
	if days <= 0 {
		days = cfg.DataRetentionDays
	}
	if days <= 0 {
		return errors.New("no retention days configured; set DATA_RETENTION_DAYS or use --days")
	}

	cutoff := time.Now().UTC().AddDate(0, 0, -days)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if pruneDryRun {
		n, err := alertStore.CountOlderThan(ctx, cutoff)
		if err != nil {
			return fmt.Errorf("failed to count alerts: %w", err)
		}
		fmt.Printf("Would delete %d resolved alerts older than %s (dry run)\n", n, cutoff.Format(time.RFC3339))
		return nil
	}

	n, err := alertStore.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		return fmt.Errorf("failed to delete alerts: %w", err)
	}
	fmt.Printf("Deleted %d resolved alerts older than %s\n", n, cutoff.Format(time.RFC3339))
	return nil
}
