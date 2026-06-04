-- name: ListConnectorLibrary :many
SELECT * FROM connector_library ORDER BY name;

-- name: GetConnectorLibraryByID :one
SELECT * FROM connector_library WHERE id = $1;

-- name: GetConnectorLibraryByShortID :one
SELECT * FROM connector_library WHERE short_id = $1;

-- name: GetConnectorLibraryByName :one
SELECT * FROM connector_library WHERE name = $1;

-- name: ListConnectorLibraryByNames :many
SELECT * FROM connector_library WHERE name = ANY($1::text[]) ORDER BY name;

-- name: CreateConnectorLibrary :one
INSERT INTO connector_library (name, url, header_name, secret_ciphertext)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: UpdateConnectorLibrary :one
UPDATE connector_library
SET name              = $2,
    url               = $3,
    header_name       = $4,
    secret_ciphertext = $5,
    updated_at        = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteConnectorLibrary :execrows
DELETE FROM connector_library WHERE id = $1;
