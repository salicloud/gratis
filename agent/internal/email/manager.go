// Package email manages Postfix/Dovecot virtual mailbox provisioning.
//
// It assumes a standard virtual hosting setup where Postfix and Dovecot
// read from a MariaDB database (virtual_domains, virtual_users,
// virtual_aliases tables). Run SetupSchema once when adding a server to
// the mail stack to create those tables.
package email

import (
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const mailRoot = "/var/mail/vhosts"

// Manager handles email provisioning via the mail database.
type Manager struct {
	db *sql.DB
}

func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// SetupSchema creates the mail database tables if they don't exist.
// Safe to call repeatedly — uses IF NOT EXISTS throughout.
func (m *Manager) SetupSchema() error {
	stmts := []string{
		`CREATE DATABASE IF NOT EXISTS mail`,
		`CREATE TABLE IF NOT EXISTS mail.virtual_domains (
			id   INT NOT NULL AUTO_INCREMENT,
			name VARCHAR(255) NOT NULL UNIQUE,
			PRIMARY KEY (id)
		)`,
		`CREATE TABLE IF NOT EXISTS mail.virtual_users (
			id        INT NOT NULL AUTO_INCREMENT,
			domain_id INT NOT NULL,
			email     VARCHAR(255) NOT NULL UNIQUE,
			password  VARCHAR(255) NOT NULL,
			quota     BIGINT NOT NULL DEFAULT 0,
			PRIMARY KEY (id),
			FOREIGN KEY (domain_id) REFERENCES mail.virtual_domains(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS mail.virtual_aliases (
			id          INT NOT NULL AUTO_INCREMENT,
			domain_id   INT NOT NULL,
			source      VARCHAR(255) NOT NULL,
			destination VARCHAR(255) NOT NULL,
			PRIMARY KEY (id),
			FOREIGN KEY (domain_id) REFERENCES mail.virtual_domains(id) ON DELETE CASCADE
		)`,
	}
	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return fmt.Errorf("setup schema: %w", err)
		}
	}
	return nil
}

// AddDomain registers a virtual domain with Postfix/Dovecot.
func (m *Manager) AddDomain(domain string) error {
	if _, err := m.db.Exec(`INSERT IGNORE INTO mail.virtual_domains (name) VALUES (?)`, domain); err != nil {
		return fmt.Errorf("add domain %s: %w", domain, err)
	}
	// Create the maildir root for this domain
	return os.MkdirAll(filepath.Join(mailRoot, domain), 0750)
}

// RemoveDomain deletes a virtual domain and cascades to users and aliases.
func (m *Manager) RemoveDomain(domain string, purgeMail bool) error {
	if _, err := m.db.Exec(`DELETE FROM mail.virtual_domains WHERE name = ?`, domain); err != nil {
		return fmt.Errorf("remove domain %s: %w", domain, err)
	}
	if purgeMail {
		domainDir := filepath.Join(mailRoot, domain)
		if err := os.RemoveAll(domainDir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("purge mail for %s: %w", domain, err)
		}
	}
	return nil
}

// CreateAccount creates a virtual mailbox.
func (m *Manager) CreateAccount(address, password string, quotaBytes uint64) error {
	local, domain, err := splitAddress(address)
	if err != nil {
		return err
	}

	domainID, err := m.domainID(domain)
	if err != nil {
		return fmt.Errorf("domain %s not found — add it first: %w", domain, err)
	}

	hash, err := hashPassword(password)
	if err != nil {
		return err
	}

	if _, err := m.db.Exec(
		`INSERT INTO mail.virtual_users (domain_id, email, password, quota) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE password = VALUES(password), quota = VALUES(quota)`,
		domainID, address, hash, quotaBytes,
	); err != nil {
		return fmt.Errorf("create account %s: %w", address, err)
	}

	// Create maildir structure
	maildir := filepath.Join(mailRoot, domain, local)
	for _, sub := range []string{"", "cur", "new", "tmp"} {
		if err := os.MkdirAll(filepath.Join(maildir, sub), 0700); err != nil {
			return fmt.Errorf("create maildir: %w", err)
		}
	}

	return nil
}

// DeleteAccount removes a virtual mailbox.
func (m *Manager) DeleteAccount(address string, purgeMail bool) error {
	local, domain, err := splitAddress(address)
	if err != nil {
		return err
	}

	if _, err := m.db.Exec(`DELETE FROM mail.virtual_users WHERE email = ?`, address); err != nil {
		return fmt.Errorf("delete account %s: %w", address, err)
	}

	if purgeMail {
		maildir := filepath.Join(mailRoot, domain, local)
		if err := os.RemoveAll(maildir); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("purge maildir: %w", err)
		}
	}

	return nil
}

// CreateAlias adds a virtual alias (forwarder).
func (m *Manager) CreateAlias(source, destination string) error {
	_, domain, err := splitAddress(source)
	if err != nil {
		return err
	}

	domainID, err := m.domainID(domain)
	if err != nil {
		return fmt.Errorf("domain %s not found: %w", domain, err)
	}

	if _, err := m.db.Exec(
		`INSERT IGNORE INTO mail.virtual_aliases (domain_id, source, destination) VALUES (?, ?, ?)`,
		domainID, source, destination,
	); err != nil {
		return fmt.Errorf("create alias %s -> %s: %w", source, destination, err)
	}

	return nil
}

// DeleteAlias removes a virtual alias.
func (m *Manager) DeleteAlias(source string) error {
	if _, err := m.db.Exec(`DELETE FROM mail.virtual_aliases WHERE source = ?`, source); err != nil {
		return fmt.Errorf("delete alias %s: %w", source, err)
	}
	return nil
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

func (m *Manager) domainID(domain string) (int64, error) {
	var id int64
	err := m.db.QueryRow(`SELECT id FROM mail.virtual_domains WHERE name = ?`, domain).Scan(&id)
	if err == sql.ErrNoRows {
		return 0, fmt.Errorf("not found")
	}
	return id, err
}

func splitAddress(address string) (local, domain string, err error) {
	parts := strings.SplitN(address, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid email address: %q", address)
	}
	return parts[0], parts[1], nil
}

// hashPassword generates a Dovecot-compatible SHA512-CRYPT hash.
func hashPassword(password string) (string, error) {
	out, err := exec.Command("doveadm", "pw", "-s", "SHA512-CRYPT", "-p", password).Output()
	if err != nil {
		return "", fmt.Errorf("hash password (is dovecot installed?): %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
