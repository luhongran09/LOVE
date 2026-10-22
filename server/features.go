package main

import (
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"time"
)

var allowedFeatures = map[string]bool{"anniversaries": true, "love_notes": true, "date_plans": true, "wishes": true, "love_capsules": true, "packing_list": true, "ingredients": true, "travel_plans": true, "expenses": true, "food_choices": true, "space_settings": true}

func (a *App) featureRoutes(api *gin.RouterGroup) {
	api.GET("/features/:key", func(c *gin.Context) {
		key := c.Param("key")
		if !allowedFeatures[key] {
			fail(c, 404, "功能不存在")
			return
		}
		var data, revision string
		err := a.db.QueryRow(`SELECT data,updated_at FROM features WHERE space_id=? AND key=?`, current(c).SpaceID, key).Scan(&data, &revision)
		if err == sql.ErrNoRows {
			data = "[]"
			revision = ""
		} else if err != nil {
			fail(c, 500, "读取失败")
			return
		}
		c.JSON(200, gin.H{"data": json.RawMessage(data), "revision": revision})
	})
	api.PUT("/features/:key", func(c *gin.Context) {
		key := c.Param("key")
		if !allowedFeatures[key] {
			fail(c, 404, "功能不存在")
			return
		}
		var input struct {
			Data     json.RawMessage `json:"data"`
			Revision string          `json:"revision"`
		}
		if c.ShouldBindJSON(&input) != nil || !json.Valid(input.Data) || len(input.Data) > 900000 {
			fail(c, 400, "数据无效或过大")
			return
		}
		tx, err := a.db.Begin()
		if err != nil {
			fail(c, 500, "保存失败")
			return
		}
		defer tx.Rollback()
		var revision string
		err = tx.QueryRow(`SELECT updated_at FROM features WHERE space_id=? AND key=?`, current(c).SpaceID, key).Scan(&revision)
		if err != nil && err != sql.ErrNoRows {
			fail(c, 500, "保存失败")
			return
		}
		if revision != input.Revision {
			fail(c, 409, "另一半刚更新了这里，请刷新后重新修改")
			return
		}
		next := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err = tx.Exec(`INSERT INTO features VALUES(?,?,?,?) ON CONFLICT(space_id,key) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at`, current(c).SpaceID, key, string(input.Data), next); err != nil || tx.Commit() != nil {
			fail(c, 500, "保存失败")
			return
		}
		c.JSON(200, gin.H{"revision": next})
	})
}
