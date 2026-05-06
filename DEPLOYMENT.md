# Deployment guide

A general playbook for deploying a Dockerized Go app (like Moco) to a single
Linux VM behind nginx, with DNS managed in Cloudflare and code pulled from a
private GitHub repo via a deploy key.

The examples below use placeholders — replace them with your own values:

| Placeholder         | Meaning                                  | Example                  |
| ------------------- | ---------------------------------------- | ------------------------ |
| `myapp`             | Short slug for the app / SSH alias / dir | `moco`, `notes`, `wiki`  |
| `app.example.com`   | Public hostname of the app               | `reader.your-domain.tld` |
| `user@vm`           | SSH user + host on the VM                | `deploy@1.2.3.4`         |
| `git@github.com:me/myapp.git` | Repo SSH URL                   | your repo                |
| `8666`              | Host port the container binds (loopback) | any free port            |

If you host multiple apps on the same VM, run each one through its own
subdomain, its own nginx vhost, its own port, and its own directory under
`/srv` (or `~`). The pieces below are designed to be copy-pasted per app.

---

## 1. Prerequisites on the VM

One-time setup, regardless of how many apps you run:

```bash
# Docker + compose plugin
sudo apt update
sudo apt install -y docker.io docker-compose-plugin nginx curl ufw

# Allow your shell user to run docker without sudo (logout/login after)
sudo usermod -aG docker "$USER"

# Firewall: SSH + HTTP + HTTPS only. Apps stay on 127.0.0.1.
sudo ufw allow OpenSSH
sudo ufw allow 'Nginx Full'
sudo ufw enable
```

Apps should always bind to `127.0.0.1:<port>` on the host. Public traffic
arrives on 80/443 and is reverse-proxied by nginx — never expose container
ports to the public interface.

---

## 2. Cloudflare: point the subdomain at the VM

In the Cloudflare dashboard, in the zone for your domain:

1. **DNS → Records → Add record**
   - Type: `A`
   - Name: `app` (or whatever subdomain — the full host becomes
     `app.example.com`)
   - IPv4 address: your VM's public IP
   - Proxy status: **Proxied** (orange cloud) — this gives you Cloudflare's
     edge cache, DDoS protection, and a free origin certificate.
