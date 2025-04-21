-- name: CreateEvent :one
INSERT INTO events (business_id, event_type, payload)
VALUES (?, ?, ?)
RETURNING id, business_id, event_type, payload, created_at, processed_at, status;

-- name: CreateInvoice :one
INSERT INTO invoices (id, business_id, amount, currency, status, description)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING id, business_id, amount, currency, status, description, created_at;

-- name: GetPendingEvents :many
SELECT id, business_id, event_type, payload, created_at, processed_at, status 
FROM events
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkEventAsProcessed :exec
UPDATE events
SET status = 'processed',
    processed_at = CURRENT_TIMESTAMP
WHERE id = ?; 