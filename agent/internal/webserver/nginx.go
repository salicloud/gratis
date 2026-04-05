package webserver

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

const (
	sitesAvailable = "/etc/nginx/sites-available"
	sitesEnabled   = "/etc/nginx/sites-enabled"
)

var vhostTemplate = template.Must(template.New("vhost").Parse(`# Managed by Gratis — do not edit manually
server {
    listen 80;
    listen [::]:80;
    server_name {{ .Domain }}{{ range .Aliases }} {{ . }}{{ end }};

    root {{ .Docroot }};
    index index.php index.html index.htm;

    access_log /var/log/nginx/{{ .Domain }}.access.log;
    error_log  /var/log/nginx/{{ .Domain }}.error.log;

    location / {
        try_files $uri $uri/ /index.php?$query_string;
    }
{{ if .PHPVersion }}
    location ~ \.php$ {
        include snippets/fastcgi-php.conf;
        fastcgi_pass unix:/run/php/php{{ .PHPVersion }}-fpm.sock;
        fastcgi_param SCRIPT_FILENAME $realpath_root$fastcgi_script_name;
        include fastcgi_params;
    }
{{ end }}
    location ~ /\.(?!well-known) {
        deny all;
    }
}
`))

type VhostConfig struct {
	Domain     string
	Aliases    []string
	Docroot    string
	PHPVersion string // empty = no PHP
}

// CreateVhost writes the nginx config, enables it, and reloads nginx.
func CreateVhost(cfg VhostConfig) error {
	if cfg.Docroot == "" {
		cfg.Docroot = fmt.Sprintf("/home/%s/public_html/%s", cfg.Domain, cfg.Domain)
	}

	if err := os.MkdirAll(cfg.Docroot, 0755); err != nil {
		return fmt.Errorf("create docroot %s: %w", cfg.Docroot, err)
	}

	var buf bytes.Buffer
	if err := vhostTemplate.Execute(&buf, cfg); err != nil {
		return fmt.Errorf("render vhost template: %w", err)
	}

	confPath := filepath.Join(sitesAvailable, cfg.Domain+".conf")
	if err := os.WriteFile(confPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("write config %s: %w", confPath, err)
	}

	linkPath := filepath.Join(sitesEnabled, cfg.Domain+".conf")
	// Remove stale symlink if it exists
	_ = os.Remove(linkPath)
	if err := os.Symlink(confPath, linkPath); err != nil {
		return fmt.Errorf("enable site %s: %w", cfg.Domain, err)
	}

	if err := testConfig(); err != nil {
		// Roll back on bad config
		_ = os.Remove(linkPath)
		_ = os.Remove(confPath)
		return fmt.Errorf("nginx config test failed: %w", err)
	}

	return reloadNginx()
}

// DeleteVhost removes the nginx config and reloads nginx.
func DeleteVhost(domain string) error {
	confPath := filepath.Join(sitesAvailable, domain+".conf")
	linkPath := filepath.Join(sitesEnabled, domain+".conf")

	_ = os.Remove(linkPath)
	if err := os.Remove(confPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove config %s: %w", confPath, err)
	}

	return reloadNginx()
}

func testConfig() error {
	out, err := exec.Command("nginx", "-t").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return nil
}

func reloadNginx() error {
	out, err := exec.Command("systemctl", "reload", "nginx").CombinedOutput()
	if err != nil {
		return fmt.Errorf("reload nginx: %w: %s", err, out)
	}
	return nil
}

// RestartService restarts a named system service via systemctl.
func RestartService(name string) error {
	out, err := exec.Command("systemctl", "restart", name).CombinedOutput()
	if err != nil {
		return fmt.Errorf("restart %s: %w: %s", name, err, out)
	}
	return nil
}
