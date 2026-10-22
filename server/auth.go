package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"net/http"
	"strings"
	"time"
)

func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { v := sha256.Sum256([]byte(s)); return hex.EncodeToString(v[:]) }
func fail(c *gin.Context, code int, message string) {
	c.AbortWithStatusJSON(code, gin.H{"error": message})
}
func current(c *gin.Context) User { return c.MustGet("user").(User) }
func (a *App) auth(c *gin.Context) {
	value, err := c.Cookie("footprint_session")
	if err != nil {
		fail(c, 401, "请先登录")
		return
	}
	var user User
	err = a.db.QueryRow(`SELECT u.id,u.name,u.space_id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.hash=? AND s.expires>?`, digest(value), time.Now().Unix()).Scan(&user.ID, &user.Name, &user.SpaceID)
	if err != nil {
		fail(c, 401, "登录已过期，请重新登录")
		return
	}
	c.Set("user", user)
	c.Header("Cache-Control", "no-store")
	c.Next()
}
func (a *App) session(c *gin.Context, u User) {
	value := token()
	_, err := a.db.Exec(`INSERT INTO sessions VALUES(?,?,?)`, digest(value), u.ID, time.Now().Add(7*24*time.Hour).Unix())
	if err != nil {
		fail(c, 500, "无法创建登录会话")
		return
	}
	a.db.Exec(`DELETE FROM sessions WHERE expires<?`, time.Now().Unix())
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie("footprint_session", value, 7*86400, "/", "", a.secure, true)
	c.JSON(200, u)
}
func (a *App) rateLimit(c *gin.Context) {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	key := c.ClientIP()
	recent := []time.Time{}
	for k, values := range a.attempts {
		if len(values) == 0 || now.Sub(values[len(values)-1]) > 15*time.Minute {
			delete(a.attempts, k)
		}
	}
	for _, t := range a.attempts[key] {
		if now.Sub(t) < 15*time.Minute {
			recent = append(recent, t)
		}
	}
	if len(recent) >= 20 {
		fail(c, 429, "尝试次数过多，请 15 分钟后再试")
		return
	}
	a.attempts[key] = append(recent, now)
}
func (a *App) register(c *gin.Context) {
	var input struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Invite   string `json:"invite"`
	}
	if c.ShouldBindJSON(&input) != nil {
		fail(c, 400, "请输入账号和密码")
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	if len([]rune(input.Name)) < 2 || len([]rune(input.Name)) > 30 || len(input.Password) < 8 || len(input.Password) > 72 {
		fail(c, 400, "账号需 2–30 个字，密码需 8–72 字节")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
	if err != nil {
		fail(c, 500, "密码处理失败")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "数据库暂时不可用")
		return
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRow(`SELECT count(*) FROM users`).Scan(&count); err != nil {
		fail(c, 500, "读取账号失败")
		return
	}
	space := token()
	if count == 0 {
		_, err = tx.Exec(`INSERT INTO spaces(id) VALUES(?)`, space)
	} else {
		err = tx.QueryRow(`SELECT id FROM spaces WHERE invite_hash=? AND invite_expires>?`, digest(strings.TrimSpace(input.Invite)), time.Now().Unix()).Scan(&space)
		if err != nil {
			fail(c, 403, "需要有效的伴侣邀请码才能加入")
			return
		}
		if err = tx.QueryRow(`SELECT count(*) FROM users WHERE space_id=?`, space).Scan(&count); err != nil {
			fail(c, 500, "读取空间失败")
			return
		}
		if count >= 2 {
			fail(c, 409, "这个空间已有两位成员")
			return
		}
		_, err = tx.Exec(`UPDATE spaces SET invite_hash=NULL,invite_expires=NULL WHERE id=?`, space)
	}
	if err != nil {
		fail(c, 500, "创建空间失败")
		return
	}
	u := User{token(), input.Name, space}
	_, err = tx.Exec(`INSERT INTO users VALUES(?,?,?,?)`, u.ID, u.Name, string(hash), space)
	if err != nil {
		fail(c, 409, "账号已存在，请换一个名字")
		return
	}
	if tx.Commit() != nil {
		fail(c, 500, "注册失败")
		return
	}
	a.session(c, u)
}
func (a *App) login(c *gin.Context) {
	var input struct {
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if c.ShouldBindJSON(&input) != nil {
		fail(c, 400, "请输入账号和密码")
		return
	}
	var u User
	var hash string
	err := a.db.QueryRow(`SELECT id,name,space_id,password FROM users WHERE name=?`, strings.TrimSpace(input.Name)).Scan(&u.ID, &u.Name, &u.SpaceID, &hash)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
		fail(c, 401, "账号或密码错误")
		return
	}
	a.session(c, u)
}
