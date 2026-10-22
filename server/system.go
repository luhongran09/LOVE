package main

import (
	"github.com/gin-gonic/gin"
	"net/http"
	"os"
	"time"
)

func (a *App) systemRoutes(api *gin.RouterGroup) {
	api.GET("/system", func(c *gin.Context) {
		c.JSON(200, gin.H{"backup_max_mb": backupLimit() >> 20, "storage": "local", "database": "sqlite", "model_configured": os.Getenv("LLM_API_KEY") != "", "formats": []string{"jpeg", "png", "gif", "webp"}})
	})
	api.GET("/revision", func(c *gin.Context) {
		var revision int64
		if a.db.QueryRow(`SELECT COALESCE(revision,0) FROM space_revisions WHERE space_id=?`, current(c).SpaceID).Scan(&revision) != nil {
			revision = 0
		}
		c.JSON(200, gin.H{"revision": revision})
	})
	api.POST("/auth/unbind", func(c *gin.Context) { a.unbind(c) })
}
func (a *App) unbind(c *gin.Context) {
	u := current(c)
	var count int
	if a.db.QueryRow(`SELECT count(*) FROM users WHERE space_id=?`, u.SpaceID).Scan(&count) != nil {
		fail(c, 500, "读取空间失败")
		return
	}
	if count < 2 {
		fail(c, 400, "当前空间尚未配对")
		return
	}
	var before int64
	a.db.QueryRow(`SELECT revision FROM space_revisions WHERE space_id=?`, u.SpaceID).Scan(&before)
	items, err := a.memories(u.SpaceID)
	if err != nil {
		fail(c, 500, "读取回忆失败")
		return
	}
	tx, err := a.db.Begin()
	if err != nil {
		fail(c, 500, "解除配对失败")
		return
	}
	defer tx.Rollback()
	var revision int64
	tx.QueryRow(`SELECT revision FROM space_revisions WHERE space_id=?`, u.SpaceID).Scan(&revision)
	if revision != before {
		fail(c, 409, "空间刚有更新，请重新确认解除配对")
		return
	}
	space := token()
	if _, err = tx.Exec(`INSERT INTO spaces(id) VALUES(?)`, space); err != nil {
		fail(c, 500, "创建独立空间失败")
		return
	}
	for _, memory := range items {
		if len(memory.Photos) == 0 {
			if memory.OwnerID == u.ID {
				if _, err = tx.Exec(`UPDATE memories SET space_id=? WHERE id=?`, space, memory.ID); err != nil {
					fail(c, 500, "分离文字回忆失败")
					return
				}
			}
			continue
		}
		ids := []string{}
		for _, p := range memory.Photos {
			if p.OwnerID == u.ID {
				ids = append(ids, p.ID)
			}
		}
		if len(ids) == 0 {
			continue
		}
		originalID := memory.ID
		memory.ID = token()
		memory.OwnerID = u.ID
		coverValid := false
		for _, id := range ids {
			if id == memory.CoverID {
				coverValid = true
			}
		}
		if !coverValid {
			memory.CoverID = ids[0]
		}
		memory.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if insertMemory(tx, space, memory) != nil {
			fail(c, 500, "分离回忆失败")
			return
		}
		for position, id := range ids {
			if _, err = tx.Exec(`UPDATE photos SET space_id=? WHERE id=? AND owner_id=?`, space, id, u.ID); err != nil {
				fail(c, 500, "分离照片失败")
				return
			}
			if _, err = tx.Exec(`UPDATE memory_photos SET memory_id=?,position=? WHERE photo_id=?`, memory.ID, position, id); err != nil {
				fail(c, 500, "分离照片关联失败")
				return
			}
		}
		if _, err = tx.Exec(`DELETE FROM memories WHERE id=? AND NOT EXISTS(SELECT 1 FROM memory_photos WHERE memory_id=memories.id)`, originalID); err != nil {
			fail(c, 500, "整理回忆失败")
			return
		}
	}
	if _, err = tx.Exec(`INSERT INTO features SELECT ?,key,data,updated_at FROM features WHERE space_id=?`, space, u.SpaceID); err != nil {
		fail(c, 500, "复制共同记录失败")
		return
	}
	if _, err = tx.Exec(`UPDATE users SET space_id=? WHERE id=?`, space, u.ID); err != nil {
		fail(c, 500, "更新空间失败")
		return
	}
	if _, err = tx.Exec(`UPDATE spaces SET invite_hash=NULL,invite_expires=NULL WHERE id=?`, u.SpaceID); err != nil {
		fail(c, 500, "取消旧邀请失败")
		return
	}
	if _, err = tx.Exec(`INSERT INTO space_revisions VALUES(?,1) ON CONFLICT(space_id) DO UPDATE SET revision=revision+1`, u.SpaceID); err != nil {
		fail(c, 500, "更新空间状态失败")
		return
	}
	if tx.Commit() != nil {
		fail(c, 500, "解除配对失败")
		return
	}
	u.SpaceID = space
	c.JSON(http.StatusOK, u)
}
