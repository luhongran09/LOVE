package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

type pairSpace struct {
	Name     string `json:"name"`
	Photos   int    `json:"photos"`
	Memories int    `json:"memories"`
	Revision int64  `json:"revision"`
	ID       string `json:"-"`
}
type pairPreview struct {
	Source pairSpace `json:"source"`
	Target pairSpace `json:"target"`
}
type pairingError struct {
	code    int
	message string
}

func (e pairingError) Error() string { return e.message }

func pairingSpaces(tx *sql.Tx, u User, invite string) (pairPreview, error) {
	var result pairPreview
	var currentSpace string
	if err := tx.QueryRow(`SELECT space_id FROM users WHERE id=?`, u.ID).Scan(&currentSpace); err != nil {
		return result, err
	}
	if currentSpace != u.SpaceID {
		return result, pairingError{409, "你的空间已改变，请刷新后重试"}
	}
	if err := tx.QueryRow(`SELECT id FROM spaces WHERE invite_hash=? AND invite_expires>?`, digest(invite), time.Now().Unix()).Scan(&result.Target.ID); err == sql.ErrNoRows {
		return result, pairingError{403, "邀请码无效、已使用或已过期"}
	} else if err != nil {
		return result, err
	}
	result.Source.ID = u.SpaceID
	if result.Target.ID == u.SpaceID {
		return result, pairingError{400, "这是你当前空间的邀请码，无需重复加入"}
	}
	for _, space := range []*pairSpace{&result.Source, &result.Target} {
		var count int
		if err := tx.QueryRow(`SELECT count(*) FROM users WHERE space_id=?`, space.ID).Scan(&count); err != nil {
			return result, err
		}
		if count != 1 {
			return result, pairingError{409, "双方都需要处于独立空间，请先解除已有配对"}
		}
		if err := tx.QueryRow(`SELECT name FROM users WHERE space_id=?`, space.ID).Scan(&space.Name); err != nil {
			return result, err
		}
		if err := tx.QueryRow(`SELECT count(*) FROM photos WHERE space_id=?`, space.ID).Scan(&space.Photos); err != nil {
			return result, err
		}
		if err := tx.QueryRow(`SELECT count(*) FROM memories WHERE space_id=?`, space.ID).Scan(&space.Memories); err != nil {
			return result, err
		}
		if err := tx.QueryRow(`SELECT COALESCE((SELECT revision FROM space_revisions WHERE space_id=?),0)`, space.ID).Scan(&space.Revision); err != nil {
			return result, err
		}
	}
	return result, nil
}
func pairFailure(c *gin.Context, err error) {
	var known pairingError
	if errors.As(err, &known) {
		fail(c, known.code, known.message)
	} else {
		fail(c, 500, "配对失败，数据未作更改")
	}
}

