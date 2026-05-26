package pages

// SaveKeyProps carries the data the /setup/save-key reveal page needs.
// Rendered once during onboarding, after admin creation, before the
// admin acknowledges they've stored the encryption key somewhere safe.
type SaveKeyProps struct {
	PageTitle string
	CSRFToken string
	// EncryptionKey is the raw 64-char hex string from cfg.EncryptionKey.
	// Rendered into a masked input plus the encoded 1Password payload.
	EncryptionKey string
	// OnePasswordValue is the base64-encoded JSON save request the
	// vendored <onepassword-save-button> consumes. Pre-computed in the
	// handler so the template stays logic-free.
	OnePasswordValue string
	// ItemTitle is the title that will appear on the saved item (the
	// 1Password item title, and the suggested filename for the .env
	// download). Defaults to "Breadbox encryption key" plus the host.
	ItemTitle string
	FlashType string
	FlashMsg  string
}
