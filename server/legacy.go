package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"path"
	"strings"
	"time"
)

func readZipJSON(file *zip.File, target any) error {
	if file == nil || file.UncompressedSize64 > 8<<20 {
		return fmt.Errorf("缺少有效 JSON 清单")
	}
	r, err := file.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	return json.NewDecoder(io.LimitReader(r, 8<<20)).Decode(target)
}
func legacyManifest(entries map[string]*zip.File) (Manifest, error) {
	var records []struct {
		ID          string   `json:"id"`
		Mode        string   `json:"mode"`
		Title       string   `json:"title"`
		Description string   `json:"description"`
		Place       string   `json:"location"`
		Date        string   `json:"date"`
		Latitude    *float64 `json:"latitude"`
		Longitude   *float64 `json:"longitude"`
		Tags        []string `json:"tags"`
		Rating      int      `json:"rating"`
		Price       float64  `json:"price"`
		Images      []string `json:"images"`
	}
	if err := readZipJSON(entries["records.json"], &records); err != nil {
		return Manifest{}, err
	}
	if len(records) > 5000 {
		return Manifest{}, fmt.Errorf("旧版记录过多")
	}
	manifest := Manifest{Version: 2, ExportedAt: time.Now().UTC().Format(time.RFC3339), Photos: []Photo{}, Memories: []Memory{}, Features: map[string]json.RawMessage{}}
	seen := map[string]bool{}
	for i, record := range records {
		if record.ID == "" {
			record.ID = fmt.Sprintf("record-%d", i)
		}
		if seen[record.ID] {
			return Manifest{}, fmt.Errorf("旧版记录 ID 重复")
		}
		seen[record.ID] = true
		date := record.Date
		if len(date) > 10 {
			date = date[:10]
		}
		if date == "" {
			date = time.Now().Format("2006-01-02")
		}
		mode := record.Mode
		if mode == "" {
			mode = "travel"
		}
		m := Memory{ID: "legacy-" + record.ID, Title: record.Title, Description: record.Description, Mode: mode, Date: date, Place: record.Place, Latitude: record.Latitude, Longitude: record.Longitude, Tags: record.Tags, Rating: record.Rating, Price: record.Price, CreatedAt: manifest.ExportedAt, UpdatedAt: manifest.ExportedAt, Photos: []Photo{}}
		for j, imageURL := range record.Images {
			parsed, err := url.Parse(imageURL)
			if err != nil || strings.HasPrefix(imageURL, "data:") {
				return Manifest{}, fmt.Errorf("旧版图片格式无法直接迁移，请先将图片导出为文件")
			}
			filename := path.Base(parsed.Path)
			entry := entries["media/"+filename]
			if entry == nil {
				return Manifest{}, fmt.Errorf("旧备份缺少照片 %s；云端直链图片需要先下载并放入 media 目录", filename)
			}
			id := digest(fmt.Sprintf("legacy:%s:%d:%s", record.ID, j, imageURL))
			newName := id + path.Ext(filename)
			entries["photos/"+newName] = entry
			p := Photo{ID: id, Filename: newName, Title: record.Title, TakenAt: date, Note: record.Description, Place: record.Place, Latitude: record.Latitude, Longitude: record.Longitude, CreatedAt: manifest.ExportedAt}
			manifest.Photos = append(manifest.Photos, p)
			m.Photos = append(m.Photos, Photo{ID: id})
		}
		if len(m.Photos) > 0 {
			m.CoverID = m.Photos[0].ID
		}
		if err := validateMemory(&m); err != nil {
			return Manifest{}, err
		}
		manifest.Memories = append(manifest.Memories, m)
	}
	if entries["features.json"] != nil {
		if err := readZipJSON(entries["features.json"], &manifest.Features); err != nil {
			return Manifest{}, err
		}
	}
	for key, value := range manifest.Features {
		var items []map[string]any
		if json.Unmarshal(value, &items) != nil {
			continue
		}
		normalized := []map[string]any{}
		for index, item := range items {
			text := func(keys ...string) string {
				for _, key := range keys {
					if value, ok := item[key].(string); ok && value != "" {
						return value
					}
				}
				return ""
			}
			id := text("id")
			if id == "" {
				id = fmt.Sprintf("legacy-%s-%d", key, index)
			}
			title := text("title", "name", "text")
			if title == "" {
				title = "旧版收藏"
			}
			note := text("note", "content", "description", "detail")
			amount := item["amount"]
			if amount == nil {
				amount = item["price"]
			}
			if amount == nil {
				amount = 0
			}
			done, _ := item["done"].(bool)
			if completed, ok := item["completed"].(bool); ok {
				done = completed
			}
			normalized = append(normalized, map[string]any{"id": id, "title": title, "note": note, "date": text("date", "unlockDate", "targetDate"), "amount": amount, "done": done})
		}
		encoded, _ := json.Marshal(normalized)
		manifest.Features[key] = encoded
	}
	if tasks, ok := manifest.Features["couple_tasks"]; ok {
		var wishes, todos []map[string]any
		json.Unmarshal(manifest.Features["wishes"], &wishes)
		json.Unmarshal(tasks, &todos)
		encoded, _ := json.Marshal(append(wishes, todos...))
		manifest.Features["wishes"] = encoded
	}
	return manifest, nil
}
