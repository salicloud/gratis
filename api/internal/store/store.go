package store

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Store handles all API-side persistence.
type Store struct {
	db *pgxpool.Pool
}

func New(ctx context.Context, dsn string) (*Store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping postgres: %w", err)
	}
	s := &Store{db: pool}
	if err := s.migrate(ctx); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *Store) Close() { s.db.Close() }

// ─── Tokens ──────────────────────────────────────────────────────────────────

// CreateToken generates a new provisioning token and stores its hash.
// Returns the plaintext token (shown once — not stored).
func (s *Store) CreateToken(ctx context.Context) (plaintext string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	plaintext = hex.EncodeToString(raw)
	hash := hashToken(plaintext)

	_, err = s.db.Exec(ctx,
		`INSERT INTO provisioning_tokens (token_hash, created_at) VALUES ($1, $2)`,
		hash, time.Now().UTC(),
	)
	return plaintext, err
}

// ValidateToken checks the token, marks it used, and returns the server ID
// to assign (generating one if this is a first-time registration).
// Returns ("", err) if the token is invalid or already used.
func (s *Store) ValidateToken(ctx context.Context, plaintext string) (serverID string, err error) {
	hash := hashToken(plaintext)

	var tokenID string
	var usedAt *time.Time
	var existingServerID *string

	err = s.db.QueryRow(ctx,
		`SELECT id, used_at, server_id FROM provisioning_tokens WHERE token_hash = $1`,
		hash,
	).Scan(&tokenID, &usedAt, &existingServerID)
	if err != nil {
		return "", fmt.Errorf("invalid token")
	}
	if usedAt != nil && existingServerID == nil {
		return "", fmt.Errorf("token already used")
	}

	// If a server was previously registered with this token, return that ID
	if existingServerID != nil {
		return *existingServerID, nil
	}

	// First use: generate a server ID and bind the token to it
	newID := generateID()
	_, err = s.db.Exec(ctx,
		`UPDATE provisioning_tokens SET used_at = $1, server_id = $2 WHERE id = $3`,
		time.Now().UTC(), newID, tokenID,
	)
	return newID, err
}

// UpsertServer creates or updates a server record.
func (s *Store) UpsertServer(ctx context.Context, serverID, hostname string) error {
	_, err := s.db.Exec(ctx,
		`INSERT INTO servers (id, hostname, last_seen)
		 VALUES ($1, $2, NOW())
		 ON CONFLICT (id) DO UPDATE SET hostname = $2, last_seen = NOW()`,
		serverID, hostname,
	)
	return err
}

// ─── Schema ──────────────────────────────────────────────────────────────────

func (s *Store) migrate(ctx context.Context) error {
	_, err := s.db.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS servers (
			id         TEXT PRIMARY KEY,
			hostname   TEXT NOT NULL,
			created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			last_seen  TIMESTAMPTZ
		);

		CREATE TABLE IF NOT EXISTS provisioning_tokens (
			id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			token_hash  TEXT NOT NULL UNIQUE,
			created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			used_at     TIMESTAMPTZ,
			server_id   TEXT REFERENCES servers(id)
		);
	`)
	return err
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func hashToken(plaintext string) string {
	h := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(h[:])
}

func generateID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return "srv_" + hex.EncodeToString(b)
}
