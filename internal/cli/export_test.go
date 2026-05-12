//go:build !lite

package cli

import "context"

// RunInitForTest exposes the unexported runInit body to integration tests
// in the cli_test package. The function lives in a _test.go file so it
// never leaks into the public CLI API.
func RunInitForTest(ctx context.Context, envFile, email, password, userName string) error {
	return runInit(ctx, initOpts{
		NonInteractive: true,
		Email:          email,
		Password:       password,
		UserName:       userName,
		EnvFile:        envFile,
		BaseURL:        "http://localhost:8080",
	})
}
