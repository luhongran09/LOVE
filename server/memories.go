package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Memory struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Mode        string   `json:"mode"`
	Date        string   `json:"date"`
	Place       string   `json:"place"`
	Province    string   `json:"province"`
	City        string   `json:"city"`
	Latitude    *float64 `json:"latitude"`
	Longitude   *float64 `json:"longitude"`
	Tags        []string `json:"tags"`
	Rating      int      `json:"rating"`
	Price       float64  `json:"price"`
	CoverID     string   `json:"cover_id"`
	OwnerID     string   `json:"owner_id"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
	Photos      []Photo  `json:"photos"`
}
type memoryInput struct {
	Memory
	PhotoIDs []string `json:"photo_ids"`
}

const memoryColumns = `id,title,description,mode,date,place,province,city,latitude,longitude,tags,rating,price,cover_id,owner_id,created_at,updated_at`

func migrateMemories(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`CREATE TABLE IF NOT EXISTS memories (
 id TEXT PRIMARY KEY,space_id TEXT NOT NULL REFERENCES spaces(id),title TEXT NOT NULL,description TEXT NOT NULL DEFAULT '',mode TEXT NOT NULL DEFAULT 'travel',date TEXT NOT NULL,place TEXT NOT NULL DEFAULT '',province TEXT NOT NULL DEFAULT '',city TEXT NOT NULL DEFAULT '',latitude REAL,longitude REAL,tags TEXT NOT NULL DEFAULT '[]',rating INTEGER NOT NULL DEFAULT 0,price REAL NOT NULL DEFAULT 0,cover_id TEXT NOT NULL DEFAULT '',owner_id TEXT NOT NULL REFERENCES users(id),created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
 CREATE TABLE IF NOT EXISTS memory_photos (memory_id TEXT NOT NULL REFERENCES memories(id) ON DELETE CASCADE,photo_id TEXT NOT NULL UNIQUE REFERENCES photos(id) ON DELETE CASCADE,position INTEGER NOT NULL,PRIMARY KEY(memory_id,photo_id));
 CREATE INDEX IF NOT EXISTS memories_space_date ON memories(space_id,date DESC);
 CREATE INDEX IF NOT EXISTS memory_photos_order ON memory_photos(memory_id,position);
 CREATE TABLE IF NOT EXISTS features(space_id TEXT NOT NULL REFERENCES spaces(id),key TEXT NOT NULL,data TEXT NOT NULL,updated_at TEXT NOT NULL,PRIMARY KEY(space_id,key));`)
	if err != nil {
		return err
	}
	if err = seedPhotoMemories(tx); err != nil {
		return err
	}
	if _, err = tx.Exec(`CREATE TABLE IF NOT EXISTS space_revisions(space_id TEXT PRIMARY KEY,revision INTEGER NOT NULL DEFAULT 0);`); err != nil {
		return err
	}
	for _, table := range []string{"photos", "memories", "features", "users"} {
		for _, event := range []string{"INSERT", "UPDATE", "DELETE"} {
			row := "NEW"
			if event == "DELETE" {
				row = "OLD"
			}
			statement := fmt.Sprintf(`CREATE TRIGGER IF NOT EXISTS revision_%s_%s AFTER %s ON %s BEGIN INSERT INTO space_revisions VALUES(%s.space_id,1) ON CONFLICT(space_id) DO UPDATE SET revision=revision+1; END`, table, event, event, table, row)
			if _, err = tx.Exec(statement); err != nil {
				return err
			}
		}
	}
	if _, err = tx.Exec(`PRAGMA user_version=2`); err != nil {
		return err
	}
	return tx.Commit()
}
func validateMemory(m *Memory) error {
	m.Title = strings.TrimSpace(m.Title)
	m.Place = strings.TrimSpace(m.Place)
	m.City = strings.TrimSpace(m.City)
	m.Province = strings.TrimSpace(m.Province)
	if m.Title == "" {
		return errors.New("请为这段回忆起个名字")
	}
	if m.Mode == "" {
		m.Mode = "travel"
	}
	if m.Mode != "travel" && m.Mode != "food" && m.Mode != "love" {
		return errors.New("回忆类型无效")
	}
	if err := validate(Photo{Title: m.Title, Note: m.Description, Place: m.Place, TakenAt: m.Date, Latitude: m.Latitude, Longitude: m.Longitude}); err != nil {
		return err
	}
	if len([]rune(m.City)) > 100 || len([]rune(m.Province)) > 100 || m.Rating < 0 || m.Rating > 5 || m.Price < 0 || m.Price > 100000000 {
		return errors.New("城市、评分或价格无效")
	}
	if len(m.Tags) > 20 {
		return errors.New("最多添加 20 个标签")
	}
	for _, tag := range m.Tags {
		if len([]rune(tag)) > 30 {
			return errors.New("标签最多 30 个字")
		}
	}
	if m.Tags == nil {
		m.Tags = []string{}
	}
	return nil
}
func readMemory(s interface{ Scan(...any) error }) (Memory, error) {
	var m Memory
	var tags string
	err := s.Scan(&m.ID, &m.Title, &m.Description, &m.Mode, &m.Date, &m.Place, &m.Province, &m.City, &m.Latitude, &m.Longitude, &tags, &m.Rating, &m.Price, &m.CoverID, &m.OwnerID, &m.CreatedAt, &m.UpdatedAt)
	if err == nil {
		err = json.Unmarshal([]byte(tags), &m.Tags)
	}
	m.Photos = []Photo{}
	return m, err
}
func (a *App) memories(space string) ([]Memory, error) {
	return a.queryMemories(space, "", nil, 0, 0)
}

func (a *App) queryMemories(space, filter string, args []any, limit, offset int) ([]Memory, error) {
	return a.readMemories(space, filter, args, limit, offset, false)
}
func (a *App) readMemories(space, filter string, args []any, limit, offset int, compact bool) ([]Memory, error) {
	query := `SELECT ` + memoryColumns + ` FROM memories WHERE space_id=?` + filter + ` ORDER BY date DESC,created_at DESC,id DESC`
	params := append([]any{space}, args...)
	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		params = append(params, limit, offset)
	}
	rows, err := a.db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	result := []Memory{}
	positions := map[string]int{}
	for rows.Next() {
		m, err := readMemory(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		positions[m.ID] = len(result)
		result = append(result, m)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	photoArgs := append([]any{space}, params...)
	noteColumn := "p.note"
	if compact {
		noteColumn = "''"
	}
	rows, err = a.db.Query(`SELECT mp.memory_id,p.id,p.filename,p.title,`+noteColumn+`,p.place,p.taken_at,p.latitude,p.longitude,p.owner_id,p.created_at FROM memory_photos mp JOIN photos p ON p.id=mp.photo_id WHERE p.space_id=? AND mp.memory_id IN (SELECT id FROM (`+query+`)) ORDER BY mp.position`, photoArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var memoryID string
		var p Photo
		if err = rows.Scan(&memoryID, &p.ID, &p.Filename, &p.Title, &p.Note, &p.Place, &p.TakenAt, &p.Latitude, &p.Longitude, &p.OwnerID, &p.CreatedAt); err != nil {
			return nil, err
		}
		if index, ok := positions[memoryID]; ok {
			result[index].Photos = append(result[index].Photos, p)
		}
	}
	for i := range result {
		valid := false
		for _, p := range result[i].Photos {
			if p.ID == result[i].CoverID {
				valid = true
			}
		}
		if !valid && len(result[i].Photos) > 0 {
			result[i].CoverID = result[i].Photos[0].ID
		}
	}
	return result, rows.Err()
}
func insertMemory(tx *sql.Tx, space string, m Memory) error {
	tags, _ := json.Marshal(m.Tags)
	_, err := tx.Exec(`INSERT INTO memories(id,space_id,title,description,mode,date,place,province,city,latitude,longitude,tags,rating,price,cover_id,owner_id,created_at,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, m.ID, space, m.Title, m.Description, m.Mode, m.Date, m.Place, m.Province, m.City, m.Latitude, m.Longitude, string(tags), m.Rating, m.Price, m.CoverID, m.OwnerID, m.CreatedAt, m.UpdatedAt)
	return err
}
func attachPhotos(tx *sql.Tx, space, id string, ids []string) error {
	if len(ids) == 0 || len(ids) > 200 {
		return errors.New("每段回忆需要 1–200 张照片")
	}
	seen := map[string]bool{}
	for _, photoID := range ids {
		if seen[photoID] {
			return errors.New("照片不能重复")
		}
		seen[photoID] = true
		var owner string
		if err := tx.QueryRow(`SELECT space_id FROM photos WHERE id=?`, photoID).Scan(&owner); err != nil || owner != space {
			return errors.New("照片不存在或不属于当前空间")
		}
	}
	previous := map[string]bool{}
	for _, photoID := range ids {
		var old string
		if tx.QueryRow(`SELECT memory_id FROM memory_photos WHERE photo_id=?`, photoID).Scan(&old) == nil {
			previous[old] = true
		}
	}
	if _, err := tx.Exec(`DELETE FROM memory_photos WHERE memory_id=?`, id); err != nil {
		return err
	}
	// A photo belongs to a single memory. Move it atomically, without deleting originals.
	for _, photoID := range ids {
		if _, err := tx.Exec(`DELETE FROM memory_photos WHERE photo_id=?`, photoID); err != nil {
			return err
		}
	}
	for position, photoID := range ids {
		if _, err := tx.Exec(`INSERT INTO memory_photos VALUES(?,?,?)`, id, photoID, position); err != nil {
			return err
		}
	}
	for old := range previous {
		if old != id {
			if _, err := tx.Exec(`DELETE FROM memories WHERE id=? AND space_id=? AND NOT EXISTS(SELECT 1 FROM memory_photos WHERE memory_id=memories.id)`, old, space); err != nil {
				return err
			}
		}
	}
	return nil
}
func memoryFromPhotos(u User, photos []Photo, title, description, mode, date, place, province, city string) (Memory, error) {
	first := photos[0]
	if title == "" {
		title = first.Title
	}
	if strings.TrimSpace(title) == "" {
		title = "未命名的回忆"
	}
	if date == "" {
		date = first.TakenAt
	}
	if place == "" {
		place = first.Place
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	m := Memory{ID: token(), Title: title, Description: description, Mode: mode, Date: date, Place: place, Province: province, City: city, Latitude: first.Latitude, Longitude: first.Longitude, OwnerID: u.ID, CoverID: first.ID, CreatedAt: now, UpdatedAt: now, Tags: []string{}, Photos: photos}
	err := validateMemory(&m)
	return m, err
}
func (a *App) saveMemory(c *gin.Context) {
	var input memoryInput
	if c.ShouldBindJSON(&input) != nil {
		fail(c, 400, "回忆数据格式错误")
		return
	}
	if err := validateMemory(&input.Memory); err != nil {
		fail(c, 400, err.Error())
		return
	}
	u := current(c)
	id := c.Param("id")
	creating := id == ""
	if creating {
		id = token()
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "保存失败")
		return
	}
	defer tx.Rollback()
	m := input.Memory
	m.ID = id
	m.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if creating {
		m.OwnerID = u.ID
		m.CreatedAt = m.UpdatedAt
		if len(input.PhotoIDs) == 0 {
			fail(c, 400, "请先选择照片")
			return
		}
		if err = insertMemory(tx, u.SpaceID, m); err != nil {
			fail(c, 500, "保存失败")
			return
		}
	} else {
		existing, err := readMemory(tx.QueryRow(`SELECT `+memoryColumns+` FROM memories WHERE id=? AND space_id=?`, id, u.SpaceID))
		if err != nil {
			fail(c, 404, "回忆不存在")
			return
		}
		if input.UpdatedAt != "" && input.UpdatedAt != existing.UpdatedAt {
			fail(c, 409, "这段回忆刚被更新，请关闭后重新打开再修改")
			return
		}
		m.OwnerID = existing.OwnerID
		m.CreatedAt = existing.CreatedAt
		if len(input.PhotoIDs) == 0 {
			rows, err := tx.Query(`SELECT photo_id FROM memory_photos WHERE memory_id=? ORDER BY position`, id)
			if err != nil {
				fail(c, 500, "读取照片失败")
				return
			}
			for rows.Next() {
				var photoID string
				rows.Scan(&photoID)
				input.PhotoIDs = append(input.PhotoIDs, photoID)
			}
			rows.Close()
		}
	}
	if err = attachPhotos(tx, u.SpaceID, id, input.PhotoIDs); err != nil {
		fail(c, 400, err.Error())
		return
	}
	coverValid := false
	for _, photoID := range input.PhotoIDs {
		if photoID == m.CoverID {
			coverValid = true
		}
	}
	if !coverValid {
		m.CoverID = input.PhotoIDs[0]
	}
	tags, _ := json.Marshal(m.Tags)
	_, err = tx.Exec(`UPDATE memories SET title=?,description=?,mode=?,date=?,place=?,province=?,city=?,latitude=?,longitude=?,tags=?,rating=?,price=?,cover_id=?,updated_at=? WHERE id=? AND space_id=?`, m.Title, m.Description, m.Mode, m.Date, m.Place, m.Province, m.City, m.Latitude, m.Longitude, string(tags), m.Rating, m.Price, m.CoverID, m.UpdatedAt, id, u.SpaceID)
	if err == nil {
		err = seedPhotoMemories(tx)
	}
	if err != nil || tx.Commit() != nil {
		fail(c, 500, "保存失败")
		return
	}
	c.JSON(200, gin.H{"id": id})
}
func (a *App) deleteMemory(c *gin.Context) {
	u := current(c)
	id := c.Param("id")
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "删除失败")
		return
	}
	defer tx.Rollback()
	var exists int
	if tx.QueryRow(`SELECT count(*) FROM memories WHERE id=? AND space_id=?`, id, u.SpaceID).Scan(&exists) != nil || exists == 0 {
		fail(c, 404, "回忆不存在")
		return
	}
	rows, err := tx.Query(`SELECT p.id,p.filename FROM photos p JOIN memory_photos mp ON mp.photo_id=p.id WHERE mp.memory_id=? AND p.space_id=?`, id, u.SpaceID)
	if err != nil {
		fail(c, 500, "读取失败")
		return
	}
	ids := []string{}
	files := []string{}
	for rows.Next() {
		var photoID, name string
		rows.Scan(&photoID, &name)
		ids = append(ids, photoID)
		files = append(files, name)
	}
	rows.Close()
	for _, photoID := range ids {
		if _, err = tx.Exec(`DELETE FROM photos WHERE id=? AND space_id=?`, photoID, u.SpaceID); err != nil {
			fail(c, 500, "删除失败")
			return
		}
	}
	if _, err = tx.Exec(`DELETE FROM memories WHERE id=? AND space_id=?`, id, u.SpaceID); err != nil || tx.Commit() != nil {
		fail(c, 500, "删除失败")
		return
	}
	for _, name := range files {
		os.Remove(filepath.Join(a.dir, "photos", name))
	}
	c.Status(http.StatusNoContent)
}
func (a *App) memoryRoutes(api *gin.RouterGroup) {
	api.GET("/memories", a.listMemories)
	api.GET("/memories/:id", func(c *gin.Context) {
		items, err := a.queryMemories(current(c).SpaceID, " AND id=?", []any{c.Param("id")}, 1, 0)
		if err != nil {
			fail(c, 500, "加载回忆失败")
			return
		}
		if len(items) == 0 {
			fail(c, 404, "回忆不存在")
			return
		}
		c.JSON(200, items[0])
	})
	api.GET("/library", func(c *gin.Context) {
		u := current(c)
		items, err := a.readMemories(u.SpaceID, "", nil, 0, 0, true)
		if err != nil {
			fail(c, 500, "加载足迹索引失败")
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
			if err = rows.Scan(&member.ID, &member.Name, &member.SpaceID); err != nil {
				fail(c, 500, "加载成员失败")
				return
			}
			members = append(members, member)
		}
		if rows.Err() != nil {
			fail(c, 500, "加载成员失败")
			return
		}
		c.JSON(200, gin.H{"memories": items, "members": members})
	})
	api.POST("/memories", a.saveMemory)
	api.PUT("/memories/:id", a.saveMemory)
	api.DELETE("/memories/:id", a.deleteMemory)
}

