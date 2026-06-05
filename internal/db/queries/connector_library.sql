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
INSERT INTO connector_library (name, url, transport, note, header_names, header_values_ciphertext)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: UpdateConnectorLibrary :one
UPDATE connector_library
SET name                      = $2,
    url                       = $3,
    transport                 = $4,
    note                      = $5,
    header_names              = $6,
    header_values_ciphertext  = $7,
    updated_at                = NOW()
WHERE id = $1
RETURNING *;

-- name: DeleteConnectorLibrary :execrows
DELETE FROM connector_library WHERE id = $1;
