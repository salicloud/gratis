package email

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	dkimKeyDir   = "/etc/opendkim/keys"
	dkimSelector = "mail"
)

// SetupDKIM generates a 2048-bit RSA DKIM key pair for the domain,
// writes the OpenDKIM key table / signing table entries, and returns
// the DNS TXT record value the caller should publish.
func SetupDKIM(domain string) (dnsTXTValue string, err error) {
	keyDir := filepath.Join(dkimKeyDir, domain)
	if err := os.MkdirAll(keyDir, 0700); err != nil {
		return "", fmt.Errorf("create dkim key dir: %w", err)
	}

	privKey := filepath.Join(keyDir, dkimSelector+".private")
	pubKey := filepath.Join(keyDir, dkimSelector+".txt")

	// Generate private key
	if out, err := exec.Command(
		"openssl", "genrsa", "-out", privKey, "2048",
	).CombinedOutput(); err != nil {
		return "", fmt.Errorf("generate dkim key: %w: %s", err, out)
	}
	if err := os.Chmod(privKey, 0600); err != nil {
		return "", err
	}

	// Extract public key in DNS TXT format
	if out, err := exec.Command(
		"openssl", "rsa", "-in", privKey, "-pubout", "-out", pubKey,
	).CombinedOutput(); err != nil {
		return "", fmt.Errorf("extract dkim public key: %w: %s", err, out)
	}

	pubPEM, err := os.ReadFile(pubKey)
	if err != nil {
		return "", err
	}
	pubStripped := stripPEMHeaders(string(pubPEM))

	// Write OpenDKIM key table entry
	keyTableEntry := fmt.Sprintf("%s._domainkey.%s %s:%s:%s\n",
		dkimSelector, domain, domain, dkimSelector, privKey)
	if err := appendUnique("/etc/opendkim/KeyTable", keyTableEntry); err != nil {
		return "", fmt.Errorf("update KeyTable: %w", err)
	}

	// Write OpenDKIM signing table entry
	signingEntry := fmt.Sprintf("*@%s %s._domainkey.%s\n", domain, dkimSelector, domain)
	if err := appendUnique("/etc/opendkim/SigningTable", signingEntry); err != nil {
		return "", fmt.Errorf("update SigningTable: %w", err)
	}

	// Reload OpenDKIM if running
	_ = exec.Command("systemctl", "reload", "opendkim").Run()

	// Return the DNS TXT record value
	dnsTXTValue = fmt.Sprintf("v=DKIM1; k=rsa; p=%s", pubStripped)
	return dnsTXTValue, nil
}

// RemoveDKIM removes DKIM config for a domain.
func RemoveDKIM(domain string) error {
	keyDir := filepath.Join(dkimKeyDir, domain)
	if err := os.RemoveAll(keyDir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove dkim keys: %w", err)
	}

	// Remove entries from OpenDKIM tables
	marker := fmt.Sprintf("._domainkey.%s", domain)
	for _, f := range []string{"/etc/opendkim/KeyTable", "/etc/opendkim/SigningTable"} {
		if err := removeLines(f, marker); err != nil {
			return fmt.Errorf("update %s: %w", f, err)
		}
	}

	_ = exec.Command("systemctl", "reload", "opendkim").Run()
	return nil
}

func stripPEMHeaders(pem string) string {
	var lines []string
	for _, line := range strings.Split(pem, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "-----") {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "")
}

func appendUnique(path, line string) error {
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), line) {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func removeLines(path, contains string) error {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var kept []string
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.Contains(line, contains) {
			kept = append(kept, line)
		}
	}
	return os.WriteFile(path, []byte(strings.Join(kept, "\n")), 0644)
}
