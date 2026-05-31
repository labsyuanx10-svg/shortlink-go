# 🔗 Shortlink Pro

> Self-hosted URL shortener + microsite/bio page manager.  
> Built with Go + SQLite + Docker. Inspired by Bit.ly, s.id, Linktree, Shlink.

---

## ✨ Features

### 🔗 Short Link
- Custom slug (random or manual)
- ✔️ Live availability check
- Password-protected links
- Link expiration / scheduling
- UTM Builder
- Click analytics (daily chart, referrers)
- Auto-fetch title & preview from URL
- QR code (download PNG)
- Export CSV

### 📋 Microsite / Bio Page
- Customizable bio page (`/u/username`)
- Themes (10 presets) + custom colors
- Button styles (rounded, pill, square, soft)
- Social links (10 platforms)
- Avatar + bio text
- Add/manage links directly inside bio

### 👥 Multi-User
- Role-based: `admin` 👑 vs `user` 👤
- Admin: manage all users, links, microsites
- User: only see own data
- Change password in Users page

### 📊 Dashboard
- Stats cards (total links, clicks, today, microsites)
- 14-day click chart (Chart.js)
- Top links leaderboard
- Search + pagination + status filter

---

## 🚀 Quick Start

```bash
# Clone & run
git clone <your-repo-url>
cd url-shortener/diy-shortlink
docker compose up -d --build
```

Then open **http://localhost:8090**  
Login: `admin` / `admin123`

### Environment Variables (docker-compose)

| Variable | Default | Description |
|---|---|---|
| `PORT` | `8090` | Web server port |
| `DOMAIN` | `localhost:8090` | Public domain |
| `DB_PATH` | `/data/links.db` | SQLite file path |
| `SESSION_KEY` | auto | Session encryption key |

---

## 🏗️ Stack

```
diy-shortlink/
├── main.go              # Go server (~1300 lines)
├── go.mod / go.sum      # Go modules
├── Dockerfile           # Multi-stage build (15MB image)
├── docker-compose.yml   # One-command deploy
└── templates/
    ├── admin.html       # Admin dashboard (SPA)
    ├── login.html       # Login page
    └── microsite.html   # Public bio page
```

- **Go 1.22** — single binary, built-in HTTP server
- **SQLite** — zero config, 1 file database
- **Chi** — lightweight HTTP router
- **Tailwind CSS** — utility-first UI (CDN)
- **Chart.js** — dashboard analytics
- **go-qrcode** — server-side QR generation

---

## 📸 Screenshots

```
Login       →  http://localhost:8090/login
Dashboard   →  http://localhost:8090/          (after login)
Links       →  Click "Links" in sidebar
Microsites  →  Click "Microsites" in sidebar
Users       →  Click "Users" in sidebar
Public Bio  →  http://localhost:8090/u/username
QR Code     →  http://localhost:8090/{code}/qr
```

---

## 🔌 API Endpoints

### Authentication
| Method | Path | Description |
|---|---|---|
| `GET` | `/login` | Login page |
| `POST` | `/login` | Login form |
| `GET` | `/logout` | Logout |

### Links (auth required)
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/links` | List links (paginated, searchable) |
| `POST` | `/api/links` | Create link |
| `PUT` | `/api/links/{code}` | Update link (including slug!) |
| `DELETE` | `/api/links/{code}` | Delete link |
| `GET` | `/api/links/{code}/stats` | Click analytics |
| `POST` | `/api/links/{code}/detach` | Remove from microsite |
| `POST` | `/api/links/{code}/verify-password` | Verify password |
| `GET` | `/api/links/export` | Export CSV/JSON |
| `GET` | `/api/check-slug/{code}` | Check slug availability |

### Microsites (auth required)
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/microsites` | List microsites |
| `POST` | `/api/microsites` | Create/update |
| `GET` | `/api/microsites/{username}` | Get with links |
| `DELETE` | `/api/microsites/{username}` | Delete |
| `GET` | `/api/microsites/{username}/stats` | Click stats |

### Users (auth required, admin for management)
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/users` | List users (admin only) |
| `POST` | `/api/users` | Create user (admin only) |
| `DELETE` | `/api/users/{username}` | Delete user (admin only) |
| `PUT` | `/api/users/{username}/password` | Set user password (admin) |
| `PUT` | `/api/settings/password` | Change own password |

### Public
| Method | Path | Description |
|---|---|---|
| `GET` | `/{code}` | Redirect to URL (or password page) |
| `GET` | `/{code}/qr` | QR code PNG |
| `GET` | `/u/{username}` | Microsite bio page |

### Utilities (auth required)
| Method | Path | Description |
|---|---|---|
| `GET` | `/api/dashboard` | Dashboard stats |
| `GET` | `/api/themes` | Theme presets (10) |
| `POST` | `/api/fetch-title` | Auto-fetch og:title from URL |

---

## 🎨 10 Theme Presets

| Name | Background | Text | Accent |
|---|---|---|---|
| Dark | `#0f172a` | `#e2e8f0` | `#3b82f6` |
| Light | `#ffffff` | `#1e293b` | `#3b82f6` |
| Midnight | `#09090b` | `#fafafa` | `#a78bfa` |
| Forest | `#052e16` | `#ecfdf5` | `#22c55e` |
| Ocean | `#0c4a6e` | `#e0f2fe` | `#06b6d4` |
| Sunset | `#431407` | `#fff7ed` | `#f97316` |
| Lavender | `#1e1b4b` | `#eef2ff` | `#8b5cf6` |
| Rose | `#4c0519` | `#fff1f2` | `#e11d48` |
| Coffee | `#292524` | `#fafaf9` | `#d97706` |
| Cyberpunk | `#020617` | `#f8fafc` | `#f43f5e` |

---

## 📦 Other Services Included

This repo also includes docker-compose setups for:

| Service | Port | Description |
|---|---|---|
| `diy-shortlink` | `:8090` | Our custom app |
| `shlink` | `:8080` | Alternative shortener |
| `linkstack` | `:8082` | Alternative bio page |

---

## 📄 License

MIT
