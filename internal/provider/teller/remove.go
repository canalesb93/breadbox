package teller

import (
	"context"
	"fmt"
	"net/http"

	"breadbox/internal/crypto"
	"breadbox/internal/provider"
)

// RemoveConnection revokes Teller's access to the enrollment by calling
// DELETE /enrollments/{enrollment_id}. If the API returns an error (e.g.,
// the token is already invalid), the error is logged and nil is returned
// so the caller can proceed with local cleanup.
func (p *TellerProvider) RemoveConnection(ctx context.Context, conn provider.Connection) error {
	accessToken, err := crypto.Decrypt(conn.EncryptedCredentials, p.encryptionKey)
	if err != nil {
		return fmt.Errorf("teller: decrypt access token for removal: %w", err)
	}

	path := fmt.Sprintf("/enrollments/%s", conn.ExternalID)
	resp, err := p.client.doWithAuth(ctx, http.MethodDelete, path, string(accessToken), nil)
	if err != nil {
		p.logger.WarnContext(ctx, "teller enrollment delete failed (token may already be invalid)",
			"enrollment_id", conn.ExternalID,
			"error", err,
		)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		p.logger.WarnContext(ctx, "teller enrollment delete returned non-success status",
			"enrollment_id", conn.ExternalID,
			"status", resp.StatusCode,
		)
	}

	return nil
}
