package database

import (
	"database/sql"
	"fmt"
	"regexp"

	_ "github.com/go-sql-driver/mysql"
)

var validIdent = regexp.MustCompile(`^[a-zA-Z0-9_]{1,64}$`)

// Manager handles MariaDB provisioning operations using the root socket connection.
type Manager struct {
	db *sql.DB
}

func NewManager(socketPath string) (*Manager, error) {
	dsn := fmt.Sprintf("root@unix(%s)/", socketPath)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connect to MariaDB at %s: %w", socketPath, err)
	}
	return &Manager{db: db}, nil
}

func (m *Manager) Close() { _ = m.db.Close() }

// DB returns the underlying sql.DB for use by other packages that share the connection.
func (m *Manager) DB() *sql.DB { return m.db }

// CreateDatabase creates a database, a user, and grants the user full access.
func (m *Manager) CreateDatabase(dbName, user, password string) error {
	if !validIdent.MatchString(dbName) {
		return fmt.Errorf("invalid database name %q", dbName)
	}
	if !validIdent.MatchString(user) {
		return fmt.Errorf("invalid username %q", user)
	}

	stmts := []string{
		fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", dbName),
	}
	for _, stmt := range stmts {
		if _, err := m.db.Exec(stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}

	if _, err := m.db.Exec(
		"CREATE USER IF NOT EXISTS ?@'localhost' IDENTIFIED BY ?", user, password,
	); err != nil {
		return fmt.Errorf("create user %q: %w", user, err)
	}

	if _, err := m.db.Exec(
		fmt.Sprintf("GRANT ALL ON `%s`.* TO ?@'localhost'", dbName), user,
	); err != nil {
		return fmt.Errorf("grant privileges: %w", err)
	}

	if _, err := m.db.Exec("FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("flush privileges: %w", err)
	}

	return nil
}

// DeleteDatabase drops the database and user.
func (m *Manager) DeleteDatabase(dbName, user string) error {
	if !validIdent.MatchString(dbName) {
		return fmt.Errorf("invalid database name %q", dbName)
	}
	if !validIdent.MatchString(user) {
		return fmt.Errorf("invalid username %q", user)
	}

	if _, err := m.db.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS `%s`", dbName)); err != nil {
		return fmt.Errorf("drop database: %w", err)
	}

	if _, err := m.db.Exec("DROP USER IF EXISTS ?@'localhost'", user); err != nil {
		return fmt.Errorf("drop user: %w", err)
	}

	if _, err := m.db.Exec("FLUSH PRIVILEGES"); err != nil {
		return fmt.Errorf("flush privileges: %w", err)
	}

	return nil
}
