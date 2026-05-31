package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/mattn/go-sqlite3"
	"github.com/skip2/go-qrcode"
)

var db *sql.DB

// ==================== CONFIG ====================

type Config struct {
	DBPath        string
	Domain        string
	AdminUser     string
	AdminPassword string
	Port          string
	SessionKey    string
}

func loadConfig() Config {
	return Config{
		DBPath:        envOr("DB_PATH", "/data/links.db"),
		Domain:        envOr("DOMAIN", "localhost:8090"),
		AdminUser:     envOr("ADMIN_USER", "admin"),
		AdminPassword: envOr("ADMIN_PASSWORD", "admin123"),
		Port:          envOr("PORT", "8090"),
		SessionKey:    envOr("SESSION_KEY", "change-me-session-key-123"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ==================== MODELS ====================

type User struct {
	ID           int    `json:"id"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	Role         string `json:"role"`
	CreatedAt    int64  `json:"created_at"`
}

type Link struct {
	Code         string `json:"code"`
	URL          string `json:"url"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	PreviewImage string `json:"preview_image"`
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
	HasPassword  bool   `json:"has_password"`
	ExpiresAt    int64  `json:"expires_at"`
	CreatedAt    int64  `json:"created_at"`
	Clicks       int    `json:"clicks"`
	ShortURL     string `json:"short_url"`
	IsActive     bool   `json:"is_active"`
	Owner        string `json:"owner"`
}

type Microsite struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Bio         string `json:"bio"`
	AvatarURL   string `json:"avatar_url"`
	BgColor     string `json:"bg_color"`
	TextColor   string `json:"text_color"`
	AccentColor string `json:"accent_color"`
	FontFamily  string `json:"font_family"`
	BtnStyle    string `json:"btn_style"`
	SocialJSON  string `json:"social_json"`
	CreatedAt   int64  `json:"created_at"`
	Links       []Link `json:"links"`
	LinkCount   int    `json:"link_count"`
	TotalClicks int    `json:"total_clicks"`
	Owner       string `json:"owner"`
}

type ClickLog struct {
	Day       string `json:"day"`
	Count     int    `json:"count"`
	Referrer  string `json:"referrer,omitempty"`
	UserAgent string `json:"user_agent,omitempty"`
}

type DashboardStats struct {
	TotalLinks  int        `json:"total_links"`
	TotalClicks int        `json:"total_clicks"`
	TodayClicks int        `json:"today_clicks"`
	TopLinks    []Link     `json:"top_links"`
	DailyClicks []ClickLog `json:"daily_clicks"`
	Microsites  int        `json:"microsites"`
	Users       int        `json:"users"`
	Role        string     `json:"role"`
	Username    string     `json:"username"`
}

type ThemePreset struct {
	Name        string `json:"name"`
	BgColor     string `json:"bg_color"`
	TextColor   string `json:"text_color"`
	AccentColor string `json:"accent_color"`
}

var themePresets = []ThemePreset{
	{"Dark", "#0f172a", "#e2e8f0", "#3b82f6"},
	{"Light", "#ffffff", "#1e293b", "#3b82f6"},
	{"Midnight", "#09090b", "#fafafa", "#a78bfa"},
	{"Forest", "#052e16", "#ecfdf5", "#22c55e"},
	{"Ocean", "#0c4a6e", "#e0f2fe", "#06b6d4"},
	{"Sunset", "#431407", "#fff7ed", "#f97316"},
	{"Lavender", "#1e1b4b", "#eef2ff", "#8b5cf6"},
	{"Rose", "#4c0519", "#fff1f2", "#e11d48"},
	{"Coffee", "#292524", "#fafaf9", "#d97706"},
	{"Cyberpunk", "#020617", "#f8fafc", "#f43f5e"},
}

// ==================== MAIN ====================

func main() {
	cfg := loadConfig()
	var err error

	db, err = sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		log.Fatal("db open:", err)
	}
	defer db.Close()

	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA foreign_keys=ON")

	// --- Tables ---
	db.Exec(`CREATE TABLE IF NOT EXISTS links (
		code TEXT PRIMARY KEY, url TEXT NOT NULL, title TEXT NOT NULL DEFAULT '',
		username TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL,
		clicks INTEGER NOT NULL DEFAULT 0, is_active INTEGER NOT NULL DEFAULT 1,
		password_hash TEXT NOT NULL DEFAULT '', expires_at INTEGER NOT NULL DEFAULT 0,
		description TEXT NOT NULL DEFAULT '', preview_image TEXT NOT NULL DEFAULT '',
		owner TEXT NOT NULL DEFAULT ''
	)`)
	db.Exec("ALTER TABLE links ADD COLUMN owner TEXT NOT NULL DEFAULT ''")

	db.Exec(`CREATE TABLE IF NOT EXISTS microsites (
		username TEXT PRIMARY KEY, display_name TEXT NOT NULL DEFAULT '',
		bio TEXT NOT NULL DEFAULT '', avatar_url TEXT NOT NULL DEFAULT '',
		bg_color TEXT NOT NULL DEFAULT '#0f172a', text_color TEXT NOT NULL DEFAULT '#e2e8f0',
		accent_color TEXT NOT NULL DEFAULT '#3b82f6', font_family TEXT NOT NULL DEFAULT 'system-ui',
		btn_style TEXT NOT NULL DEFAULT 'rounded', social_json TEXT NOT NULL DEFAULT '[]',
		created_at INTEGER NOT NULL, owner TEXT NOT NULL DEFAULT ''
	)`)
	db.Exec("ALTER TABLE microsites ADD COLUMN owner TEXT NOT NULL DEFAULT ''")
	db.Exec("ALTER TABLE microsites ADD COLUMN font_family TEXT NOT NULL DEFAULT 'system-ui'")
	db.Exec("ALTER TABLE microsites ADD COLUMN btn_style TEXT NOT NULL DEFAULT 'rounded'")
	db.Exec("ALTER TABLE microsites ADD COLUMN social_json TEXT NOT NULL DEFAULT '[]'")

	db.Exec(`CREATE TABLE IF NOT EXISTS click_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT, code TEXT NOT NULL,
		clicked_at INTEGER NOT NULL, referrer TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '', ip TEXT NOT NULL DEFAULT ''
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS users (
		id INTEGER PRIMARY KEY AUTOINCREMENT, username TEXT UNIQUE NOT NULL,
		password_hash TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'user', created_at INTEGER NOT NULL
	)`)

	db.Exec(`CREATE TABLE IF NOT EXISTS sessions (
		token TEXT PRIMARY KEY,
		username TEXT NOT NULL,
		created_at INTEGER NOT NULL
	)`)

	// Seed default admin user
	var adminExists int
	db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'admin'").Scan(&adminExists)
	if adminExists == 0 {
		// Check old settings table
		var oldPass string
		db.QueryRow("SELECT value FROM settings WHERE key = 'admin_password'").Scan(&oldPass)
		if oldPass == "" {
			oldPass = cfg.AdminPassword
		}
		h := sha256.Sum256([]byte(oldPass))
		db.Exec("INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, 'admin', ?)",
			cfg.AdminUser, fmt.Sprintf("%x", h), time.Now().Unix())
	}

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.Logger, middleware.Recoverer, middleware.RealIP)

	fileServer := http.FileServer(http.Dir("./static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	r.Get("/login", func(w http.ResponseWriter, r *http.Request) { renderLogin(w) })
	r.Post("/login", loginHandler)
	r.Get("/logout", logoutHandler)

	// Public
	r.Post("/api/links/{code}/verify-password", verifyPassword)
	r.Get("/u/{username}", renderMicrosite)
	r.Get("/{code}/qr", qrRedirect)
	r.Get("/{code}", handleRedirect)

	// Protected
	r.Group(func(r chi.Router) {
		r.Use(authMiddleware)
		r.Get("/", dashboardPage)
		r.Get("/api/dashboard", dashboardAPI)
		r.Get("/api/links", listLinks)
		r.Post("/api/links", createLink)
		r.Put("/api/links/{code}", updateLink)
		r.Delete("/api/links/{code}", deleteLink)
		r.Get("/api/links/{code}/stats", linkStats)
		r.Get("/api/links/export", exportLinks)
		r.Post("/api/links/{code}/detach", detachLink)
		r.Get("/api/microsites", listMicrosites)
		r.Post("/api/microsites", upsertMicrosite)
		r.Get("/api/microsites/{username}", getMicrosite)
		r.Delete("/api/microsites/{username}", deleteMicrosite)
		r.Get("/api/microsites/{username}/stats", micrositeStats)
		r.Get("/api/themes", themesHandler)
		r.Post("/api/fetch-title", fetchTitleHandler)
		r.Get("/api/users", listUsers)
		r.Post("/api/users", createUser)
		r.Delete("/api/users/{username}", deleteUser)
		r.Put("/api/users/{username}/password", changeUserPassword)
		r.Get("/api/settings", getSettingsHandler)
		r.Put("/api/settings/password", changeOwnPassword)
		r.Get("/api/check-slug/{code}", checkSlugHandler)
	})

	port := cfg.Port
	log.Printf("🚀 Shortlink Pro running on :%s", port)
	log.Printf("📍 http://%s:%s", cfg.Domain, port)
	http.ListenAndServe(":"+port, r)
}

// ==================== HELPERS ====================

func generateCode() string {
	b := make([]byte, 4)
	rand.Read(b)
	return strings.TrimRight(base64.URLEncoding.EncodeToString(b), "=")[:6]
}

func jsonResp(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}

func jsonErr(w http.ResponseWriter, msg string, status int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func hashPassword(pw string) string {
	h := sha256.Sum256([]byte(pw))
	return fmt.Sprintf("%x", h)
}

func todayRange() (int64, int64) {
	now := time.Now()
	t := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	return t.Unix(), t.Add(24 * time.Hour).Unix()
}

func weekAgo() int64 {
	return time.Now().AddDate(0, 0, -7).Unix()
}

// ==================== AUTH ====================

type contextKey string

const (
	ctxUser     contextKey = "user"
	ctxUsername contextKey = "username"
	ctxRole     contextKey = "role"
)

func getUserFromDB(username string) *User {
	var u User
	err := db.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?", username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil {
		return nil
	}
	return &u
}

func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// API Key
		key := r.Header.Get("X-API-Key")
		if key != "" {
			h := hashPassword(key)
			var u User
			err := db.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE password_hash = ?", h).
				Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
			if err == nil {
				ctx := context.WithValue(r.Context(), ctxUser, &u)
				ctx = context.WithValue(ctx, ctxUsername, u.Username)
				ctx = context.WithValue(ctx, ctxRole, u.Role)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// Session
		cookie, err := r.Cookie("session")
		if err == nil && cookie.Value != "" {
			var username string
			err := db.QueryRow("SELECT username FROM sessions WHERE token = ?", cookie.Value).Scan(&username)
			if err == nil {
				u := getUserFromDB(username)
				if u != nil {
					ctx := context.WithValue(r.Context(), ctxUser, u)
					ctx = context.WithValue(ctx, ctxUsername, u.Username)
					ctx = context.WithValue(ctx, ctxRole, u.Role)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		if strings.HasPrefix(r.URL.Path, "/api/") {
			jsonErr(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		http.Redirect(w, r, "/login", http.StatusFound)
	})
}

func getCurrentUser(r *http.Request) *User {
	if u, ok := r.Context().Value(ctxUser).(*User); ok {
		return u
	}
	return nil
}

func getCurrentUsername(r *http.Request) string {
	if u, ok := r.Context().Value(ctxUsername).(string); ok {
		return u
	}
	return ""
}

func isAdmin(r *http.Request) bool {
	role, _ := r.Context().Value(ctxRole).(string)
	return role == "admin"
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()
	user := r.FormValue("username")
	pw := r.FormValue("password")

	var u User
	err := db.QueryRow("SELECT id, username, password_hash, role, created_at FROM users WHERE username = ?", user).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt)
	if err != nil || hashPassword(pw) != u.PasswordHash {
		renderLoginError(w, "Invalid credentials")
		return
	}

	sessionToken := generateCode() + generateCode()
	db.Exec("INSERT INTO sessions (token, username, created_at) VALUES (?, ?, ?)",
		sessionToken, u.Username, time.Now().Unix())

	http.SetCookie(w, &http.Cookie{
		Name: "session", Value: sessionToken, Path: "/",
		HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 86400 * 7,
	})
	http.Redirect(w, r, "/", http.StatusFound)
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: "session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	http.Redirect(w, r, "/login", http.StatusFound)
}

// ==================== LINK HANDLERS ====================

func createLink(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL          string `json:"url"`
		Title        string `json:"title"`
		Code         string `json:"code"`
		Username     string `json:"username"`
		Password     string `json:"password"`
		ExpiresAt    int64  `json:"expires_at"`
		Description  string `json:"description"`
		PreviewImage string `json:"preview_image"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.URL == "" {
		jsonErr(w, "url required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
		body.URL = "https://" + body.URL
	}

	owner := getCurrentUsername(r)

	if body.Username != "" {
		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM microsites WHERE username = ?)", body.Username).Scan(&exists)
		if !exists {
			jsonErr(w, "microsite not found", http.StatusBadRequest)
			return
		}
	}

	code := body.Code
	if code != "" {
		var exists bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)", code).Scan(&exists)
		if exists {
			jsonErr(w, "code already taken", http.StatusConflict)
			return
		}
	} else {
		code = generateCode()
		for {
			var exists bool
			db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)", code).Scan(&exists)
			if !exists {
				break
			}
			code = generateCode()
		}
	}

	passwordHash := ""
	if body.Password != "" {
		passwordHash = hashPassword(body.Password)
	}

	db.Exec("INSERT INTO links (code, url, title, description, preview_image, username, password_hash, expires_at, created_at, owner) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		code, body.URL, body.Title, body.Description, body.PreviewImage, body.Username, passwordHash, body.ExpiresAt, time.Now().Unix(), owner)

	if body.Title == "" {
		go func(c, u string) {
			t, d, i := fetchPageMeta(u)
			if t != "" {
				db.Exec("UPDATE links SET title = ?, description = ?, preview_image = ? WHERE code = ?", t, d, i, c)
			}
		}(code, body.URL)
	}

	jsonResp(w, Link{
		Code: code, URL: body.URL, Title: body.Title,
		Username: body.Username, HasPassword: passwordHash != "",
		ExpiresAt: body.ExpiresAt, CreatedAt: time.Now().Unix(),
		ShortURL: fmt.Sprintf("%s/%s", r.Host, code), IsActive: true, Owner: owner,
	})
}

func updateLink(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	owner := getCurrentUsername(r)

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ? AND (owner = ? OR ?))", code, owner, isAdmin(r)).Scan(&exists)
	if !exists {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}

	var body struct {
		URL      string `json:"url"`
		Title    string `json:"title"`
		Code     string `json:"code"`
		IsActive *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}

	newCode := code
	if body.Code != "" && body.Code != code {
		// Check new code not taken
		var taken bool
		db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)", body.Code).Scan(&taken)
		if taken {
			jsonErr(w, "code already taken", http.StatusConflict)
			return
		}
		newCode = body.Code
		// Get old data and migrate
		var url, title, username, desc, previewImg, pwHash, ownerVal string
		var createdAt, expiresAt, clicks int64
		var isActive int
		db.QueryRow("SELECT url, title, username, created_at, clicks, is_active, password_hash, expires_at, description, preview_image, owner FROM links WHERE code = ?", code).
			Scan(&url, &title, &username, &createdAt, &clicks, &isActive, &pwHash, &expiresAt, &desc, &previewImg, &ownerVal)
		db.Exec("INSERT INTO links (code, url, title, username, created_at, clicks, is_active, password_hash, expires_at, description, preview_image, owner) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			newCode, url, title, username, createdAt, clicks, isActive, pwHash, expiresAt, desc, previewImg, ownerVal)
		db.Exec("UPDATE click_logs SET code = ? WHERE code = ?", newCode, code)
		db.Exec("DELETE FROM links WHERE code = ?", code)
	}

	if body.URL != "" {
		if !strings.HasPrefix(body.URL, "http://") && !strings.HasPrefix(body.URL, "https://") {
			body.URL = "https://" + body.URL
		}
		db.Exec("UPDATE links SET url = ? WHERE code = ?", body.URL, newCode)
	}
	if body.Title != "" {
		db.Exec("UPDATE links SET title = ? WHERE code = ?", body.Title, newCode)
	}
	if body.IsActive != nil {
		v := 0
		if *body.IsActive {
			v = 1
		}
		db.Exec("UPDATE links SET is_active = ? WHERE code = ?", v, newCode)
	}

	var l Link
	var pwHash string
	db.QueryRow("SELECT code, url, title, username, created_at, clicks, is_active, password_hash FROM links WHERE code = ?", newCode).
		Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive, &pwHash)
	l.HasPassword = pwHash != ""
	l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)
	jsonResp(w, l)
}

func detachLink(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	owner := getCurrentUsername(r)

	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ? AND (owner = ? OR ?))", code, owner, isAdmin(r)).Scan(&exists)
	if !exists {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}

	db.Exec("UPDATE links SET username = '' WHERE code = ?", code)
	jsonResp(w, map[string]bool{"ok": true})
}

func listLinks(w http.ResponseWriter, r *http.Request) {
	username := r.URL.Query().Get("username")
	search := r.URL.Query().Get("search")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	limit := 20
	offset := (page - 1) * limit

	owner := getCurrentUsername(r)
	where := "1=1"
	args := []interface{}{}

	if !isAdmin(r) {
		where += " AND owner = ?"
		args = append(args, owner)
	}
	if username != "" {
		where += " AND username = ?"
		args = append(args, username)
	}
	if search != "" {
		where += " AND (title LIKE ? OR url LIKE ? OR code LIKE ?)"
		s := "%" + search + "%"
		args = append(args, s, s, s)
	}

	var total int
	db.QueryRow("SELECT COUNT(*) FROM links WHERE "+where, args...).Scan(&total)

	query := "SELECT code, url, title, username, created_at, clicks, is_active, password_hash FROM links WHERE " + where + " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)
	rows, err := db.Query(query, args...)
	if err != nil {
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var l Link
		var pw string
		rows.Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive, &pw)
		l.HasPassword = pw != ""
		l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)
		links = append(links, l)
	}
	if links == nil {
		links = []Link{}
	}

	jsonResp(w, map[string]interface{}{
		"links": links, "total": total, "page": page, "pages": (total + limit - 1) / limit,
	})
}

func deleteLink(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	owner := getCurrentUsername(r)

	if isAdmin(r) {
		db.Exec("DELETE FROM click_logs WHERE code = ?", code)
		db.Exec("DELETE FROM links WHERE code = ?", code)
	} else {
		db.Exec("DELETE FROM click_logs WHERE code = ?", code)
		_, err := db.Exec("DELETE FROM links WHERE code = ? AND owner = ?", code, owner)
		if err != nil {
			jsonErr(w, "not found", http.StatusNotFound)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func linkStats(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")

	var l Link
	var pwHash string
	err := db.QueryRow("SELECT code, url, title, username, created_at, clicks, is_active, password_hash FROM links WHERE code = ?", code).
		Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive, &pwHash)
	if err != nil {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}
	l.HasPassword = pwHash != ""
	l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)

	monthAgo := time.Now().AddDate(0, -1, 0).Unix()

	rows, _ := db.Query(`SELECT date(clicked_at, 'unixepoch') as day, COUNT(*) as count FROM click_logs WHERE code = ? AND clicked_at > ? GROUP BY day ORDER BY day`, code, monthAgo)
	var daily []ClickLog
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var c ClickLog
			rows.Scan(&c.Day, &c.Count)
			daily = append(daily, c)
		}
	}
	if daily == nil {
		daily = []ClickLog{}
	}

	rrows, _ := db.Query(`SELECT referrer, COUNT(*) as count FROM click_logs WHERE code = ? AND referrer != '' AND clicked_at > ? GROUP BY referrer ORDER BY count DESC LIMIT 10`, code, monthAgo)
	var topRefs []ClickLog
	if rrows != nil {
		defer rrows.Close()
		for rrows.Next() {
			var c ClickLog
			rrows.Scan(&c.Referrer, &c.Count)
			topRefs = append(topRefs, c)
		}
	}
	if topRefs == nil {
		topRefs = []ClickLog{}
	}

	rclicks, _ := db.Query(`SELECT referrer, user_agent, clicked_at FROM click_logs WHERE code = ? ORDER BY clicked_at DESC LIMIT 20`, code)
	var recent []ClickLog
	if rclicks != nil {
		defer rclicks.Close()
		for rclicks.Next() {
			var c ClickLog
			var t int64
			rclicks.Scan(&c.Referrer, &c.UserAgent, &t)
			c.Day = time.Unix(t, 0).Format("2006-01-02 15:04")
			recent = append(recent, c)
		}
	}

	jsonResp(w, map[string]interface{}{
		"link": l, "daily_clicks": daily, "top_referrers": topRefs, "recent_clicks": recent,
	})
}

func exportLinks(w http.ResponseWriter, r *http.Request) {
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	owner := getCurrentUsername(r)
	where := "1=1"
	if !isAdmin(r) {
		where = "owner = ?"
	}

	rows, _ := db.Query("SELECT code, url, title, username, created_at, clicks, is_active FROM links WHERE "+where+" ORDER BY created_at DESC", owner)
	var links []Link
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l Link
			rows.Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive)
			links = append(links, l)
		}
	}

	if format == "csv" {
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=links.csv")
		writer := csv.NewWriter(w)
		writer.Write([]string{"Code", "URL", "Title", "Microsite", "Created", "Clicks", "Active"})
		for _, l := range links {
			a := "Yes"
			if !l.IsActive {
				a = "No"
			}
			writer.Write([]string{l.Code, l.URL, l.Title, l.Username, time.Unix(l.CreatedAt, 0).Format("2006-01-02"), strconv.Itoa(l.Clicks), a})
		}
		writer.Flush()
		return
	}
	jsonResp(w, links)
}

// ==================== MICROSITE HANDLERS ====================

func upsertMicrosite(w http.ResponseWriter, r *http.Request) {
	var body Microsite
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Username == "" {
		jsonErr(w, "username required", http.StatusBadRequest)
		return
	}

	owner := getCurrentUsername(r)

	// Check ownership if updating existing
	var existingOwner string
	db.QueryRow("SELECT owner FROM microsites WHERE username = ?", body.Username).Scan(&existingOwner)
	if existingOwner != "" && existingOwner != owner && !isAdmin(r) {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	db.Exec(`INSERT INTO microsites (username, display_name, bio, avatar_url, bg_color, text_color, accent_color, font_family, btn_style, social_json, created_at, owner)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET display_name=excluded.display_name, bio=excluded.bio,
		avatar_url=excluded.avatar_url, bg_color=excluded.bg_color, text_color=excluded.text_color,
		accent_color=excluded.accent_color, font_family=excluded.font_family, btn_style=excluded.btn_style,
		social_json=excluded.social_json`,
		body.Username, body.DisplayName, body.Bio, body.AvatarURL,
		body.BgColor, body.TextColor, body.AccentColor, body.FontFamily, body.BtnStyle, body.SocialJSON,
		time.Now().Unix(), owner)
	jsonResp(w, body)
}

func listMicrosites(w http.ResponseWriter, r *http.Request) {
	owner := getCurrentUsername(r)
	var rows *sql.Rows
	if isAdmin(r) {
		rows, _ = db.Query(`SELECT m.username, m.display_name, m.bio, m.avatar_url, m.bg_color, m.text_color, m.accent_color,
			m.font_family, m.btn_style, m.social_json, m.created_at, m.owner,
			COUNT(l.code) as lc, COALESCE(SUM(l.clicks), 0) as tc
			FROM microsites m LEFT JOIN links l ON l.username = m.username GROUP BY m.username ORDER BY m.created_at DESC`)
	} else {
		rows, _ = db.Query(`SELECT m.username, m.display_name, m.bio, m.avatar_url, m.bg_color, m.text_color, m.accent_color,
			m.font_family, m.btn_style, m.social_json, m.created_at, m.owner,
			COUNT(l.code) as lc, COALESCE(SUM(l.clicks), 0) as tc
			FROM microsites m LEFT JOIN links l ON l.username = m.username WHERE m.owner = ? GROUP BY m.username ORDER BY m.created_at DESC`, owner)
	}
	defer rows.Close()

	var sites []Microsite
	if rows != nil {
		for rows.Next() {
			var s Microsite
			rows.Scan(&s.Username, &s.DisplayName, &s.Bio, &s.AvatarURL, &s.BgColor, &s.TextColor, &s.AccentColor,
				&s.FontFamily, &s.BtnStyle, &s.SocialJSON, &s.CreatedAt, &s.Owner, &s.LinkCount, &s.TotalClicks)
			sites = append(sites, s)
		}
	}
	if sites == nil {
		sites = []Microsite{}
	}
	jsonResp(w, sites)
}

func getMicrosite(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var s Microsite
	err := db.QueryRow(`SELECT username, display_name, bio, avatar_url, bg_color, text_color, accent_color,
		font_family, btn_style, social_json, created_at, owner FROM microsites WHERE username = ?`, username).
		Scan(&s.Username, &s.DisplayName, &s.Bio, &s.AvatarURL, &s.BgColor, &s.TextColor, &s.AccentColor,
			&s.FontFamily, &s.BtnStyle, &s.SocialJSON, &s.CreatedAt, &s.Owner)
	if err != nil {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}
	rows, _ := db.Query("SELECT code, url, title, username, created_at, clicks, is_active FROM links WHERE username = ? AND is_active = 1 ORDER BY created_at ASC", username)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l Link
			rows.Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive)
			l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)
			s.Links = append(s.Links, l)
		}
	}
	if s.Links == nil {
		s.Links = []Link{}
	}
	jsonResp(w, s)
}

func deleteMicrosite(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	owner := getCurrentUsername(r)
	if isAdmin(r) {
		db.Exec("UPDATE links SET username = '' WHERE username = ?", username)
		db.Exec("DELETE FROM microsites WHERE username = ?", username)
	} else {
		db.Exec("UPDATE links SET username = '' WHERE username = ? AND owner = ?", username, owner)
		_, err := db.Exec("DELETE FROM microsites WHERE username = ? AND owner = ?", username, owner)
		if err != nil {
			jsonErr(w, "not found", http.StatusNotFound)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func micrositeStats(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM microsites WHERE username = ?)", username).Scan(&exists)
	if !exists {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}

	monthAgo := time.Now().AddDate(0, -1, 0).Unix()
	rows, _ := db.Query(`SELECT date(cl.clicked_at, 'unixepoch') as day, COUNT(*) as count FROM click_logs cl
		JOIN links l ON l.code = cl.code WHERE l.username = ? AND cl.clicked_at > ? GROUP BY day ORDER BY day`, username, monthAgo)
	var daily []ClickLog
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var c ClickLog
			rows.Scan(&c.Day, &c.Count)
			daily = append(daily, c)
		}
	}
	if daily == nil {
		daily = []ClickLog{}
	}

	var totalClicks, linkCount int
	db.QueryRow("SELECT COALESCE(SUM(clicks), 0) FROM links WHERE username = ?", username).Scan(&totalClicks)
	db.QueryRow("SELECT COUNT(*) FROM links WHERE username = ?", username).Scan(&linkCount)

	jsonResp(w, map[string]interface{}{"username": username, "total_clicks": totalClicks, "link_count": linkCount, "daily_clicks": daily})
}

// ==================== USER HANDLERS ====================

func listUsers(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}
	rows, _ := db.Query("SELECT id, username, role, created_at FROM users ORDER BY created_at ASC")
	defer rows.Close()
	var users []User
	if rows != nil {
		for rows.Next() {
			var u User
			rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt)
			users = append(users, u)
		}
	}
	if users == nil {
		users = []User{}
	}
	jsonResp(w, users)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Username == "" || body.Password == "" {
		jsonErr(w, "username & password required", http.StatusBadRequest)
		return
	}
	if body.Role == "" {
		body.Role = "user"
	}
	if body.Role != "user" && body.Role != "admin" {
		jsonErr(w, "role must be 'user' or 'admin'", http.StatusBadRequest)
		return
	}

	_, err := db.Exec("INSERT INTO users (username, password_hash, role, created_at) VALUES (?, ?, ?, ?)",
		body.Username, hashPassword(body.Password), body.Role, time.Now().Unix())
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			jsonErr(w, "username already exists", http.StatusConflict)
			return
		}
		jsonErr(w, "db error", http.StatusInternalServerError)
		return
	}
	jsonResp(w, map[string]interface{}{"username": body.Username, "role": body.Role})
}

func deleteUser(w http.ResponseWriter, r *http.Request) {
	if !isAdmin(r) {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}
	username := chi.URLParam(r, "username")
	// Don't allow deleting self
	current := getCurrentUsername(r)
	if username == current {
		jsonErr(w, "cannot delete yourself", http.StatusBadRequest)
		return
	}
	db.Exec("DELETE FROM users WHERE username = ?", username)
	w.WriteHeader(http.StatusNoContent)
}

func changeUserPassword(w http.ResponseWriter, r *http.Request) {
	target := chi.URLParam(r, "username")
	current := getCurrentUsername(r)

	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}
	if body.Password == "" {
		jsonErr(w, "password required", http.StatusBadRequest)
		return
	}

	// Admin can change anyone's password, users can only change their own
	if !isAdmin(r) && target != current {
		jsonErr(w, "forbidden", http.StatusForbidden)
		return
	}

	_, err := db.Exec("UPDATE users SET password_hash = ? WHERE username = ?", hashPassword(body.Password), target)
	if err != nil {
		jsonErr(w, "not found", http.StatusNotFound)
		return
	}
	jsonResp(w, map[string]bool{"ok": true})
}

func changeOwnPassword(w http.ResponseWriter, r *http.Request) {
	current := getCurrentUser(r)
	if current == nil {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var body struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid json", http.StatusBadRequest)
		return
	}
	if hashPassword(body.OldPassword) != current.PasswordHash {
		jsonErr(w, "current password is wrong", http.StatusForbidden)
		return
	}
	db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", hashPassword(body.NewPassword), current.ID)
	jsonResp(w, map[string]bool{"ok": true})
}

func getSettingsHandler(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	if u == nil {
		jsonErr(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	jsonResp(w, map[string]string{"username": u.Username, "role": u.Role})
}

// ==================== DASHBOARD ====================

func dashboardAPI(w http.ResponseWriter, r *http.Request) {
	var stats DashboardStats
	owner := getCurrentUsername(r)
	isAdm := isAdmin(r)

	where := "1=1"
	if !isAdm {
		where = "owner = '" + owner + "'"
	}

	db.QueryRow("SELECT COUNT(*) FROM links WHERE "+where).Scan(&stats.TotalLinks)
	db.QueryRow("SELECT COUNT(*) FROM microsites WHERE "+where).Scan(&stats.Microsites)
	db.QueryRow("SELECT COALESCE(SUM(clicks), 0) FROM links WHERE "+where).Scan(&stats.TotalClicks)
	stats.Users = 1
	if isAdm {
		db.QueryRow("SELECT COUNT(*) FROM users").Scan(&stats.Users)
	}

	start, end := todayRange()
	db.QueryRow("SELECT COUNT(*) FROM click_logs WHERE clicked_at >= ? AND clicked_at < ?", start, end).Scan(&stats.TodayClicks)

	rows, _ := db.Query(`SELECT date(clicked_at, 'unixepoch') as day, COUNT(*) as count FROM click_logs WHERE clicked_at > ? GROUP BY day ORDER BY day`, weekAgo())
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var c ClickLog
			rows.Scan(&c.Day, &c.Count)
			stats.DailyClicks = append(stats.DailyClicks, c)
		}
	}
	stats.DailyClicks = fillMissingDays(stats.DailyClicks, 14)

	trows, _ := db.Query("SELECT code, url, title, username, created_at, clicks, is_active FROM links WHERE "+where+" ORDER BY clicks DESC LIMIT 10")
	if trows != nil {
		defer trows.Close()
		for trows.Next() {
			var l Link
			trows.Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive)
			l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)
			stats.TopLinks = append(stats.TopLinks, l)
		}
	}
	if stats.TopLinks == nil {
		stats.TopLinks = []Link{}
	}
	stats.Role = "user"
	if isAdm {
		stats.Role = "admin"
	}
	stats.Username = owner

	jsonResp(w, stats)
}

func fillMissingDays(entries []ClickLog, days int) []ClickLog {
	now := time.Now()
	existing := make(map[string]int)
	for _, e := range entries {
		existing[e.Day] = e.Count
	}
	var result []ClickLog
	for i := days - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i).Format("2006-01-02")
		result = append(result, ClickLog{Day: day, Count: existing[day]})
	}
	return result
}

func themesHandler(w http.ResponseWriter, r *http.Request) {
	jsonResp(w, themePresets)
}

// ==================== FETCH / VERIFY ====================

func fetchAndUpdateTitle(code, url string) {
	title, desc, img := fetchPageMeta(url)
	if title != "" {
		db.Exec("UPDATE links SET title = ?, description = ?, preview_image = ? WHERE code = ?", title, desc, img, code)
	}
}

func fetchPageMeta(rawURL string) (title, desc, img string) {
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		rawURL = "https://" + rawURL
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(rawURL)
	if err != nil {
		return
	}
	defer resp.Body.Close()
	body := make([]byte, 65536)
	n, _ := resp.Body.Read(body)
	content := string(body[:n])

	if m := extractMeta(content, "og:title"); m != "" {
		title = m
	} else if m := extractMeta(content, "twitter:title"); m != "" {
		title = m
	} else {
		idx := strings.Index(content, "<title>")
		if idx >= 0 {
			end := strings.Index(content[idx:], "</title>")
			if end > 7 {
				title = content[idx+7 : idx+end]
			}
		}
	}
	desc = extractMeta(content, "og:description")
	if desc == "" {
		desc = extractMeta(content, "description")
	}
	img = extractMeta(content, "og:image")
	if img == "" {
		img = extractMeta(content, "twitter:image")
	}
	return
}

func extractMeta(content, name string) string {
	patterns := []string{
		`property="` + name + `" content="`, `property='` + name + `' content='`,
		`name="` + name + `" content="`, `name='` + name + `' content='`,
	}
	for _, p := range patterns {
		idx := strings.Index(content, p)
		if idx >= 0 {
			start := idx + len(p)
			end := strings.Index(content[start:], `"`)
			if end < 0 {
				end = strings.Index(content[start:], "'")
			}
			if end > 0 && end < 500 {
				return content[start : start+end]
			}
		}
	}
	return ""
}

func checkSlugHandler(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)", code).Scan(&exists)
	jsonResp(w, map[string]interface{}{
		"code":    code,
		"available": !exists,
	})
}

func fetchTitleHandler(w http.ResponseWriter, r *http.Request) {
	var body struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.URL == "" {
		jsonErr(w, "url required", http.StatusBadRequest)
		return
	}
	title, desc, img := fetchPageMeta(body.URL)
	jsonResp(w, map[string]string{"title": title, "description": desc, "image": img})
}

func verifyPassword(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var body struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid", http.StatusBadRequest)
		return
	}
	var hash string
	db.QueryRow("SELECT password_hash FROM links WHERE code = ?", code).Scan(&hash)
	if hash == "" {
		jsonErr(w, "no password", http.StatusBadRequest)
		return
	}
	if hashPassword(body.Password) == hash {
		jsonResp(w, map[string]bool{"valid": true})
	} else {
		jsonErr(w, "invalid password", http.StatusUnauthorized)
	}
}

// ==================== PUBLIC ====================

func handleRedirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	if code == "" {
		http.NotFound(w, r)
		return
	}

	var url, passwordHash, title, desc, previewImg string
	var expiresAt int64
	err := db.QueryRow("SELECT url, password_hash, expires_at, title, description, preview_image FROM links WHERE code = ? AND is_active = 1",
		code).Scan(&url, &passwordHash, &expiresAt, &title, &desc, &previewImg)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if expiresAt > 0 && time.Now().Unix() > expiresAt {
		http.NotFound(w, r)
		return
	}
	if passwordHash != "" {
		ck, _ := r.Cookie("lp_" + code)
		if ck == nil || ck.Value != passwordHash[:16] {
			tmpl := template.Must(template.New("pass").Parse(passwordPage))
			tmpl.Execute(w, map[string]string{
				"Code": code, "URL": url, "Title": title,
				"Description": desc, "PreviewImage": previewImg,
				"Domain": r.Host, "HashPrefix": passwordHash[:16],
			})
			return
		}
	}

	db.Exec("INSERT INTO click_logs (code, clicked_at, referrer, user_agent, ip) VALUES (?, ?, ?, ?, ?)",
		code, time.Now().Unix(), r.Referer(), r.UserAgent(), r.RemoteAddr)
	db.Exec("UPDATE links SET clicks = clicks + 1 WHERE code = ?", code)
	http.Redirect(w, r, url, http.StatusFound)
}

func qrRedirect(w http.ResponseWriter, r *http.Request) {
	code := chi.URLParam(r, "code")
	var exists bool
	db.QueryRow("SELECT EXISTS(SELECT 1 FROM links WHERE code = ?)", code).Scan(&exists)
	if !exists {
		http.NotFound(w, r)
		return
	}
	shortURL := fmt.Sprintf("http://%s/%s", r.Host, code)
	png, err := qrcode.Encode(shortURL, qrcode.Medium, 512)
	if err != nil {
		http.Error(w, "qr error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(png)
}

// ==================== PUBLIC MICROSITE ====================

func renderMicrosite(w http.ResponseWriter, r *http.Request) {
	username := chi.URLParam(r, "username")
	var s Microsite
	err := db.QueryRow(`SELECT username, display_name, bio, avatar_url, bg_color, text_color, accent_color,
		font_family, btn_style, social_json, created_at FROM microsites WHERE username = ?`, username).
		Scan(&s.Username, &s.DisplayName, &s.Bio, &s.AvatarURL, &s.BgColor, &s.TextColor, &s.AccentColor,
			&s.FontFamily, &s.BtnStyle, &s.SocialJSON, &s.CreatedAt)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	rows, _ := db.Query("SELECT code, url, title, username, created_at, clicks, is_active FROM links WHERE username = ? AND is_active = 1 ORDER BY created_at ASC", username)
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var l Link
			rows.Scan(&l.Code, &l.URL, &l.Title, &l.Username, &l.CreatedAt, &l.Clicks, &l.IsActive)
			l.ShortURL = fmt.Sprintf("%s/%s", r.Host, l.Code)
			s.Links = append(s.Links, l)
		}
	}

	funcs := template.FuncMap{
		"parseJSON": func(data string) []map[string]string {
			var result []map[string]string
			json.Unmarshal([]byte(data), &result)
			return result
		},
	}
	tmpl := template.Must(template.New("microsite.html").Funcs(funcs).ParseFiles("templates/microsite.html"))
	tmpl.Execute(w, s)
}

// ==================== PAGES ====================

func renderLogin(w http.ResponseWriter) {
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, nil)
}

