package main

import (
	"database/sql"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type App struct {
	db       *sql.DB
	dir      string
	secure   bool
	origin   string
	mu       sync.Mutex
	attempts map[string][]time.Time
}

func (a *App) routes() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.SetTrustedProxies(nil)
	r.Use(func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.Header("Cache-Control", "no-store")
		}
		if c.Request.Method != "GET" && c.Request.Method != "HEAD" {
			origin := c.GetHeader("Origin")
			if origin != "" {
				parsed, err := url.Parse(origin)
				allowed := err == nil && parsed.Host == c.Request.Host
				if a.origin != "" {
					allowed = origin == a.origin
				}
				if !allowed {
					fail(c, 403, "不允许跨站请求")
					return
				}
			}
			if c.GetHeader("Sec-Fetch-Site") == "cross-site" {
				fail(c, 403, "不允许跨站请求")
				return
			}
			if !strings.HasPrefix(c.Request.URL.Path, "/api/photos/upload") && !strings.HasPrefix(c.Request.URL.Path, "/api/backup/restore") {
				c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
			}
		}
		c.Next()
	})
	r.GET("/api/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "ok"}) })
	r.GET("/api/setup", func(c *gin.Context) {
		var count int
		if a.db.QueryRow(`SELECT count(*) FROM users`).Scan(&count) != nil {
			fail(c, 500, "读取账号失败")
			return
		}
		c.JSON(200, gin.H{"initialized": count > 0})
	})
	r.POST("/api/auth/register", a.rateLimit, a.register)
	r.POST("/api/auth/login", a.rateLimit, a.login)
	api := r.Group("/api", a.auth)
	api.GET("/auth/me", func(c *gin.Context) { c.JSON(200, current(c)) })
	api.POST("/auth/logout", func(c *gin.Context) {
		v, _ := c.Cookie("footprint_session")
		a.db.Exec(`DELETE FROM sessions WHERE hash=?`, digest(v))
		c.SetSameSite(http.SameSiteLaxMode)
		c.SetCookie("footprint_session", "", -1, "/", "", a.secure, true)
		c.Status(204)
	})
	api.POST("/invite", func(c *gin.Context) {
		var count int
		u := current(c)
		if a.db.QueryRow(`SELECT count(*) FROM users WHERE space_id=?`, u.SpaceID).Scan(&count) != nil {
			fail(c, 500, "读取成员失败")
			return
		}
		if count >= 2 {
			fail(c, 409, "你们已经在同一个空间了")
			return
		}
		v := token()
		if _, err := a.db.Exec(`UPDATE spaces SET invite_hash=?,invite_expires=? WHERE id=?`, digest(v), time.Now().Add(24*time.Hour).Unix(), u.SpaceID); err != nil {
			fail(c, 500, "生成邀请失败")
			return
		}
		c.JSON(200, gin.H{"invite": v})
	})
	api.GET("/photos", func(c *gin.Context) {
		u := current(c)
		photos, err := a.list(u.SpaceID)
		if err != nil {
			fail(c, 500, "加载照片失败")
			return
		}
		rows, err := a.db.Query(`SELECT id,name,space_id FROM users WHERE space_id=? ORDER BY rowid`, u.SpaceID)
		if err != nil {
			fail(c, 500, "加载成员失败")
			return
		}
		defer rows.Close()
		members := []User{}
		for rows.Next() {
			var member User
			if rows.Scan(&member.ID, &member.Name, &member.SpaceID) != nil {
				fail(c, 500, "加载成员失败")
				return
			}
			members = append(members, member)
		}
		c.JSON(200, gin.H{"photos": photos, "members": members})
	})
	api.POST("/photos/upload", a.upload)
	a.memoryRoutes(api)
	a.geocodeRoutes(api)
	a.featureRoutes(api)
	a.exportRoutes(api)
	a.storyRoute(api)
	a.systemRoutes(api)
	a.pairingRoutes(api)
	api.GET("/photos/:id/thumb", a.thumbnail)
	api.GET("/photos/:id/file", func(c *gin.Context) {
		p, ok := a.photo(c)
		if !ok {
			return
		}
		c.Header("Cache-Control", "private, no-cache")
		c.Header("Vary", "Cookie")
		c.Header("Content-Disposition", "inline")
		c.File(filepath.Join(a.dir, "photos", p.Filename))
	})
	api.PUT("/photos/:id", func(c *gin.Context) {
		p, ok := a.photo(c)
		if !ok {
			return
		}
		var data Photo
		if c.ShouldBindJSON(&data) != nil {
			fail(c, 400, "照片信息格式错误")
			return
		}
		if err := validate(data); err != nil {
			fail(c, 400, err.Error())
			return
		}
		_, err := a.db.Exec(`UPDATE photos SET title=?,note=?,place=?,taken_at=?,latitude=?,longitude=? WHERE id=? AND space_id=?`, strings.TrimSpace(data.Title), data.Note, strings.TrimSpace(data.Place), data.TakenAt, data.Latitude, data.Longitude, p.ID, current(c).SpaceID)
		if err != nil {
			fail(c, 500, "保存失败")
			return
		}
		p.Title = strings.TrimSpace(data.Title)
		p.Note = data.Note
		p.Place = strings.TrimSpace(data.Place)
		p.TakenAt = data.TakenAt
		p.Latitude = data.Latitude
		p.Longitude = data.Longitude
		c.JSON(200, p)
	})
	api.DELETE("/photos/:id", func(c *gin.Context) {
		p, ok := a.photo(c)
		if !ok {
			return
		}
		if _, err := a.db.Exec(`DELETE FROM photos WHERE id=? AND space_id=?`, p.ID, current(c).SpaceID); err != nil {
			fail(c, 500, "删除失败")
			return
		}
		os.Remove(filepath.Join(a.dir, "photos", p.Filename))
		c.Status(204)
	})
	api.GET("/backup", a.backup)
	api.POST("/backup/restore", a.restore)
	r.Static("/assets", "../web/dist/assets")
	r.Static("/icons", "../web/dist/icons")
	r.GET("/manifest.webmanifest", func(c *gin.Context) {
		c.Header("Content-Type", "application/manifest+json")
		c.Header("Cache-Control", "no-cache")
		c.File("../web/dist/manifest.webmanifest")
	})
	r.GET("/sw.js", func(c *gin.Context) {
		c.Header("Content-Type", "text/javascript; charset=utf-8")
		c.Header("Cache-Control", "no-cache")
		c.Header("Service-Worker-Allowed", "/")
		c.File("../web/dist/sw.js")
	})
	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			fail(c, 404, "接口不存在")
			return
		}
		if c.Request.Method != "GET" {
			c.Status(404)
			return
		}
		if _, err := os.Stat("../web/dist/index.html"); err != nil {
			c.String(404, "请先构建 Vue 前端，或访问开发服务器 http://localhost:5173")
			return
		}
		c.File("../web/dist/index.html")
	})
	return r
}
