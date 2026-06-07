package pages

// SaveKeyProps carries the data the /setup/save-key reveal page needs.
// Rendered once during onboarding, after admin creation, before the
// admin acknowledges they've stored the encryption key somewhere safe.
type SaveKeyProps struct {
	PageTitle string
	CSRFToken string
	// EncryptionKey is the raw 64-char hex string from cfg.EncryptionKey,
	// rendered into the masked reveal field.
	EncryptionKey string
	FlashType     string
	FlashMsg      string
}