func renderLoginError(w http.ResponseWriter, err string) {
	tmpl := template.Must(template.ParseFiles("templates/login.html"))
	tmpl.Execute(w, map[string]string{"Error": err})
}

func dashboardPage(w http.ResponseWriter, r *http.Request) {
	u := getCurrentUser(r)
	tmpl := template.Must(template.ParseFiles("templates/admin.html"))
	tmpl.Execute(w, map[string]interface{}{
		"Username": func() string { if u != nil { return u.Username }; return "" }(),
		"Role":     func() string { if u != nil { return u.Role }; return "" }(),
		"IsAdmin":  func() bool { if u != nil { return u.Role == "admin" }; return false }(),
	})
}

// ==================== PASSWORD PAGE ====================

var passwordPage = `<!DOCTYPE html><html lang="en"><head>
<meta charset="UTF-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>Protected Link</title>
<script src="https://cdn.tailwindcss.com"></script>
<style>body{background:#0f172a;color:#e2e8f0;font-family:system-ui,sans-serif;min-height:100vh;display:flex;align-items:center;justify-content:center;padding:1rem}input{background:#0f172a!important;color:#e2e8f0!important;border:1px solid #475569!important;outline:none!important}input:focus{border-color:#3b82f6!important;box-shadow:0 0 0 2px rgba(59,130,246,0.3)!important}.card{background:#1e293b;border:1px solid #334155;border-radius:1rem;padding:2rem;width:100%;max-width:400px;text-align:center}.btn{background:#2563eb;color:white;padding:0.6rem 1.5rem;border-radius:0.5rem;font-weight:500;font-size:0.875rem;border:none;cursor:pointer}.btn:hover{background:#1d4ed8}</style>
</head><body><div class="card">
<div class="text-4xl mb-4">🔒</div>
<h2 class="text-xl font-bold text-white mb-1">Protected Link</h2>
{{if .Title}}<p class="text-sm text-gray-400 mb-2">{{.Title}}</p>{{end}}
{{if .PreviewImage}}<img src="{{.PreviewImage}}" class="w-full h-32 object-cover rounded-lg mb-4">{{end}}
<form id="pf" onsubmit="event.preventDefault();verify()">
<input id="pw" type="password" placeholder="Enter password" class="w-full px-4 py-2.5 rounded-lg text-sm mb-3 text-center" autofocus>
<p id="pwerr" class="text-red-400 text-xs mb-2 hidden">Wrong password!</p>
<button type="submit" class="btn w-full">Unlock Link →</button>
</form>
<p class="text-xs text-gray-500 mt-3">This link is password protected</p>
</div>
<script>
function verify(){const pw=document.getElementById('pw');fetch('/api/links/{{.Code}}/verify-password',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({password:pw.value})}).then(r=>r.json()).then(d=>{if(d.valid){document.cookie='lp_{{.Code}}={{.HashPrefix}};path=/;max-age=86400';window.location.href='/{{.Code}}'}else{document.getElementById('pwerr').classList.remove('hidden');pw.value=''}}).catch(()=>{document.getElementById('pwerr').classList.remove('hidden')})}
</script>
</body></html>`