// Seed an independent memory for every unattached photo without changing files or metadata.
func seedPhotoMemories(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT p.id,p.filename,p.title,p.note,p.place,p.taken_at,p.latitude,p.longitude,p.owner_id,p.created_at,p.space_id FROM photos p WHERE NOT EXISTS(SELECT 1 FROM memory_photos mp WHERE mp.photo_id=p.id)`)
	if err != nil {
		return err
	}
	type orphan struct {
		p     Photo
		space string
	}
	items := []orphan{}
	for rows.Next() {
		var item orphan
		if err = rows.Scan(&item.p.ID, &item.p.Filename, &item.p.Title, &item.p.Note, &item.p.Place, &item.p.TakenAt, &item.p.Latitude, &item.p.Longitude, &item.p.OwnerID, &item.p.CreatedAt, &item.space); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, item := range items {
		m, err := memoryFromPhotos(User{ID: item.p.OwnerID, SpaceID: item.space}, []Photo{item.p}, item.p.Title, item.p.Note, "travel", item.p.TakenAt, item.p.Place, "", "")
		if err != nil {
			return err
		}
		m.CreatedAt = item.p.CreatedAt
		if err = insertMemory(tx, item.space, m); err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO memory_photos VALUES(?,?,0)`, m.ID, item.p.ID); err != nil {
			return err
		}
	}
	return nil
}

