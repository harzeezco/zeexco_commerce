package main

import (
	"context"
	"log"
	"os"

	"github.com/harzeezco/zeexco_commerce/internal/config"
	"github.com/harzeezco/zeexco_commerce/internal/platform/database"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("connect to database: %v", err)
	}
	defer pool.Close()
	if len(os.Args) == 2 && os.Args[1] == "status" {
		status, err := database.InspectSchema(ctx, pool)
		if err != nil {
			log.Fatalf("inspect database schema: %v", err)
		}
		log.Printf(
			"migration status tracking=%t tracking_columns=%q tracking_rows=%d tracked_version=%d dirty=%t users=%t whatsapp_identities=%t whatsapp_messages=%t whatsapp_resolution=%t password_hash=%t businesses=%t business_memberships=%t audit_events=%t refresh_sessions=%t whatsapp_onboarding_sessions=%t duplicate_email_groups=%d active_users_without_password=%d",
			status.MigrationTracking, status.MigrationTrackingColumns, status.MigrationTrackingRows,
			status.TrackedVersion, status.TrackingDirty, status.Users, status.WhatsAppIdentities,
			status.WhatsAppMessages, status.WhatsAppResolutionColumn, status.PasswordHashColumn, status.Businesses,
			status.BusinessMemberships, status.AuditEvents, status.RefreshSessions,
			status.WhatsAppOnboardingSessions,
			status.CaseInsensitiveDuplicateEmailSets, status.ActiveUsersWithoutPassword,
		)
		return
	}
	if len(os.Args) > 1 {
		log.Fatal("usage: go run ./cmd/migrate [status]")
	}

	report, err := database.Migrate(ctx, pool)
	if err != nil {
		log.Fatalf("apply database migrations: %v", err)
	}
	log.Printf("database migrations complete applied=%d baselined=%d", len(report.Applied), len(report.Baselined))
	if report.LegacyActiveUsersWithoutPassword > 0 {
		log.Printf(
			"warning: %d legacy ACTIVE user(s) have no password hash and require explicit account remediation",
			report.LegacyActiveUsersWithoutPassword,
		)
	}
}