2. **SSL/TLS → Overview** for the zone:
   - **Flexible**: Cloudflare ↔ origin is plain HTTP. Easiest, but anyone on
     your VM's network sees clear traffic. Acceptable for a single-tenant VM
     where nginx listens on 80 only.
   - **Full**: Cloudflare ↔ origin is HTTPS using any cert (including
     self-signed). You need to install a cert on the VM (Let's Encrypt or
     Cloudflare's [Origin CA](https://developers.cloudflare.com/ssl/origin-configuration/origin-ca/)).
   - **Full (strict)**: Same as Full, but the cert must validate against a
     real CA. Use Let's Encrypt via certbot for this.

For most personal projects, **Flexible** is fine to start. Switch to **Full
(strict)** once you have certbot wired up.

---

## 3. GitHub deploy key (per-repo, read-only)

Deploy keys are SSH keys scoped to a single repo. Each app gets its own key
pair so that a leaked key only affects one project.

### 3a. Generate the key on the VM

```bash
# One key per repo. Name the file after the app.
ssh-keygen -t ed25519 -C "deploy@myapp" -f ~/.ssh/myapp_deploy -N ""
```

This creates `~/.ssh/myapp_deploy` (private) and `~/.ssh/myapp_deploy.pub`
(public).

### 3b. Add the public key to GitHub

```bash
cat ~/.ssh/myapp_deploy.pub
```

Copy the output. In your GitHub repo: **Settings → Deploy keys → Add deploy
key**. Paste it. Leave **Allow write access** unchecked unless your deploy
script needs to push.

### 3c. Tell SSH which key to use for this repo

Each deploy key needs its own `Host` alias in `~/.ssh/config`, otherwise SSH
tries the same default key for every clone:

```
# ~/.ssh/config
Host github-myapp
  HostName github.com
  User git
  IdentityFile ~/.ssh/myapp_deploy
  IdentitiesOnly yes
```

Now clone using the alias instead of `github.com`:

```bash
# Use the alias in place of github.com
git clone git@github-myapp:me/myapp.git /srv/myapp
```

The `IdentitiesOnly yes` line is important — without it, SSH may offer your
other keys to GitHub first and hit auth-attempt limits.

For a second app, repeat: new key (`secondapp_deploy`), new alias
(`Host github-secondapp`), new clone URL (`git@github-secondapp:me/secondapp.git`).

---

## 4. App directory + .env

```bash
cd /srv/myapp
cp .env.example .env
# edit .env — set ports, secrets, storage credentials, etc.
nano .env
```

Make sure the host port you pick is **free** on the VM. List what's already
bound:

```bash
ss -tlnp | grep 127.0.0.1
```

Pick a port that doesn't show up. Each app on the VM needs a different one.

---

## 5. nginx reverse proxy

One vhost file per app. The pattern:

```nginx
# /etc/nginx/sites-available/myapp
server {
    listen 80;
    listen [::]:80;
    server_name app.example.com;

    # Optional: a friendly cap on uploads. Match what the app accepts.
    client_max_body_size 100M;

    location / {
        proxy_pass         http://127.0.0.1:8666;
        proxy_http_version 1.1;

        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket / SSE / long-poll friendly.
        proxy_set_header Upgrade    $http_upgrade;
        proxy_set_header Connection "upgrade";

        # Streaming downloads (PDFs etc.) — let the response take its time.
        proxy_read_timeout 300s;
        proxy_send_timeout 300s;
    }
}
```

Enable it and reload:

```bash
sudo ln -s /etc/nginx/sites-available/myapp /etc/nginx/sites-enabled/
sudo nginx -t          # syntax check
sudo systemctl reload nginx
```

If you change the host port in `.env`, update `proxy_pass` here and reload
nginx — the values must match.

---

## 6. (Optional) HTTPS at the origin

Skip this if you're using Cloudflare **Flexible** SSL.

For **Full (strict)**, get a Let's Encrypt cert via certbot:

```bash
sudo apt install -y certbot python3-certbot-nginx
sudo certbot --nginx -d app.example.com
```

Certbot rewrites the vhost to listen on 443 and adds an HTTP→HTTPS redirect.
Renewals run automatically via the `certbot.timer` systemd unit — verify with
`systemctl list-timers | grep certbot`.

---

## 7. First deploy

```bash
cd /srv/myapp
docker compose up -d --build
docker compose logs -f --tail=50
```

Health check from the VM itself (replace the path with whatever the app
exposes):

```bash
curl -fsS http://127.0.0.1:8666/api/v1/health
```

Then hit the public URL from your laptop:

```bash
curl -I https://app.example.com
```

---

## 8. Updates: a tiny `deploy.sh`

A repeatable update script lives at the app root. The pattern:

```bash
#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

git pull --ff-only
docker compose up -d --build

# Poll the health endpoint until it answers (or give up).
for i in {1..30}; do
    if curl -fsS http://127.0.0.1:8666/api/v1/health >/dev/null 2>&1; then
        echo "✓ healthy"
        exit 0
    fi
    sleep 1
done

echo "✖ health check timed out"
docker compose logs --tail=50
exit 1
```

Run it after any push:

```bash
ssh user@vm 'cd /srv/myapp && ./deploy.sh'
```

For zero-downtime, you'd add a second container and swap behind nginx — but
for personal projects the `up -d --build` cycle is a few seconds and not
worth the complexity.

---

## 9. Backups

What's worth backing up depends on where the app stores state. Two common
shapes:

**SQLite + local files in a bind-mounted directory.** Snapshot the whole
directory:

```bash
tar -czf "myapp-$(date +%Y%m%d).tar.gz" -C /srv/myapp var/
```

For consistent SQLite backups under load, use the online backup API:

```bash
sqlite3 /srv/myapp/var/app.db ".backup '/tmp/app-$(date +%Y%m%d).db'"
```

**Object storage (S3 / R2).** The bucket is the source of truth — keep
versioning enabled on the bucket and you don't need a separate backup job for
the files. Only the SQLite metadata needs snapshotting.

Cron it nightly and rotate by date. Shipping the tarballs off-box (rsync to
another host, upload to a separate bucket) is what makes a backup actually a
backup.

---

## 10. Adding a second app

Repeat per app, changing only what's app-specific:

1. New deploy key + SSH alias (`github-secondapp`).
2. New repo clone under `/srv/secondapp`.
3. New `.env` with a **different host port**.
4. New nginx vhost in `sites-available/secondapp` with a different
   `server_name` and matching `proxy_pass`.
5. New Cloudflare DNS record for the subdomain.
6. `nginx -t && systemctl reload nginx`, then `docker compose up -d --build`.

Apps stay isolated: their own ports, their own containers, their own data
directories, their own deploy keys. One getting compromised doesn't reach the
others.

---

## Troubleshooting

| Symptom                                 | First place to look                                              |
| --------------------------------------- | ---------------------------------------------------------------- |
| `502 Bad Gateway` from the public URL   | `docker compose ps` — container running? port matches `proxy_pass`? |
| `git pull` asks for a password          | SSH config alias missing or `IdentityFile` path wrong            |
| `ERR_TOO_MANY_REDIRECTS` after enabling SSL | Cloudflare set to Flexible while origin redirects 80→443     |
| Health check passes locally, public fails | nginx vhost not enabled (`ls /etc/nginx/sites-enabled/`)        |
| Container can't reach external services | Check `ufw status` — outbound is open by default but verify     |
| Disk filling up                         | `docker system prune -a` to clear old images and build cache    |