// Pagination is opt-in to retain compatibility with backups and existing clients.
func (a *App) listMemories(c *gin.Context) {
	space := current(c).SpaceID
	if c.Query("page") == "" && c.Query("per_page") == "" {
		items, err := a.memories(space)
		if err != nil {
			fail(c, 500, "加载回忆失败")
			return
		}
		c.JSON(200, gin.H{"memories": items})
		return
	}
	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, sizeErr := strconv.Atoi(c.DefaultQuery("per_page", "30"))
	if err != nil || sizeErr != nil || page < 1 || page > 1000000 || size < 1 || size > 100 {
		fail(c, 400, "分页参数无效")
		return
	}
	filter := ""
	args := []any{}
	if mode := c.Query("mode"); mode != "" {
		if mode != "travel" && mode != "food" && mode != "love" {
			fail(c, 400, "回忆分类无效")
			return
		}
		filter += " AND mode=?"
		args = append(args, mode)
	}
	if year := c.Query("year"); year != "" {
		if len(year) != 4 || strings.Trim(year, "0123456789") != "" {
			fail(c, 400, "年份无效")
			return
		}
		filter += " AND substr(date,1,4)=?"
		args = append(args, year)
	}
	if place := c.Query("place"); place != "" {
		filter += " AND (place=? OR city=?)"
		args = append(args, place, place)
	}
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		if len(q) > 1000 {
			fail(c, 400, "搜索内容过长")
			return
		}
		filter += " AND instr(lower(title||' '||description||' '||place||' '||city||' '||tags),lower(?))>0"
		args = append(args, q)
	}
	var total int
	if err = a.db.QueryRow("SELECT count(*) FROM memories WHERE space_id=?"+filter, append([]any{space}, args...)...).Scan(&total); err != nil {
		fail(c, 500, "加载回忆失败")
		return
	}
	items, err := a.queryMemories(space, filter, args, size, (page-1)*size)
	if err != nil {
		fail(c, 500, "加载回忆失败")
		return
	}
	c.JSON(200, gin.H{"memories": items, "total": total, "page": page, "per_page": size})
}
