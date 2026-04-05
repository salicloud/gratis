# Deployment Runbook

Two deployment paths are documented here:
- **DOKS (Kubernetes)** — recommended for production, HA, rolling deploys
- **Single server (Docker Compose)** — simpler, good for staging or getting started fast

Agents connect to the control plane the same way regardless of which path you use.

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

## DOKS deployment (Kubernetes)

Images are built automatically by GitHub Actions on every push to `main` and pushed to `ghcr.io/salicloud/gratis-api` and `ghcr.io/salicloud/gratis-web`.

### Prerequisites

```bash
# Install tools if not already present
brew install helm kubectl doctl   # or apt/choco equivalents

# Authenticate
doctl auth init
```

### 1. Create the cluster

```bash
doctl kubernetes cluster create gratis \
  --region nyc3 \
  --node-pool "name=default;size=s-1vcpu-2gb;count=2;auto-scale=true;min-nodes=2;max-nodes=4" \
  --wait

doctl kubernetes cluster kubeconfig save gratis
kubectl get nodes  # verify
```

### 2. Create DO Managed Postgres

```bash
doctl databases create gratis-db \
  --engine pg \
  --version 16 \
  --size db-s-1vcpu-1gb \
  --region nyc3 \
  --num-nodes 1

# Get the connection string
doctl databases connection gratis-db --format URI
```

Save the URI — you'll need it as `secrets.dbURL` below.

### 3. Install cert-manager

```bash
helm repo add jetstack https://charts.jetstack.io
helm repo update

helm install cert-manager jetstack/cert-manager \
  --namespace cert-manager --create-namespace \
  --set crds.enabled=true

# Create Let's Encrypt ClusterIssuer
kubectl apply -f - <<EOF
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: letsencrypt-prod
spec:
  acme:
    server: https://acme-v02.api.letsencrypt.org/directory
    email: you@sali.cloud
    privateKeySecretRef:
      name: letsencrypt-prod
    solvers:
      - http01:
          ingress:
            ingressClassName: nginx
EOF
```

### 4. Install nginx-ingress with gRPC TCP support

The gRPC port (9090) is exposed through the ingress load balancer via nginx's TCP services feature — no second load balancer needed.

```bash
helm repo add ingress-nginx https://kubernetes.github.io/ingress-nginx
helm repo update

helm install ingress-nginx ingress-nginx/ingress-nginx \
  --namespace ingress-nginx --create-namespace \
  --set tcp.9090="default/gratis-gratis-api:9090"
```

Get the load balancer IP after it provisions (takes ~60 seconds):

```bash
kubectl get svc -n ingress-nginx ingress-nginx-controller \
  --output jsonpath='{.status.loadBalancer.ingress[0].ip}'
```

Point your DNS at this IP:
```
panel.sali.cloud   A   <load-balancer-ip>
```

### 5. Create your production values file

```bash
cat > deploy/helm/values.prod.yaml <<EOF
ingress:
  host: panel.sali.cloud

secrets:
  dbURL: "postgresql://..."   # from step 2
  adminKey: "$(openssl rand -hex 32)"
EOF
# This file is gitignored — keep it safe
```

### 6. Deploy Gratis

```bash
helm install gratis deploy/helm/gratis \
  --namespace default \
  -f deploy/helm/values.prod.yaml \
  --wait
```

Check rollout:
```bash
kubectl get pods
kubectl logs -l app.kubernetes.io/name=gratis-api
```

### Updating

Every push to `main` builds new images tagged with the git SHA. To deploy a new version:

```bash
# Get the SHA of the commit you want to deploy
SHA=$(git rev-parse --short HEAD)

helm upgrade gratis deploy/helm/gratis \
  -f deploy/helm/values.prod.yaml \
  --set image.api.tag=$SHA \
  --set image.web.tag=$SHA \
  --wait
```

Rolling update — zero downtime.

### Viewing logs (DOKS)

```bash
kubectl logs -l app.kubernetes.io/name=gratis-api -f
kubectl logs -l app.kubernetes.io/name=gratis-web -f
```

---

## gRPC TLS (recommended for production)

Currently the agent connects to gRPC over plain TCP. Before exposing port 9090 publicly at scale, add TLS to the gRPC server. Two options:

1. **nginx stream proxy with SSL termination** — add an `nginx stream` block that terminates TLS on 9090 and forwards to the Docker container. Requires nginx compiled with `--with-stream`.

2. **Native gRPC TLS in the API** — pass a cert/key to `grpc.NewServer` with `grpc.Creds(credentials.NewTLS(...))` and update the agent to use `credentials.NewClientTLSFromFile(...)`.

Option 2 is cleaner. This is the next security hardening item.