// Keep both lists, including edits to entries that share an ID after an earlier
// unbind. Only byte-equivalent JSON values with the same ID are deduplicated.
func mergePairFeature(key, source, target string) (string, error) {
	var src, dst any
	if json.Unmarshal([]byte(source), &src) != nil || json.Unmarshal([]byte(target), &dst) != nil {
		return "", pairingError{409, "共同清单格式无效，请先导出备份并整理"}
	}
	if key == "space_settings" {
		settings := map[string]any{}
		if object, ok := src.(map[string]any); ok {
			for k, v := range object {
				settings[k] = v
			}
		}
		if object, ok := dst.(map[string]any); ok {
			for k, v := range object {
				settings[k] = v
			}
		}
		data, _ := json.Marshal(settings)
		return string(data), nil
	}
	a, aOK := src.([]any)
	b, bOK := dst.([]any)
	if !aOK || !bOK {
		return "", pairingError{409, "存在无法自动合并的清单，请先在双方空间导出并整理"}
	}
	result := append([]any{}, b...)
	used := map[string]any{}
	for _, item := range b {
		if obj, ok := item.(map[string]any); ok {
			if id, ok := obj["id"].(string); ok && id != "" {
				used[id] = obj
			}
		}
	}
	for _, item := range a {
		if obj, ok := item.(map[string]any); ok {
			if id, ok := obj["id"].(string); ok && id != "" {
				if old, exists := used[id]; exists {
					if reflect.DeepEqual(old, obj) {
						continue
					}
					obj["id"] = token()
				}
			} else {
				obj["id"] = token()
			}
			used[obj["id"].(string)] = obj
		}
		result = append(result, item)
	}
	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	if len(data) > 900000 {
		return "", pairingError{409, "合并后的清单超过大小上限，请先导出备份并整理"}
	}
	return string(data), nil
}
func mergePairFeatures(tx *sql.Tx, source, target string) error {
	rows, err := tx.Query(`SELECT key,data FROM features WHERE space_id=?`, source)
	if err != nil {
		return err
	}
	type feature struct{ key, data string }
	items := []feature{}
	for rows.Next() {
		var f feature
		if err = rows.Scan(&f.key, &f.data); err != nil {
			rows.Close()
			return err
		}
		items = append(items, f)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, f := range items {
		var other string
		err = tx.QueryRow(`SELECT data FROM features WHERE space_id=? AND key=?`, target, f.key).Scan(&other)
		data := f.data
		if err == nil {
			data, err = mergePairFeature(f.key, f.data, other)
		} else if err == sql.ErrNoRows {
			err = nil
		}
		if err != nil {
			return err
		}
		if _, err = tx.Exec(`INSERT INTO features VALUES(?,?,?,?) ON CONFLICT(space_id,key) DO UPDATE SET data=excluded.data,updated_at=excluded.updated_at`, target, f.key, data, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	_, err = tx.Exec(`DELETE FROM features WHERE space_id=?`, source)
	return err
}
func (a *App) pairingRoutes(api *gin.RouterGroup) {
	api.POST("/auth/pair/preview", a.rateLimit, func(c *gin.Context) {
		var input struct {
			Invite string `json:"invite"`
		}
		if c.ShouldBindJSON(&input) != nil || len(strings.TrimSpace(input.Invite)) != 64 {
			fail(c, 400, "请输入完整的伴侣邀请码")
			return
		}
		tx, err := a.db.Begin()
		if err != nil {
			pairFailure(c, err)
			return
		}
		defer tx.Rollback()
		result, err := pairingSpaces(tx, current(c), strings.TrimSpace(input.Invite))
		if err != nil {
			pairFailure(c, err)
			return
		}
		c.JSON(200, result)
	})
	api.POST("/auth/pair", a.rateLimit, func(c *gin.Context) {
		var input struct {
			Invite         string `json:"invite"`
			Password       string `json:"password"`
			SourceRevision *int64 `json:"source_revision"`
			TargetRevision *int64 `json:"target_revision"`
		}
		if c.ShouldBindJSON(&input) != nil || len(strings.TrimSpace(input.Invite)) != 64 || input.SourceRevision == nil || input.TargetRevision == nil {
			fail(c, 400, "请先预览配对信息并确认")
			return
		}
		u := current(c)
		var hash string
		if err := a.db.QueryRow(`SELECT password FROM users WHERE id=?`, u.ID).Scan(&hash); err != nil {
			pairFailure(c, err)
			return
		}
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(input.Password)) != nil {
			fail(c, 403, "当前账号密码不正确")
			return
		}
		tx, err := a.db.Begin()
		if err != nil {
			pairFailure(c, err)
			return
		}
		defer tx.Rollback()
		spaces, err := pairingSpaces(tx, u, strings.TrimSpace(input.Invite))
		if err != nil {
			pairFailure(c, err)
			return
		}
		if spaces.Source.Revision != *input.SourceRevision || spaces.Target.Revision != *input.TargetRevision {
			fail(c, 409, "双方空间刚有更新，请重新预览后确认配对")
			return
		}
		if err = mergePairFeatures(tx, u.SpaceID, spaces.Target.ID); err != nil {
			pairFailure(c, err)
			return
		}
		for _, table := range []string{"photos", "memories", "users"} {
			if _, err = tx.Exec(`UPDATE `+table+` SET space_id=? WHERE space_id=?`, spaces.Target.ID, u.SpaceID); err != nil {
				pairFailure(c, err)
				return
			}
		}
		if _, err = tx.Exec(`UPDATE spaces SET invite_hash=NULL,invite_expires=NULL WHERE id IN (?,?)`, u.SpaceID, spaces.Target.ID); err != nil {
			pairFailure(c, err)
			return
		}
		if _, err = tx.Exec(`INSERT INTO space_revisions VALUES(?,1) ON CONFLICT(space_id) DO UPDATE SET revision=revision+1`, u.SpaceID); err != nil {
			pairFailure(c, err)
			return
		}
		if err = tx.Commit(); err != nil {
			pairFailure(c, err)
			return
		}
		u.SpaceID = spaces.Target.ID
		c.JSON(200, u)
	})
}
