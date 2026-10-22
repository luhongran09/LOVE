package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func uploadMany(t *testing.T, h http.Handler, cookie *http.Cookie, n int, memoryID string) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	for i := 0; i < n; i++ {
		file, _ := writer.CreateFormFile("photos", "moment.png")
		file.Write(samplePNG(t))
	}
	writer.WriteField("title", "第一次一起看海")
	writer.WriteField("mode", "love")
	writer.WriteField("taken_at", "2025-05-20")
	writer.WriteField("memory_id", memoryID)
	writer.Close()
	r := httptest.NewRequest("POST", "/api/photos/upload", &b)
	r.Header.Set("Content-Type", writer.FormDataContentType())
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func memoryList(t *testing.T, h http.Handler, cookie *http.Cookie) []Memory {
	t.Helper()
	w := request(t, h, "GET", "/api/memories", nil, cookie)
	expect(t, w, 200)
	var result struct {
		Memories []Memory `json:"memories"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Memories
}
func TestMemoryGroupingOrderingAndBackup(t *testing.T) {
	a, h := testApp(t)
	cookie := registerTest(t, h, "多图测试", "")
	expect(t, uploadMany(t, h, cookie, 4, ""), 201)
	items := memoryList(t, h, cookie)
	if len(items) != 1 || len(items[0].Photos) != 4 || items[0].Mode != "love" {
		t.Fatalf("not grouped: %+v", items)
	}
	m := items[0]
	ids := []string{m.Photos[3].ID, m.Photos[2].ID, m.Photos[1].ID, m.Photos[0].ID}
	m.CoverID = ids[1]
	m.City = "厦门"
	m.Tags = []string{"第一次", "海边"}
	m.Rating = 5
	w := request(t, h, "PUT", "/api/memories/"+m.ID, memoryInput{Memory: m, PhotoIDs: ids}, cookie)
	expect(t, w, 200)
	items = memoryList(t, h, cookie)
	if items[0].Photos[0].ID != ids[0] || items[0].CoverID != ids[1] || items[0].City != "厦门" {
		t.Fatal("order/cover metadata lost")
	}
	m.UpdatedAt = items[0].UpdatedAt
	bad := memoryInput{Memory: m, PhotoIDs: []string{ids[0], "not-a-photo"}}
	expect(t, request(t, h, "PUT", "/api/memories/"+m.ID, bad, cookie), 400)
	if len(memoryList(t, h, cookie)[0].Photos) != 4 {
		t.Fatal("failed update modified association")
	}
	expect(t, uploadMany(t, h, cookie, 2, m.ID), 201)
	if len(memoryList(t, h, cookie)[0].Photos) != 6 {
		t.Fatal("append did not retain original photos")
	}
	// Save a v2 grouped backup, remove the memory and restore its ordering/cover/types.
	w = request(t, h, "GET", "/api/backup", nil, cookie)
	expect(t, w, 200)
	backup := append([]byte(nil), w.Body.Bytes()...)
	z, err := zip.NewReader(bytes.NewReader(backup), int64(len(backup)))
	if err != nil {
		t.Fatal(err)
	}
	if len(z.File) != 7 {
		t.Fatal("backup omitted images")
	}
	expect(t, request(t, h, "DELETE", "/api/memories/"+m.ID, nil, cookie), 204)
	if len(memoryList(t, h, cookie)) != 0 {
		t.Fatal("delete left memory")
	}
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "v2.zip", backup, cookie), 200)
	restored := memoryList(t, h, cookie)
	if len(restored) != 1 || len(restored[0].Photos) != 6 || restored[0].Mode != "love" || restored[0].Tags[0] != "第一次" {
		t.Fatal("restore lost grouping", restored)
	}
	if restored[0].Photos[0].ID != digest(currentSpace(t, a, cookie)+":"+ids[0]) {
		t.Fatal("restore lost order")
	}
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "v2.zip", backup, cookie), 200)
	if len(memoryList(t, h, cookie)) != 1 {
		t.Fatal("repeat restore duplicated memory")
	}
	if err = migrateMemories(a.db); err != nil {
		t.Fatal(err)
	}
	if len(memoryList(t, h, cookie)) != 1 {
		t.Fatal("restart migration split grouped photos")
	}
}
func currentSpace(t *testing.T, a *App, cookie *http.Cookie) string {
	t.Helper()
	var space string
	err := a.db.QueryRow(`SELECT u.space_id FROM users u JOIN sessions s ON s.user_id=u.id WHERE s.hash=?`, digest(cookie.Value)).Scan(&space)
	if err != nil {
		t.Fatal(err)
	}
	return space
}
func TestMemoryOwnershipAndMerge(t *testing.T) {
	a, h := testApp(t)
	cookie := registerTest(t, h, "合并测试", "")
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	items := memoryList(t, h, cookie)
	first := items[0]
	ids := []string{items[0].Photos[0].ID, items[1].Photos[0].ID}
	expect(t, request(t, h, "POST", "/api/memories", memoryInput{Memory: first, PhotoIDs: ids}, cookie), 200)
	if len(memoryList(t, h, cookie)) != 1 {
		t.Fatal("merge left empty memories")
	}
	a.db.Exec(`INSERT INTO spaces(id) VALUES('other-memory-space')`)
	a.db.Exec(`INSERT INTO users VALUES('foreign-member','foreign-member','unused','other-memory-space')`)
	a.db.Exec(`INSERT INTO sessions VALUES(?,'foreign-member',9999999999)`, digest("foreign-session"))
	other := &http.Cookie{Name: "footprint_session", Value: "foreign-session"}
	m := memoryList(t, h, cookie)[0]
	expect(t, request(t, h, "PUT", "/api/memories/"+m.ID, memoryInput{Memory: m, PhotoIDs: ids}, other), 404)
	expect(t, request(t, h, "DELETE", "/api/memories/"+m.ID, nil, other), 404)
	expect(t, request(t, h, "POST", "/api/memories", memoryInput{Memory: m, PhotoIDs: ids}, other), 400)
	if len(memoryList(t, h, other)) != 0 {
		t.Fatal("cross-space merge created records")
	}
}
func TestSharedFeaturesConflictProtection(t *testing.T) {
	_, h := testApp(t)
	cookie := registerTest(t, h, "工具测试", "")
	w := request(t, h, "PUT", "/api/features/wishes", map[string]any{"data": []any{map[string]string{"title": "一起看海"}}, "revision": ""}, cookie)
	expect(t, w, 200)
	expect(t, request(t, h, "PUT", "/api/features/wishes", map[string]any{"data": []any{}, "revision": ""}, cookie), 409)
	expect(t, request(t, h, "GET", "/api/features/not-allowed", nil, cookie), 404)
}

func TestLegacyFullZIPAndEmptyRecord(t *testing.T) {
	_, h := testApp(t)
	cookie := registerTest(t, h, "旧版迁移", "")
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	records, _ := z.Create("records.json")
	json.NewEncoder(records).Encode([]map[string]any{{"id": "old-trip", "mode": "food", "title": "旧版聚餐", "description": "原有备注", "date": "2024-05-20", "location": "杭州", "rating": 5, "images": []string{"/uploads/a.png", "/uploads/b.png"}}, {"id": "text-only", "mode": "love", "title": "没有照片的约定", "date": "2024-05-21", "images": []string{}}})
	manifest, _ := z.Create("manifest.json")
	manifest.Write([]byte(`{"version":"1.0","type":"footprint_disaster_recovery_backup"}`))
	for _, name := range []string{"a.png", "b.png"} {
		f, _ := z.Create("media/" + name)
		f.Write(samplePNG(t))
	}
	z.Close()
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "legacy.zip", b.Bytes(), cookie), 200)
	items := memoryList(t, h, cookie)
	if len(items) != 2 {
		t.Fatal("legacy text or photos lost", items)
	}
	var food Memory
	for _, m := range items {
		if m.Mode == "food" {
			food = m
		}
	}
	if len(food.Photos) != 2 || food.Rating != 5 || food.Description != "原有备注" {
		t.Fatal("legacy record not preserved")
	}
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "legacy.zip", b.Bytes(), cookie), 200)
	if len(memoryList(t, h, cookie)) != 2 {
		t.Fatal("legacy import not idempotent")
	}
}
func TestUnbindSeparatesOwnedPhotos(t *testing.T) {
	_, h := testApp(t)
	first := registerTest(t, h, "分离甲", "")
	w := request(t, h, "POST", "/api/invite", nil, first)
	var invitation struct {
		Invite string `json:"invite"`
	}
	json.Unmarshal(w.Body.Bytes(), &invitation)
	second := registerTest(t, h, "分离乙", invitation.Invite)
	expect(t, uploadMany(t, h, first, 1, ""), 201)
	m := memoryList(t, h, first)[0]
	firstPhoto := m.Photos[0].ID
	expect(t, uploadMany(t, h, second, 1, m.ID), 201)
	items := memoryList(t, h, first)
	secondPhoto := items[0].Photos[1].ID
	expect(t, request(t, h, "POST", "/api/auth/unbind", map[string]any{}, first), 200)
	own := memoryList(t, h, first)
	partner := memoryList(t, h, second)
	if len(own) != 1 || len(own[0].Photos) != 1 || len(partner) != 1 || len(partner[0].Photos) != 1 {
		t.Fatal("shared memory was not safely split")
	}
	expect(t, request(t, h, "GET", "/api/photos/"+secondPhoto+"/file", nil, first), 404)
	expect(t, request(t, h, "GET", "/api/photos/"+firstPhoto+"/file", nil, second), 404)
	expect(t, request(t, h, "GET", "/api/photos/"+firstPhoto+"/file", nil, first), 200)
}
func TestThumbnailAndRevision(t *testing.T) {
	_, h := testApp(t)
	cookie := registerTest(t, h, "缩略测试", "")
	w := request(t, h, "GET", "/api/revision", nil, cookie)
	var before struct {
		Revision int64 `json:"revision"`
	}
	json.Unmarshal(w.Body.Bytes(), &before)
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	photo := memoryList(t, h, cookie)[0].Photos[0]
	expect(t, request(t, h, "GET", "/api/photos/"+photo.ID+"/thumb", nil, nil), 401)
	w = request(t, h, "GET", "/api/photos/"+photo.ID+"/thumb", nil, cookie)
	expect(t, w, 200)
	if len(w.Body.Bytes()) < 2 || w.Body.Bytes()[0] != 0xff || w.Body.Bytes()[1] != 0xd8 {
		t.Fatal("thumbnail is not JPEG")
	}
	if w.Header().Get("Vary") != "Cookie" {
		t.Fatal("media cache is not scoped by cookie")
	}
	w = request(t, h, "GET", "/api/revision", nil, cookie)
	var after struct {
		Revision int64 `json:"revision"`
	}
	json.Unmarshal(w.Body.Bytes(), &after)
	if after.Revision <= before.Revision {
		t.Fatal("revision did not advance")
	}
}
func TestV1BackupCreatesMemory(t *testing.T) {
	_, h := testApp(t)
	cookie := registerTest(t, h, "版本一兼容", "")
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	f, _ := z.Create("manifest.json")
	json.NewEncoder(f).Encode(Manifest{Version: 1, Photos: []Photo{{ID: "v1-photo", Filename: "one.png", Title: "旧照片", Note: "原备注", TakenAt: "2023-03-04"}}})
	f, _ = z.Create("photos/one.png")
	f.Write(samplePNG(t))
	z.Close()
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "v1.zip", b.Bytes(), cookie), 200)
	items := memoryList(t, h, cookie)
	if len(items) != 1 || items[0].Title != "旧照片" || items[0].Description != "原备注" {
		t.Fatal("v1 photo did not migrate into a memory")
	}
}

func TestBackupExportUsesRestoreLimit(t *testing.T) {
	t.Setenv("BACKUP_MAX_MB", "1")
	a, h := testApp(t)
	cookie := registerTest(t, h, "备份限制", "")
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	photo := memoryList(t, h, cookie)[0].Photos[0]
	if err := os.WriteFile(filepath.Join(a.dir, "photos", photo.Filename), make([]byte, 2<<20), 0600); err != nil {
		t.Fatal(err)
	}
	expect(t, request(t, h, "GET", "/api/backup", nil, cookie), 400)
}
func TestUpgradeSnapshotAndUnnamedPhoto(t *testing.T) {
	dir := t.TempDir()
	a, err := openApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := a.routes()
	cookie := registerTest(t, h, "升级保护", "")
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	if _, err = a.db.Exec(`DELETE FROM memory_photos;DROP TABLE memory_photos;DROP TABLE memories;UPDATE photos SET title='';PRAGMA user_version=0;`); err != nil {
		t.Fatal(err)
	}
	a.db.Close()
	a, err = openApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	items, err := a.memories(currentSpace(t, a, cookie))
	if err != nil || len(items) != 1 || items[0].Title == "" {
		t.Fatal("upgrade lost unnamed photo", err)
	}
	a.db.Close()
	snapshots, _ := filepath.Glob(filepath.Join(dir, "before-memory-migration-*.sqlite3"))
	if len(snapshots) != 1 {
		t.Fatal("upgrade did not create a snapshot")
	}
	a, err = openApp(dir)
	if err != nil {
		t.Fatal(err)
	}
	a.db.Close()
	snapshots, _ = filepath.Glob(filepath.Join(dir, "before-memory-migration-*.sqlite3"))
	if len(snapshots) != 1 {
		t.Fatal("restart created redundant migration snapshots")
	}
}

func TestMemoryPaginationFilteringAndIsolation(t *testing.T) {
	a, h := testApp(t)
	cookie := registerTest(t, h, "分页测试", "")
	for i := 0; i < 5; i++ {
		expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	}
	all := memoryList(t, h, cookie)
	first := all[0]
	first.City = "厦门"
	first.Description = "海边的100%纪念"
	first.Tags = []string{"晚霞"}
	first.Date = "2024-06-01"
	expect(t, request(t, h, "PUT", "/api/memories/"+first.ID, memoryInput{Memory: first}, cookie), 200)
	var seen = map[string]bool{}
	for page := 1; page <= 3; page++ {
		w := request(t, h, "GET", fmt.Sprintf("/api/memories?page=%d&per_page=2", page), nil, cookie)
		expect(t, w, 200)
		var result struct {
			Memories []Memory `json:"memories"`
			Total    int      `json:"total"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		if result.Total != 5 || len(result.Memories) == 0 || len(result.Memories) > 2 {
			t.Fatalf("bad page: %s", w.Body.String())
		}
		for _, m := range result.Memories {
			if seen[m.ID] {
				t.Fatal("duplicate across pages")
			}
			seen[m.ID] = true
			if len(m.Photos) != 1 {
				t.Fatal("missing page photos")
			}
		}
	}
	if len(seen) != 5 {
		t.Fatal("missing records")
	}
	for _, query := range []string{"q=100%25", "q=%E6%99%9A%E9%9C%9E", "year=2024&place=%E5%8E%A6%E9%97%A8&mode=love"} {
		w := request(t, h, "GET", "/api/memories?page=1&per_page=2&"+query, nil, cookie)
		expect(t, w, 200)
		var result struct {
			Memories []Memory `json:"memories"`
			Total    int      `json:"total"`
		}
		json.Unmarshal(w.Body.Bytes(), &result)
		if result.Total != 1 || len(result.Memories) != 1 || result.Memories[0].ID != first.ID {
			t.Fatalf("filter mismatch: %s", w.Body.String())
		}
	}
	for _, query := range []string{"page=0", "page=abc", "page=1&per_page=101", "page=1&year=oops", "page=1&mode=invalid"} {
		expect(t, request(t, h, "GET", "/api/memories?"+query, nil, cookie), 400)
	}
	expect(t, request(t, h, "GET", "/api/memories?page=1", nil, nil), 401)
	a.db.Exec(`INSERT INTO spaces(id) VALUES('pagination-outsider')`)
	a.db.Exec(`INSERT INTO users VALUES('pagination-outsider','分页外部账号','unused','pagination-outsider')`)
	a.db.Exec(`INSERT INTO sessions VALUES(?,?,?)`, digest("pagination-session"), "pagination-outsider", time.Now().Add(time.Hour).Unix())
	other := &http.Cookie{Name: "footprint_session", Value: "pagination-session"}
	w := request(t, h, "GET", "/api/memories?page=1", nil, other)
	expect(t, w, 200)
	var result struct {
		Total    int      `json:"total"`
		Memories []Memory `json:"memories"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Total != 0 || len(result.Memories) != 0 {
		t.Fatal("cross-space data leak")
	}
}

func TestLibraryIndexAndOnDemandDetail(t *testing.T) {
	_, h := testApp(t)
	first := registerTest(t, h, "索引甲", "")
	second := registerTest(t, h, "索引乙", testInvitation(t, h, first))
	expect(t, request(t, h, "POST", "/api/auth/unbind", map[string]any{}, second), 200)
	expect(t, uploadMany(t, h, first, 2, ""), 201)
	memory := memoryList(t, h, first)[0]
	photo := memory.Photos[0]
	photo.Note = "只在打开详情时获取的完整照片备注"
	expect(t, request(t, h, "PUT", "/api/photos/"+photo.ID, photo, first), 200)
	w := request(t, h, "GET", "/api/library", nil, first)
	expect(t, w, 200)
	var index struct {
		Memories []Memory `json:"memories"`
		Members  []User   `json:"members"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Memories) != 1 || len(index.Memories[0].Photos) != 2 || len(index.Members) != 1 {
		t.Fatal("index lost photo/member metadata")
	}
	for _, p := range index.Memories[0].Photos {
		if p.Note != "" {
			t.Fatal("index includes full photo notes")
		}
	}
	w = request(t, h, "GET", "/api/memories/"+memory.ID, nil, first)
	expect(t, w, 200)
	var detail Memory
	json.Unmarshal(w.Body.Bytes(), &detail)
	if detail.Photos[0].Note != photo.Note {
		t.Fatal("full detail lost photo note")
	}
	expect(t, request(t, h, "GET", "/api/memories/"+memory.ID, nil, second), 404)
	expect(t, request(t, h, "GET", "/api/library", nil, nil), 401)
	other := request(t, h, "GET", "/api/library", nil, second)
	expect(t, other, 200)
	if err := json.Unmarshal(other.Body.Bytes(), &index); err != nil {
		t.Fatal(err)
	}
	if len(index.Memories) != 0 {
		t.Fatal("index leaked another space")
	}
}
