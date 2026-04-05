package rpc

import (
	"fmt"
	"log/slog"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
	"github.com/salicloud/gratis/agent/internal/database"
	"github.com/salicloud/gratis/agent/internal/dns"
	"github.com/salicloud/gratis/agent/internal/system"
	"github.com/salicloud/gratis/agent/internal/webserver"
)

// Dispatcher routes commands to the appropriate handler modules.
// Modules that require external services (DB, DNS) are nil when not configured.
type Dispatcher struct {
	db  *database.Manager
	dns *dns.Client
}

func NewDispatcher(db *database.Manager, dnsClient *dns.Client) *Dispatcher {
	return &Dispatcher{db: db, dns: dnsClient}
}

func (d *Dispatcher) Dispatch(cmd *agentv1.Command) *agentv1.CommandResult {
	var err error

	switch p := cmd.Payload.(type) {
	// Web server
	case *agentv1.Command_CreateVhost:
		err = handleCreateVhost(p.CreateVhost)
	case *agentv1.Command_DeleteVhost:
		err = handleDeleteVhost(p.DeleteVhost)

	// System accounts
	case *agentv1.Command_CreateAccount:
		err = handleCreateAccount(p.CreateAccount)
	case *agentv1.Command_DeleteAccount:
		err = handleDeleteAccount(p.DeleteAccount)

	// Databases
	case *agentv1.Command_CreateDatabase:
		err = d.handleCreateDatabase(p.CreateDatabase)
	case *agentv1.Command_DeleteDatabase:
		err = d.handleDeleteDatabase(p.DeleteDatabase)

	// DNS
	case *agentv1.Command_CreateDnsZone:
		err = d.handleCreateDNSZone(p.CreateDnsZone)
	case *agentv1.Command_DeleteDnsZone:
		err = d.handleDeleteDNSZone(p.DeleteDnsZone)
	case *agentv1.Command_UpsertDnsRecord:
		err = d.handleUpsertDNSRecord(p.UpsertDnsRecord)
	case *agentv1.Command_DeleteDnsRecord:
		err = d.handleDeleteDNSRecord(p.DeleteDnsRecord)

	// Service management
	case *agentv1.Command_RestartService:
		err = handleRestartService(p.RestartService)

	default:
		err = fmt.Errorf("unhandled command type %T", cmd.Payload)
		slog.Warn("unhandled command", "type", fmt.Sprintf("%T", cmd.Payload))
	}

	if err != nil {
		return &agentv1.CommandResult{CommandId: cmd.CommandId, Success: false, Error: err.Error()}
	}
	return &agentv1.CommandResult{CommandId: cmd.CommandId, Success: true}
}

// ─── Web server ──────────────────────────────────────────────────────────────

func handleCreateVhost(cmd *agentv1.CreateVhostCmd) error {
	return webserver.CreateVhost(webserver.VhostConfig{
		Domain:     cmd.Domain,
		Aliases:    cmd.Aliases,
		Docroot:    cmd.Docroot,
		PHPVersion: cmd.PhpVersion,
	})
}

func handleDeleteVhost(cmd *agentv1.DeleteVhostCmd) error {
	return webserver.DeleteVhost(cmd.Domain)
}

// ─── System accounts ─────────────────────────────────────────────────────────

func handleCreateAccount(cmd *agentv1.CreateAccountCmd) error {
	return system.CreateAccount(cmd.Username, cmd.Uid, cmd.Homedir, cmd.DiskQuotaBytes)
}

func handleDeleteAccount(cmd *agentv1.DeleteAccountCmd) error {
	return system.DeleteAccount(cmd.Username, cmd.PurgeFiles)
}

// ─── Databases ───────────────────────────────────────────────────────────────

func (d *Dispatcher) handleCreateDatabase(cmd *agentv1.CreateDatabaseCmd) error {
	if d.db == nil {
		return fmt.Errorf("database manager not configured on this server")
	}
	return d.db.CreateDatabase(cmd.DbName, cmd.DbUser, cmd.Password)
}

func (d *Dispatcher) handleDeleteDatabase(cmd *agentv1.DeleteDatabaseCmd) error {
	if d.db == nil {
		return fmt.Errorf("database manager not configured on this server")
	}
	return d.db.DeleteDatabase(cmd.DbName, cmd.DbUser)
}

// ─── DNS ─────────────────────────────────────────────────────────────────────

func (d *Dispatcher) handleCreateDNSZone(cmd *agentv1.CreateDNSZoneCmd) error {
	if d.dns == nil {
		return fmt.Errorf("DNS client not configured on this server")
	}
	records := make([]dns.ZoneRecord, len(cmd.Records))
	for i, r := range cmd.Records {
		records[i] = dns.ZoneRecord{
			Name: r.Name, Type: r.Type, Content: r.Content,
			TTL: r.Ttl, Priority: r.Priority,
		}
	}
	return d.dns.CreateZone(cmd.Zone, records)
}

func (d *Dispatcher) handleDeleteDNSZone(cmd *agentv1.DeleteDNSZoneCmd) error {
	if d.dns == nil {
		return fmt.Errorf("DNS client not configured on this server")
	}
	return d.dns.DeleteZone(cmd.Zone)
}

func (d *Dispatcher) handleUpsertDNSRecord(cmd *agentv1.UpsertDNSRecordCmd) error {
	if d.dns == nil {
		return fmt.Errorf("DNS client not configured on this server")
	}
	return d.dns.UpsertRecord(cmd.Zone, dns.ZoneRecord{
		Name: cmd.Record.Name, Type: cmd.Record.Type, Content: cmd.Record.Content,
		TTL: cmd.Record.Ttl, Priority: cmd.Record.Priority,
	})
}

func (d *Dispatcher) handleDeleteDNSRecord(cmd *agentv1.DeleteDNSRecordCmd) error {
	if d.dns == nil {
		return fmt.Errorf("DNS client not configured on this server")
	}
	return d.dns.DeleteRecord(cmd.Zone, cmd.Name, cmd.Type)
}

// ─── Service management ──────────────────────────────────────────────────────

func handleRestartService(cmd *agentv1.RestartServiceCmd) error {
	// Allowlist to prevent arbitrary service restarts
	allowed := map[string]bool{
		"nginx": true, "apache2": true, "php8.1-fpm": true, "php8.2-fpm": true,
		"php8.3-fpm": true, "postfix": true, "dovecot": true, "pdns": true,
		"mariadb": true, "mysqld": true,
	}
	if !allowed[cmd.Service] {
		return fmt.Errorf("service %q is not in the allowed restart list", cmd.Service)
	}
	return webserver.RestartService(cmd.Service)
}
