package main

import (
	"archive/zip"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type Manifest struct {
	Version    int                        `json:"version"`
	ExportedAt string                     `json:"exported_at"`
	Photos     []Photo                    `json:"photos"`
	Memories   []Memory                   `json:"memories,omitempty"`
	Features   map[string]json.RawMessage `json:"features,omitempty"`
}

func backupLimit() int64 {
	mb, err := strconv.ParseInt(os.Getenv("BACKUP_MAX_MB"), 10, 64)
	if err != nil || mb < 1 || mb > 8192 {
		mb = 250
	}
	return mb << 20
}
func (a *App) backup(c *gin.Context) {
	u := current(c)
	photos, err := a.list(u.SpaceID)
	if err != nil {
		fail(c, 500, "读取照片失败")
		return
	}
	memories, err := a.memories(u.SpaceID)
	if err != nil {
		fail(c, 500, "读取回忆失败")
		return
	}
	for i := range memories {
		references := []Photo{}
		for _, p := range memories[i].Photos {
			references = append(references, Photo{ID: p.ID})
		}
		memories[i].Photos = references
	}
	features := map[string]json.RawMessage{}
	rows, err := a.db.Query(`SELECT key,data FROM features WHERE space_id=?`, u.SpaceID)
	if err != nil {
		fail(c, 500, "读取设置失败")
		return
	}
	for rows.Next() {
		var key, value string
		if rows.Scan(&key, &value) != nil {
			rows.Close()
			fail(c, 500, "读取设置失败")
			return
		}
		features[key] = json.RawMessage(value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		fail(c, 500, "读取设置失败")
		return
	}
	manifest, err := json.Marshal(Manifest{Version: 2, ExportedAt: time.Now().UTC().Format(time.RFC3339), Photos: photos, Memories: memories, Features: features})
	if err != nil || len(manifest) > 8<<20 || len(photos) > 5000 {
		fail(c, 400, "图库清单过大，请使用完整数据目录备份")
		return
	}
	total := int64(len(manifest))
	for _, p := range photos {
		info, err := os.Stat(filepath.Join(a.dir, "photos", p.Filename))
		if err != nil {
			fail(c, 500, "照片文件不完整，无法备份")
			return
		}
		total += info.Size()
	}
	if total > backupLimit() {
		fail(c, 400, "图库超过当前备份恢复上限，请调整 BACKUP_MAX_MB 或备份整个数据目录")
		return
	}
	f, err := os.CreateTemp(a.dir, "backup-*.zip")
	if err != nil {
		fail(c, 500, "创建备份失败")
		return
	}
	defer os.Remove(f.Name())
	defer f.Close()
	z := zip.NewWriter(f)
	entry, err := z.Create("manifest.json")
	if err == nil {
		_, err = entry.Write(manifest)
	}
	for _, p := range photos {
		if err != nil {
			break
		}
		var input *os.File
		input, err = os.Open(filepath.Join(a.dir, "photos", p.Filename))
		if err != nil {
			break
		}
		entry, err = z.Create("photos/" + p.Filename)
		if err == nil {
			_, err = io.Copy(entry, input)
		}
		input.Close()
	}
	if closeErr := z.Close(); err != nil || closeErr != nil {
		fail(c, 500, "备份失败")
		return
	}
	info, err := f.Stat()
	if err != nil || info.Size() > backupLimit() {
		fail(c, 400, "压缩文件超过恢复上限，请调整 BACKUP_MAX_MB")
		return
	}
	c.Header("Content-Type", "application/zip")
	c.FileAttachment(f.Name(), "footprint-"+time.Now().Format("2006-01-02")+".zip")
}
func (a *App) restore(c *gin.Context) {
	limit := backupLimit()
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, limit+(10<<20))
	if err := c.Request.ParseMultipartForm(8 << 20); err != nil {
		fail(c, 400, "请选择未超过备份上限的 ZIP 文件")
		return
	}
	defer c.Request.MultipartForm.RemoveAll()
	header, err := c.FormFile("backup")
	if err != nil || header.Size > limit {
		fail(c, 400, "备份文件超过上限")
		return
	}
	file, err := header.Open()
	if err != nil {
		fail(c, 400, "无法读取备份")
		return
	}
	defer file.Close()
	z, err := zip.NewReader(file, header.Size)
	if err != nil || len(z.File) > 5001 {
		fail(c, 400, "备份格式无效或照片过多")
		return
	}
	entries := map[string]*zip.File{}
	var total uint64
	for _, e := range z.File {
		if _, ok := entries[e.Name]; ok {
			fail(c, 400, "备份包含重复文件")
			return
		}
		if e.UncompressedSize64 > uint64(limit) || total > uint64(limit)-e.UncompressedSize64 {
			fail(c, 400, "备份解压后超过上限")
			return
		}
		total += e.UncompressedSize64
		entries[e.Name] = e
	}
	var manifest Manifest
	if entries["records.json"] != nil {
		manifest, err = legacyManifest(entries)
	} else {
		err = readZipJSON(entries["manifest.json"], &manifest)
	}
	if err != nil {
		fail(c, 400, "备份清单无效："+err.Error())
		return
	}
	if (manifest.Version != 1 && manifest.Version != 2) || len(manifest.Photos) > 5000 || len(manifest.Memories) > 5000 {
		fail(c, 400, "备份版本或清单无效")
		return
	}
	var expanded uint64
	for _, photo := range manifest.Photos {
		entry := entries["photos/"+photo.Filename]
		if entry == nil {
			fail(c, 400, "备份照片缺失")
			return
		}
		if entry.UncompressedSize64 > uint64(limit) || expanded > uint64(limit)-entry.UncompressedSize64 {
			fail(c, 400, "回忆所引用的照片总量超过恢复上限")
			return
		}
		expanded += entry.UncompressedSize64
	}
	u := current(c)
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "无法恢复备份")
		return
	}
	defer tx.Rollback()
	written := []string{}
	success := false
	defer func() {
		if !success {
			for _, path := range written {
				os.Remove(path)
			}
		}
	}()
	added := 0
	mapping := map[string]string{}
	for _, p := range manifest.Photos {
		if p.ID == "" || mapping[p.ID] != "" || filepath.Base(p.Filename) != p.Filename || strings.ContainsAny(p.Filename, "\\/") {
			fail(c, 400, "备份照片条目无效")
			return
		}
		if err = validate(p); err != nil {
			fail(c, 400, "备份照片信息无效")
			return
		}
		originalID := p.ID
		newID := digest(u.SpaceID + ":" + p.ID)
		var existingID string
		err = tx.QueryRow(`SELECT id FROM photos WHERE space_id=? AND id IN (?,?) LIMIT 1`, u.SpaceID, p.ID, newID).Scan(&existingID)
		if err == nil {
			mapping[originalID] = existingID
			continue
		}
		entry, ok := entries["photos/"+p.Filename]
		if !ok || entry.UncompressedSize64 > 20<<20 {
			fail(c, 400, "备份照片缺失或单张超过 20MB")
			return
		}
		r, err := entry.Open()
		if err != nil {
			fail(c, 400, "无法读取备份照片")
			return
		}
		b, err := io.ReadAll(io.LimitReader(r, (20<<20)+1))
		r.Close()
		if err != nil || len(b) > 20<<20 {
			fail(c, 400, "备份照片损坏")
			return
		}
		ext, err := imageExtension(b)
		if err != nil {
			fail(c, 400, err.Error())
			return
		}
		p.ID = newID
		p.Filename = p.ID + ext
		p.OwnerID = u.ID
		path := filepath.Join(a.dir, "photos", p.Filename)
		out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			fail(c, 500, "备份文件冲突或磁盘不可写")
			return
		}
		written = append(written, path)
		_, err = out.Write(b)
		closeErr := out.Close()
		if err != nil || closeErr != nil {
			fail(c, 500, "写入照片失败")
			return
		}
		if insertPhoto(tx, u.SpaceID, p) != nil {
			fail(c, 500, "保存恢复记录失败")
			return
		}
		mapping[originalID] = newID
		added++
	}
	seenMemories := map[string]bool{}
	seenReferences := map[string]bool{}
	for _, memory := range manifest.Memories {
		if memory.ID == "" || seenMemories[memory.ID] {
			fail(c, 400, "回忆清单包含重复条目")
			return
		}
		seenMemories[memory.ID] = true
		if err = validateMemory(&memory); err != nil {
			fail(c, 400, "备份回忆信息无效")
			return
		}
		ids := []string{}
		for _, p := range memory.Photos {
			mapped := mapping[p.ID]
			if mapped == "" || seenReferences[p.ID] {
				fail(c, 400, "回忆引用了缺失或重复的照片")
				return
			}
			seenReferences[p.ID] = true
			ids = append(ids, mapped)
		}
		if len(ids) > 200 {
			fail(c, 400, "回忆照片数量无效")
			return
		}
		restoredID := digest(u.SpaceID + ":memory:" + memory.ID)
		var existing int
		tx.QueryRow(`SELECT count(*) FROM memories WHERE space_id=? AND id IN (?,?)`, u.SpaceID, memory.ID, restoredID).Scan(&existing)
		if existing > 0 {
			continue
		}
		memory.ID = restoredID
		memory.OwnerID = u.ID
		memory.CoverID = mapping[memory.CoverID]
		if memory.CoverID == "" && len(ids) > 0 {
			memory.CoverID = ids[0]
		}
		if insertMemory(tx, u.SpaceID, memory) != nil {
			fail(c, 500, "恢复回忆失败")
			return
		}
		if len(ids) > 0 && attachPhotos(tx, u.SpaceID, restoredID, ids) != nil {
			fail(c, 400, "恢复回忆照片关联失败")
			return
		}
	}
	for key, value := range manifest.Features {
		if len(key) > 100 || len(value) > 1<<20 || !json.Valid(value) {
			fail(c, 400, "备份扩展数据无效")
			return
		}
		if _, err = tx.Exec(`INSERT OR IGNORE INTO features VALUES(?,?,?,?)`, u.SpaceID, key, string(value), time.Now().UTC().Format(time.RFC3339)); err != nil {
			fail(c, 500, "恢复扩展数据失败")
			return
		}
	}
	if seedPhotoMemories(tx) != nil || tx.Commit() != nil {
		fail(c, 500, "恢复失败")
		return
	}
	success = true
	c.JSON(200, gin.H{"added": added, "skipped": len(manifest.Photos) - added})
}
