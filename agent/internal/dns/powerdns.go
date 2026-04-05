package dns

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client wraps the PowerDNS HTTP API.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{},
	}
}

// ─── PowerDNS API types ───────────────────────────────────────────────────────

type zone struct {
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	Nameservers []string `json:"nameservers,omitempty"`
}

type rrsetPatch struct {
	RRsets []rrset `json:"rrsets"`
}

type rrset struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	TTL        uint32   `json:"ttl,omitempty"`
	ChangeType string   `json:"changetype"`
	Records    []record `json:"records,omitempty"`
	Priority   uint32   `json:"-"` // encoded into content for MX/SRV
}

type record struct {
	Content  string `json:"content"`
	Disabled bool   `json:"disabled"`
}

// ─── Public API ──────────────────────────────────────────────────────────────

// CreateZone creates a new authoritative zone in PowerDNS.
func (c *Client) CreateZone(zoneName string, records []ZoneRecord) error {
	zoneName = fqdn(zoneName)

	body := zone{
		Name: zoneName,
		Kind: "Native",
	}

	if err := c.post("/servers/localhost/zones", body, nil); err != nil {
		return fmt.Errorf("create zone %s: %w", zoneName, err)
	}

	if len(records) > 0 {
		return c.upsertRecords(zoneName, records)
	}
	return nil
}

// DeleteZone removes a zone from PowerDNS.
func (c *Client) DeleteZone(zoneName string) error {
	zoneName = fqdn(zoneName)
	if err := c.delete(fmt.Sprintf("/servers/localhost/zones/%s", zoneName)); err != nil {
		return fmt.Errorf("delete zone %s: %w", zoneName, err)
	}
	return nil
}

// UpsertRecord creates or replaces a DNS record in an existing zone.
func (c *Client) UpsertRecord(zoneName string, r ZoneRecord) error {
	return c.upsertRecords(fqdn(zoneName), []ZoneRecord{r})
}

// DeleteRecord removes a DNS record from a zone.
func (c *Client) DeleteRecord(zoneName, name, rrType string) error {
	zoneName = fqdn(zoneName)
	patch := rrsetPatch{
		RRsets: []rrset{{
			Name:       fqdn(expandName(name, zoneName)),
			Type:       strings.ToUpper(rrType),
			ChangeType: "DELETE",
		}},
	}
	return c.patch(fmt.Sprintf("/servers/localhost/zones/%s", zoneName), patch)
}

// ZoneRecord is the caller-facing record type (maps to proto DNSRecord).
type ZoneRecord struct {
	Name     string // relative to zone, "@" for apex
	Type     string
	Content  string
	TTL      uint32
	Priority uint32
}

// ─── Internals ───────────────────────────────────────────────────────────────

func (c *Client) upsertRecords(zoneFQDN string, records []ZoneRecord) error {
	sets := make([]rrset, 0, len(records))
	for _, r := range records {
		content := r.Content
		if r.Priority > 0 {
			content = fmt.Sprintf("%d %s", r.Priority, r.Content)
		}
		ttl := r.TTL
		if ttl == 0 {
			ttl = 300
		}
		sets = append(sets, rrset{
			Name:       fqdn(expandName(r.Name, zoneFQDN)),
			Type:       strings.ToUpper(r.Type),
			TTL:        ttl,
			ChangeType: "REPLACE",
			Records:    []record{{Content: content}},
		})
	}
	return c.patch(fmt.Sprintf("/servers/localhost/zones/%s", zoneFQDN), rrsetPatch{RRsets: sets})
}

func (c *Client) post(path string, body, out any) error {
	return c.do(http.MethodPost, path, body, out)
}

func (c *Client) patch(path string, body any) error {
	return c.do(http.MethodPatch, path, body, nil)
}

func (c *Client) delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil)
}

func (c *Client) do(method, path string, body, out any) error {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.baseURL+"/api/v1"+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, b)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

// fqdn ensures a domain name ends with a dot (PowerDNS requirement).
func fqdn(name string) string {
	if !strings.HasSuffix(name, ".") {
		return name + "."
	}
	return name
}

// expandName expands "@" to the zone apex and relative names to FQDNs.
func expandName(name, zoneFQDN string) string {
	if name == "@" || name == "" {
		return zoneFQDN
	}
	if strings.HasSuffix(name, ".") {
		return name // already FQDN
	}
	return name + "." + zoneFQDN
}
