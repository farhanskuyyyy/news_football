# Deployment Runbook

Flow: push ke `main` → GitHub Actions (test → docker build → deploy SSH) → VPS pull + rebuild → nginx expose ke internet.

```
internet → nginx :80 → app container :8082 → postgres / redis (internal)
```

---

## Step 1 — Laptop: commit & push

```bash
cd ~/Projects/belajar/apigo-docker
git add .
git commit -m "add CD pipeline and deployment config"

# repo belum ada di GitHub:
gh repo create apigo-docker --private --source=. --push
# atau manual: bikin repo kosong di github.com lalu
# git remote add origin git@github.com:farhanarfianto/apigo-docker.git
# git push -u origin main
```

> **Note:** `NEWS_API_KEY` ada di `.env.example` dan ikut ke-commit. Repo private: OK.
> Mau public: kosongin value-nya dan rotate key di newsapi.org dulu.

Job `deploy` bakal **gagal** di push pertama karena secrets belum ada — normal, lanjut saja ke step berikutnya, nanti re-run.

---

## Step 2 — Laptop: bikin SSH keypair khusus deploy

```bash
ssh-keygen -t ed25519 -f ~/.ssh/apigo-deploy -N "" -C "github-actions-deploy"

# copy public key ke VPS
ssh-copy-id -i ~/.ssh/apigo-deploy.pub USER@IP_VPS

# tes harus bisa masuk tanpa password
ssh -i ~/.ssh/apigo-deploy USER@IP_VPS "echo ok"
```

---

## Step 3 — GitHub: isi secrets

Repo → **Settings → Secrets and variables → Actions → New repository secret**:

| Secret            | Isi                                                                 |
|-------------------|---------------------------------------------------------------------|
| `SERVER_HOST`     | IP VPS                                                              |
| `SERVER_USER`     | user SSH di VPS                                                     |
| `SSH_PRIVATE_KEY` | isi file `~/.ssh/apigo-deploy` (bukan `.pub`), full satu file termasuk baris `-----BEGIN/END-----` |

```bash
# tampilkan private key buat di-copy:
cat ~/.ssh/apigo-deploy
```

---

## Step 4 — VPS: prasyarat (sekali saja)

Asumsi Docker sudah terinstall.

```bash
# docker tanpa sudo
sudo usermod -aG docker $USER
# logout + login lagi biar group aktif

docker ps        # harus jalan tanpa sudo
```

---

## Step 5 — VPS: clone & konfigurasi project

Path **harus** `~/apigo-docker` (dipakai job deploy).

```bash
git clone https://github.com/farhanarfianto/apigo-docker.git ~/apigo-docker
cd ~/apigo-docker

cp .env.example .env
nano .env        # pastikan NEWS_API_KEY keisi
```

Bikin override khusus server — app cuma listen di localhost (nginx yang expose),
postgres tidak dibuka ke internet. File ini TIDAK di-commit, cukup ada di server:

```bash
cat > docker-compose.override.yml <<'EOF'
services:
  app:
    ports: !override
      - "127.0.0.1:8082:8080"
  postgres:
    ports: !override []
EOF
```

> `!override` wajib — tanpa itu compose me-MERGE list `ports` base + override
> (dua mapping rebutan port 8082 → app gagal start), bukan me-replace.
> Butuh docker compose v2.24+ (`docker compose version`).

First run manual:

```bash
docker compose up -d --build
docker compose ps                          # 3 container up, postgres/redis healthy
curl http://localhost:8082/health          # {"status":"ok"}
curl http://localhost:8082/news | head -c 200
```

---

## Step 6 — VPS: firewall

```bash
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw enable
sudo ufw status
```

Port 8082/5434 TIDAK usah di-allow — app cuma diakses via nginx, DB internal.

---

## Step 7 — VPS: nginx

```bash
sudo apt update && sudo apt install -y nginx

sudo tee /etc/nginx/sites-available/apigo > /dev/null <<'EOF'
server {
    listen 80 default_server;
    server_name _;

    location / {
        proxy_pass http://127.0.0.1:8082;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
EOF

sudo rm -f /etc/nginx/sites-enabled/default
sudo ln -sf /etc/nginx/sites-available/apigo /etc/nginx/sites-enabled/apigo
sudo nginx -t && sudo systemctl reload nginx
```

