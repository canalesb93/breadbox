package cli

import (
	"context"
	"fmt"

	"breadbox/internal/config"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/spf13/cobra"
	"golang.org/x/crypto/bcrypt"
)

// pgText is a tiny convenience for the few CLI call sites that need
// pgtype.Text values directly.
func pgText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// AddCreateAdminCmd registers `breadbox create-admin`. Hidden in the help
// index because the typical first-run flow goes through the /setup wizard
// in the dashboard; this is the operator escape hatch.
func AddCreateAdminCmd(root *cobra.Command) {
	var username, password string
	cmd := &cobra.Command{
		Use:    "create-admin",
		Short:  "Create the first admin login account (local DB)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCreateAdmin(username, password)
		},
	}
	cmd.Flags().StringVar(&username, "username", "", "admin username")
	cmd.Flags().StringVar(&password, "password", "", "admin password (min 8 chars)")
	root.AddCommand(cmd)
}

func runCreateAdmin(username, password string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := db.New(pool)

	if username == "" {
		fmt.Print("Username: ")
		fmt.Scanln(&username)
	}
	if username == "" {
		return fmt.Errorf("username is required")
	}

	if password == "" {
		fmt.Print("Password (min 8 characters): ")
		fmt.Scanln(&password)

		fmt.Print("Confirm password: ")
		var confirm string
		fmt.Scanln(&confirm)
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	if _, err := queries.GetAuthAccountByUsername(ctx, username); err == nil {
		return fmt.Errorf("account with username %q already exists", username)
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = queries.CreateAuthAccount(ctx, db.CreateAuthAccountParams{
		Username:       username,
		HashedPassword: hashedPassword,
		Role:           "admin",
	})
	if err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}

	fmt.Printf("Admin account created successfully: %s\n", username)
	return nil
}

// AddResetPasswordCmd registers `breadbox reset-password`. Hidden — same
// rationale as create-admin.
func AddResetPasswordCmd(root *cobra.Command) {
	var password string
	cmd := &cobra.Command{
		Use:    "reset-password",
		Short:  "Reset the first admin account's password (local DB)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runResetPassword(password)
		},
	}
	cmd.Flags().StringVar(&password, "password", "", "new password (min 8 chars)")
	root.AddCommand(cmd)
}

func runResetPassword(password string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("connect to database: %w", err)
	}
	defer pool.Close()

	queries := db.New(pool)

	count, err := queries.CountAuthAccounts(ctx)
	if err != nil {
		return fmt.Errorf("check admin accounts: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("no admin account found — run the setup wizard first")
	}

	var adminID pgtype.UUID
	var adminUsername string
	row := pool.QueryRow(ctx, "SELECT id, username FROM auth_accounts WHERE role = 'admin' ORDER BY created_at LIMIT 1")
	if err := row.Scan(&adminID, &adminUsername); err != nil {
		return fmt.Errorf("get admin account: %w", err)
	}

	if password == "" {
		fmt.Printf("Resetting password for admin: %s\n", adminUsername)
		fmt.Print("New password (min 8 characters): ")
		fmt.Scanln(&password)
		fmt.Print("Confirm password: ")
		var confirm string
		fmt.Scanln(&confirm)
		if password != confirm {
			return fmt.Errorf("passwords do not match")
		}
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	if err := queries.UpdateAuthAccountPassword(ctx, db.UpdateAuthAccountPasswordParams{
		ID:             adminID,
		HashedPassword: hashedPassword,
	}); err != nil {
		return fmt.Errorf("update password: %w", err)
	}

	fmt.Printf("Password updated successfully for admin: %s\n", adminUsername)
	return nil
}
