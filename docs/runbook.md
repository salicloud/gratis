# Deployment Runbook

This covers deploying the Gratis control plane (API + frontend + PostgreSQL) to a single server and connecting managed servers via the agent.

---

## Control plane server requirements

| Resource | Minimum | Recommended |
|---|---|---|
| CPU | 1 vCPU | 2 vCPU |
| RAM | 1 GB | 2 GB |
| Disk | 20 GB | 40 GB |
| OS | Ubuntu 22.04+ / Debian 12+ | Ubuntu 24.04 |

The control plane doesn't do any heavy lifting — it's just Postgres, the API, and the Next.js frontend. A $6/mo DigitalOcean droplet works fine to start.

---

## 1. DNS

Point your panel domain at the server's IP before anything else — certbot needs it to resolve.

```
panel.sali.cloud   A   <server-ip>
```

---

## 2. Server setup

```bash
# Install Docker
curl -fsSL https://get.docker.com | sh
systemctl enable --now docker

# Install Docker Compose plugin
apt-get install -y docker-compose-plugin

# Install nginx and certbot
apt-get install -y nginx certbot python3-certbot-nginx

# Create certbot webroot
mkdir -p /var/www/certbot
```

---

## 3. Firewall

Open the ports nginx and gRPC need. Everything else stays closed.

```bash
ufw allow 22/tcp    # SSH — don't lock yourself out
ufw allow 80/tcp    # HTTP (certbot + redirect)
ufw allow 443/tcp   # HTTPS (panel)
ufw allow 9090/tcp  # gRPC (agents connect here from managed servers)
ufw enable
```

---

## 4. Deploy the control plane

```bash
# Clone the repo
git clone git@github.com:salicloud/gratis.git /opt/gratis
cd /opt/gratis/deploy

# Create your .env file
cp .env.example .env
$EDITOR .env  # set POSTGRES_PASSWORD and GRATIS_ADMIN_KEY to strong random values
```

Generate strong values with:
```bash
openssl rand -hex 32  # run twice — once for each
```

```bash
# Build and start
docker compose up -d --build

# Check everything came up
docker compose ps
docker compose logs -f
```

Expected output: postgres healthy, api and web running.

---

## 5. SSL certificate

Get the cert before enabling the HTTPS nginx config:

```bash
certbot certonly --webroot -w /var/www/certbot -d panel.sali.cloud \
  --email you@sali.cloud --agree-tos --non-interactive
```

---

## 6. Configure nginx

```bash
# Copy the config and substitute your domain
sed 's/panel.sali.cloud/YOUR_ACTUAL_DOMAIN/g' \
  /opt/gratis/deploy/nginx/gratis.conf \
  > /etc/nginx/sites-available/gratis.conf

ln -s /etc/nginx/sites-available/gratis.conf /etc/nginx/sites-enabled/gratis.conf

# Remove the default site if present
rm -f /etc/nginx/sites-enabled/default

nginx -t && systemctl reload nginx
```

Your panel should now be live at `https://panel.sali.cloud`.

---

## 7. Create a provisioning token

Every managed server needs a token to authenticate with the API. Generate one:

```bash
curl -s -X POST https://panel.sali.cloud/api/v1/admin/tokens \
  -H "X-Admin-Key: YOUR_GRATIS_ADMIN_KEY" | jq .
```

Response:
```json
{ "token": "a3f9c2d1e8b7..." }
```

**Save this token — it's shown once.**

---

## 8. Install the agent on a managed server

On each server you want to manage:

```bash
curl -fsSL https://raw.githubusercontent.com/salicloud/gratis/main/deploy/agent/install.sh | \
  bash -s -- --api panel.sali.cloud:9090 --token YOUR_TOKEN
```

Or manually:

```bash
# Download the binary (replace with actual release URL when available)
curl -fsSL https://github.com/salicloud/gratis/releases/latest/download/gratis-agent-linux-amd64 \
  -o /usr/local/bin/gratis-agent
chmod +x /usr/local/bin/gratis-agent

# Write config
mkdir -p /etc/gratis
cat > /etc/gratis/agent.env <<EOF
GRATIS_API_ADDR=panel.sali.cloud:9090
GRATIS_TOKEN=YOUR_TOKEN
EOF
chmod 600 /etc/gratis/agent.env

# Install and start the service
curl -fsSL https://raw.githubusercontent.com/salicloud/gratis/main/deploy/agent/gratis-agent.service \
  -o /etc/systemd/system/gratis-agent.service

systemctl daemon-reload
systemctl enable --now gratis-agent
journalctl -u gratis-agent -f
```

The server should appear in the Gratis dashboard within a few seconds.

---

## 9. Verify

```bash
# Check the API sees the agent
curl -s https://panel.sali.cloud/api/v1/servers | jq .

# Check agent logs on the managed server
journalctl -u gratis-agent --since "5 minutes ago"
```

---

## Ongoing operations

### Updating Gratis

```bash
cd /opt/gratis
git pull
docker compose up -d --build
```

### Renewing SSL (automatic via certbot timer, but manual if needed)

```bash
certbot renew --quiet
systemctl reload nginx
```

### Viewing logs

```bash
# All services
docker compose -f /opt/gratis/deploy/docker-compose.yml logs -f

# Just the API
docker compose -f /opt/gratis/deploy/docker-compose.yml logs -f api
```

### Backing up PostgreSQL

```bash
docker compose -f /opt/gratis/deploy/docker-compose.yml exec postgres \
  pg_dump -U gratis gratis | gzip > gratis-$(date +%Y%m%d).sql.gz
```

---

## gRPC TLS (recommended for production)

Currently the agent connects to gRPC over plain TCP. Before exposing port 9090 publicly at scale, add TLS to the gRPC server. Two options:

1. **nginx stream proxy with SSL termination** — add an `nginx stream` block that terminates TLS on 9090 and forwards to the Docker container. Requires nginx compiled with `--with-stream`.

2. **Native gRPC TLS in the API** — pass a cert/key to `grpc.NewServer` with `grpc.Creds(credentials.NewTLS(...))` and update the agent to use `credentials.NewClientTLSFromFile(...)`.

Option 2 is cleaner. This is the next security hardening item.
