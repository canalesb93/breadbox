package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"breadbox/internal/config"
	"breadbox/internal/db"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

func runCreateAdmin() error {
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

	// Parse flags: --username and --password
	var username, password string
	for i := 2; i < len(os.Args); i++ {
		switch os.Args[i] {
		case "--username":
			if i+1 < len(os.Args) {
				username = os.Args[i+1]
				i++
			}
		case "--password":
			if i+1 < len(os.Args) {
				password = os.Args[i+1]
				i++
			}
		}
	}

	// Interactive prompts if flags not provided.
	if username == "" {
		fmt.Print("Username: ")
		fmt.Scanln(&username)
	}
	username = strings.TrimSpace(username)

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

	// Check if username already exists.
	_, err = queries.GetAdminAccountByUsername(ctx, username)
	if err == nil {
		return fmt.Errorf("admin account with username %q already exists", username)
	}

	// Hash password with bcrypt.
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	// Create admin account.
	_, err = queries.CreateAdminAccount(ctx, db.CreateAdminAccountParams{
		Username:       username,
		HashedPassword: hashedPassword,
	})
	if err != nil {
		return fmt.Errorf("create admin account: %w", err)
	}

	fmt.Printf("Admin account created successfully: %s\n", username)
	return nil
}
