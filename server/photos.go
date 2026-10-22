package main

import (
	"bytes"
	"database/sql"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/rwcarlsen/goexif/exif"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const photoColumns = "id,filename,title,note,place,taken_at,latitude,longitude,owner_id,created_at"

func readPhoto(s interface{ Scan(...any) error }) (Photo, error) {
	var p Photo
	err := s.Scan(&p.ID, &p.Filename, &p.Title, &p.Note, &p.Place, &p.TakenAt, &p.Latitude, &p.Longitude, &p.OwnerID, &p.CreatedAt)
	return p, err
}
func (a *App) list(space string) ([]Photo, error) {
	rows, err := a.db.Query(`SELECT `+photoColumns+` FROM photos WHERE space_id=? ORDER BY taken_at DESC,created_at DESC`, space)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	photos := []Photo{}
	for rows.Next() {
		p, err := readPhoto(rows)
		if err != nil {
			return nil, err
		}
		photos = append(photos, p)
	}
	return photos, rows.Err()
}
func validate(p Photo) error {
	if len([]rune(p.Title)) > 120 || len([]rune(p.Note)) > 5000 || len([]rune(p.Place)) > 200 {
		return errors.New("标题、备注或地点过长")
	}
	if _, err := time.Parse("2006-01-02", p.TakenAt); err != nil {
		return errors.New("请选择有效的拍摄日期")
	}
	if (p.Latitude == nil) != (p.Longitude == nil) {
		return errors.New("经纬度需要同时填写")
	}
	if p.Latitude != nil && (math.IsNaN(*p.Latitude) || math.IsNaN(*p.Longitude) || math.IsInf(*p.Latitude, 0) || math.IsInf(*p.Longitude, 0) || *p.Latitude < -90 || *p.Latitude > 90 || *p.Longitude < -180 || *p.Longitude > 180) {
		return errors.New("经纬度超出有效范围")
	}
	return nil
}
func insertPhoto(tx *sql.Tx, space string, p Photo) error {
	_, err := tx.Exec(`INSERT INTO photos(id,space_id,filename,title,note,place,taken_at,latitude,longitude,owner_id,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?)`, p.ID, space, p.Filename, p.Title, p.Note, p.Place, p.TakenAt, p.Latitude, p.Longitude, p.OwnerID, p.CreatedAt)
	return err
}
func imageExtension(b []byte) (string, error) {
	kind := http.DetectContentType(b)
	ext := map[string]string{"image/jpeg": ".jpg", "image/png": ".png", "image/gif": ".gif", "image/webp": ".webp"}[kind]
	if ext == "" {
		return "", errors.New("目前支持 JPG、PNG、GIF 和 WebP；请先将 HEIC 转换为 JPG")
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil || cfg.Width <= 0 || cfg.Height <= 0 || int64(cfg.Width)*int64(cfg.Height) > 100000000 {
		return "", errors.New("图片无效或超过一亿像素")
	}
	return ext, nil
}
func (a *App) upload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 105<<20)
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		fail(c, 400, "上传失败：每批不能超过 100MB")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	files := c.Request.MultipartForm.File["photos"]
	if len(files) == 0 || len(files) > 20 {
		fail(c, 400, "每次请选择 1–20 张照片")
		return
	}
	u := current(c)
	photos := []Photo{}
	written := []string{}
	success := false
	defer func() {
		if !success {
			for _, path := range written {
				os.Remove(path)
			}
		}
	}()
	for _, header := range files {
		if header.Size > 20<<20 {
			fail(c, 400, "单张照片不能超过 20MB")
			return
		}
		f, err := header.Open()
		if err != nil {
			fail(c, 400, "无法读取照片")
			return
		}
		b, err := io.ReadAll(io.LimitReader(f, (20<<20)+1))
		f.Close()
		if err != nil || len(b) > 20<<20 {
			fail(c, 400, "无法读取照片或照片过大")
			return
		}
		ext, err := imageExtension(b)
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		p := Photo{ID: token(), Title: strings.TrimSuffix(filepath.Base(header.Filename), filepath.Ext(header.Filename)), TakenAt: time.Now().Format("2006-01-02"), OwnerID: u.ID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
		if len([]rune(p.Title)) > 120 {
			p.Title = string([]rune(p.Title)[:120])
		}
		if x, err := exif.Decode(bytes.NewReader(b)); err == nil {
			if t, err := x.DateTime(); err == nil {
				p.TakenAt = t.Format("2006-01-02")
			}
			if lat, lon, err := x.LatLong(); err == nil {
				p.Latitude = &lat
				p.Longitude = &lon
			}
		}
		if value := c.PostForm("taken_at"); value != "" {
			p.TakenAt = value
		}
		p.Place = strings.TrimSpace(c.PostForm("place"))
		p.Note = c.PostForm("note")
		if err := validate(p); err != nil {
			fail(c, 400, err.Error())
			return
		}
		p.Filename = p.ID + ext
		path := filepath.Join(a.dir, "photos", p.Filename)
		if os.WriteFile(path, b, 0600) != nil {
			fail(c, 500, "保存照片失败")
			return
		}
		written = append(written, path)
		photos = append(photos, p)
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "保存照片失败")
		return
	}
	defer tx.Rollback()
	for _, p := range photos {
		if err = insertPhoto(tx, u.SpaceID, p); err != nil {
			fail(c, 500, "保存照片失败")
			return
		}
	}
	memoryID := c.PostForm("memory_id")
	if memoryID != "" {
		var count int
		if tx.QueryRow(`SELECT count(*) FROM memories WHERE id=? AND space_id=?`, memoryID, u.SpaceID).Scan(&count) != nil || count == 0 {
			fail(c, 404, "要追加的回忆不存在")
			return
		}
		var position int
		tx.QueryRow(`SELECT COALESCE(MAX(position),-1)+1 FROM memory_photos WHERE memory_id=?`, memoryID).Scan(&position)
		if position+len(photos) > 200 {
			fail(c, 400, "每段回忆最多 200 张照片")
			return
		}
		for i, p := range photos {
			if _, err = tx.Exec(`INSERT INTO memory_photos VALUES(?,?,?)`, memoryID, p.ID, position+i); err != nil {
				fail(c, 500, "关联照片失败")
				return
			}
		}
		tx.Exec(`UPDATE memories SET updated_at=? WHERE id=?`, time.Now().UTC().Format(time.RFC3339Nano), memoryID)
	} else {
		m, err := memoryFromPhotos(u, photos, c.PostForm("title"), c.PostForm("note"), c.PostForm("mode"), c.PostForm("taken_at"), c.PostForm("place"), c.PostForm("province"), c.PostForm("city"))
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		if c.PostForm("latitude") != "" && c.PostForm("longitude") != "" {
			lat, e1 := strconv.ParseFloat(c.PostForm("latitude"), 64)
			lon, e2 := strconv.ParseFloat(c.PostForm("longitude"), 64)
			if e1 != nil || e2 != nil {
				fail(c, 400, "回忆经纬度无效")
				return
			}
			m.Latitude = &lat
			m.Longitude = &lon
			if err = validateMemory(&m); err != nil {
				fail(c, 400, err.Error())
				return
			}
		}
		if err = insertMemory(tx, u.SpaceID, m); err != nil {
			fail(c, 500, "创建回忆失败")
			return
		}
		memoryID = m.ID
		ids := []string{}
		for _, p := range photos {
			ids = append(ids, p.ID)
		}
		if err = attachPhotos(tx, u.SpaceID, memoryID, ids); err != nil {
			fail(c, 500, "关联照片失败")
			return
		}
	}
	if tx.Commit() != nil {
		fail(c, 500, "保存照片失败")
		return
	}
	success = true
	c.JSON(201, gin.H{"photos": photos, "memory_id": memoryID})
}
func (a *App) photo(c *gin.Context) (Photo, bool) {
	p, err := readPhoto(a.db.QueryRow(`SELECT `+photoColumns+` FROM photos WHERE id=? AND space_id=?`, c.Param("id"), current(c).SpaceID))
	if err != nil {
		fail(c, 404, "照片不存在")
		return p, false
	}
	return p, true
}