Tes dari laptop:

```bash
curl http://IP_VPS/health
curl http://IP_VPS/news | head -c 200
```

---

## Step 8 — Tes full CI/CD

```bash
# di laptop
echo "" >> README.md
git commit -am "test cicd pipeline"
git push
```

GitHub → tab **Actions** → pipeline `test` → `docker` → `deploy` semua hijau.
Habis itu cek `http://IP_VPS/news` — perubahan ke-deploy otomatis.

Kalau job `deploy` merah di push pertama (sebelum secrets ada): buka run yang gagal → **Re-run failed jobs**.

---

## Selesai. Cheat sheet operasional

| Aksi | Command (di VPS, `cd ~/apigo-docker`) |
|---|---|
| Lihat status | `docker compose ps` |
| Log app | `docker compose logs -f app` |
| Restart manual | `docker compose up -d --build` |
| Masuk DB | `docker compose exec postgres psql -U postgres -d newsdb` |
| Cek redis | `docker compose exec redis redis-cli` |
| Stop semua (data aman) | `docker compose down` |
| Reset total + hapus data | `docker compose down -v` |

## Nanti kalau mau HTTPS

HTTPS butuh domain (Let's Encrypt nggak issue cert buat IP polos). Gratis pakai DuckDNS.

### 1. Bikin subdomain di DuckDNS

1. Buka [duckdns.org](https://www.duckdns.org), login (GitHub/Google)
2. Kolom **sub domain**: ketik nama unik milikmu, misal `farhanapi` → klik **add domain**
3. Di baris domain baru, kolom **current ip**: isi IP VPS → klik **update ip**

> ⚠️ Di semua command bawah, `SUBDOMAINLU` = nama yang barusan kamu daftarkan.
> Jangan dijalankan mentah-mentah pakai placeholder — subdomain milik orang lain
> bakal bikin validasi certbot gagal (`Timeout during connect` ke IP yang bukan VPS-mu).

### 2. Verifikasi DNS nunjuk ke VPS

```bash
dig +short SUBDOMAINLU.duckdns.org
# output HARUS = IP VPS. Kalau kosong/beda, benerin dulu di duckdns, tunggu 1-2 menit.
```

### 3. Pasang cert

```bash
sudo apt install -y certbot python3-certbot-nginx

# ganti server_name di nginx (yang sekarang masih `_`)
sudo sed -i 's/server_name .*;/server_name SUBDOMAINLU.duckdns.org;/' /etc/nginx/sites-available/apigo
sudo nginx -t && sudo systemctl reload nginx

# port 80 harus sudah ke-allow di ufw (dipakai validasi Let's Encrypt)
sudo ufw allow 443/tcp
sudo certbot --nginx -d SUBDOMAINLU.duckdns.org
```

Certbot otomatis nambah `listen 443 ssl` + redirect 80→443 + auto-renewal.

### 4. Tes

```bash
curl https://SUBDOMAINLU.duckdns.org/health
```

Gagal `Timeout during connect` = DNS belum nunjuk ke VPS (balik ke step 2) atau port 80 ketutup (`sudo ufw status`).

## Troubleshooting cepat

| Gejala | Cek |
|---|---|
| Deploy job gagal `Permission denied (publickey)` | Secret `SSH_PRIVATE_KEY` harus full file termasuk BEGIN/END, public key ada di `~/.ssh/authorized_keys` VPS |
| Deploy jalan tapi app nggak update | `docker compose logs app`; pastikan `git pull` di VPS nggak conflict (`git status` di `~/apigo-docker`) |
| `502 Bad Gateway` dari nginx | App container mati → `docker compose ps`, `docker compose logs app` |
| `/news` return 502 `failed to fetch from newsapi` | `NEWS_API_KEY` kosong/invalid di `.env` VPS |
| Port bentrok pas `up` | `sudo lsof -nP -iTCP:8082 -sTCP:LISTEN` cari siapa yang pegang |
