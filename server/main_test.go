package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func testApp(t *testing.T) (*App, http.Handler) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	a, err := openApp(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.db.Close() })
	return a, a.routes()
}
func request(t *testing.T, h http.Handler, method, path string, data any, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	if data != nil {
		if err := json.NewEncoder(&b).Encode(data); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest(method, path, &b)
	r.Header.Set("Content-Type", "application/json")
	if cookie != nil {
		r.AddCookie(cookie)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func expect(t *testing.T, w *httptest.ResponseRecorder, status int) {
	t.Helper()
	if w.Code != status {
		t.Fatalf("want %d got %d: %s", status, w.Code, w.Body.String())
	}
}
func registerTest(t *testing.T, h http.Handler, name, invite string) *http.Cookie {
	t.Helper()
	w := request(t, h, "POST", "/api/auth/register", map[string]string{"name": name, "password": "test-password-123", "invite": invite}, nil)
	expect(t, w, 200)
	return w.Result().Cookies()[0]
}
func multipartRequest(t *testing.T, h http.Handler, path, key, filename string, data []byte, cookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	f, err := mw.CreateFormFile(key, filename)
	if err != nil {
		t.Fatal(err)
	}
	f.Write(data)
	if key == "photos" {
		mw.WriteField("taken_at", "2025-05-20")
		mw.WriteField("place", "杭州 · 西湖")
	}
	mw.Close()
	r := httptest.NewRequest("POST", path, &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	r.AddCookie(cookie)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}
func samplePNG(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestSharedAlbumLifecycle(t *testing.T) {
	a, h := testApp(t)
	expect(t, request(t, h, "GET", "/api/photos", nil, nil), 401)
	first := registerTest(t, h, "小明", "")
	expect(t, request(t, h, "POST", "/api/auth/register", map[string]string{"name": "陌生人", "password": "test-password-123"}, nil), 403)
	w := request(t, h, "POST", "/api/invite", nil, first)
	expect(t, w, 200)
	var invitation struct {
		Invite string `json:"invite"`
	}
	json.Unmarshal(w.Body.Bytes(), &invitation)
	second := registerTest(t, h, "小红", invitation.Invite)
	expect(t, request(t, h, "POST", "/api/auth/register", map[string]string{"name": "第三人", "password": "test-password-123", "invite": invitation.Invite}, nil), 403)
	expect(t, request(t, h, "POST", "/api/invite", nil, first), 409)
	expect(t, multipartRequest(t, h, "/api/photos/upload", "photos", "bad.svg", []byte("<svg/>"), first), 400)
	w = multipartRequest(t, h, "/api/photos/upload", "photos", "小旅行.png", samplePNG(t), first)
	expect(t, w, 201)
	var result struct {
		Photos []Photo `json:"photos"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	p := result.Photos[0]
	if p.Place != "杭州 · 西湖" || p.TakenAt != "2025-05-20" {
		t.Fatalf("upload metadata lost: %+v", p)
	}
	expect(t, request(t, h, "GET", "/api/photos/"+p.ID+"/file", nil, nil), 401)
	w = request(t, h, "GET", "/api/photos/"+p.ID+"/file", nil, second)
	expect(t, w, 200)
	if !bytes.Equal(w.Body.Bytes(), samplePNG(t)) {
		t.Fatal("original image changed")
	}
	lat, lon := 30.25, 120.15
	p.Note = "一起看落日"
	p.Latitude = &lat
	p.Longitude = &lon
	expect(t, request(t, h, "PUT", "/api/photos/"+p.ID, p, second), 200)
	w = request(t, h, "GET", "/api/photos", nil, first)
	expect(t, w, 200)
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Photos[0].Note != p.Note {
		t.Fatal("partner edit not shared")
	}
	invalid := p
	invalid.TakenAt = "2025-02-31"
	expect(t, request(t, h, "PUT", "/api/photos/"+p.ID, invalid, second), 400)
	// A valid session in a separate space must not read, edit, or delete this photo.
	a.db.Exec(`INSERT INTO spaces(id) VALUES('other')`)
	a.db.Exec(`INSERT INTO users VALUES('outsider','outsider','unused','other')`)
	a.db.Exec(`INSERT INTO sessions VALUES(?,'outsider',9999999999)`, digest("outsider-token"))
	outsider := &http.Cookie{Name: "footprint_session", Value: "outsider-token"}
	for _, method := range []string{"GET", "PUT", "DELETE"} {
		path := "/api/photos/" + p.ID
		if method == "GET" {
			path += "/file"
		}
		expect(t, request(t, h, method, path, p, outsider), 404)
	}
	// Export includes originals and metadata; restore is additive and idempotent.
	w = request(t, h, "GET", "/api/backup", nil, first)
	expect(t, w, 200)
	backup := append([]byte(nil), w.Body.Bytes()...)
	z, err := zip.NewReader(bytes.NewReader(backup), int64(len(backup)))
	if err != nil || len(z.File) != 2 {
		t.Fatal("backup missing original or manifest", err)
	}
	w = multipartRequest(t, h, "/api/backup/restore", "backup", "backup.zip", backup, second)
	expect(t, w, 200)
	if !bytes.Contains(w.Body.Bytes(), []byte(`"skipped":1`)) {
		t.Fatal("restore duplicated existing photo")
	}
	expect(t, request(t, h, "DELETE", "/api/photos/"+p.ID, nil, first), 204)
	if _, err = os.Stat(filepath.Join(a.dir, "photos", p.Filename)); !os.IsNotExist(err) {
		t.Fatal("deleted original still exists")
	}
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "backup.zip", backup, second), 200)
	w = request(t, h, "GET", "/api/photos", nil, first)
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.Photos) != 1 || result.Photos[0].Note != p.Note || *result.Photos[0].Latitude != lat {
		t.Fatal("restore lost photo metadata")
	}
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "backup.zip", backup, second), 200)
	expect(t, request(t, h, "POST", "/api/auth/logout", nil, first), 204)
	expect(t, request(t, h, "GET", "/api/photos", nil, first), 401)
}
func TestRestoreRollbackAndCrossSite(t *testing.T) {
	a, h := testApp(t)
	cookie := registerTest(t, h, "测试账号", "")
	r := httptest.NewRequest("POST", "/api/invite", nil)
	r.AddCookie(cookie)
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	expect(t, w, 403)
	// First entry is valid, second is missing. Neither may survive a failed restore.
	p := Photo{ID: "one", Filename: "one.png", Title: "test", TakenAt: "2025-05-20"}
	q := p
	q.ID = "two"
	q.Filename = "missing.png"
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	m, _ := z.Create("manifest.json")
	json.NewEncoder(m).Encode(Manifest{Version: 1, Photos: []Photo{p, q}})
	f, _ := z.Create("photos/one.png")
	io.Copy(f, bytes.NewReader(samplePNG(t)))
	z.Close()
	expect(t, multipartRequest(t, h, "/api/backup/restore", "backup", "backup.zip", b.Bytes(), cookie), 400)
	var count int
	a.db.QueryRow(`SELECT count(*) FROM photos`).Scan(&count)
	entries, _ := os.ReadDir(filepath.Join(a.dir, "photos"))
	if count != 0 || len(entries) != 0 {
		t.Fatal("failed restore left partial records or files")
	}
}

func TestLoginAndInviteExpiry(t *testing.T) {
	a, h := testApp(t)
	cookie := registerTest(t, h, "登录测试", "")
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode {
		t.Fatal("session cookie must be HttpOnly and SameSite=Lax")
	}
	expect(t, request(t, h, "POST", "/api/auth/login", map[string]string{"name": "登录测试", "password": "incorrect"}, nil), 401)
	w := request(t, h, "POST", "/api/auth/login", map[string]string{"name": "登录测试", "password": "test-password-123"}, nil)
	expect(t, w, 200)
	expect(t, request(t, h, "GET", "/api/auth/me", nil, w.Result().Cookies()[0]), 200)
	a.db.Exec(`UPDATE spaces SET invite_hash=?,invite_expires=0`, digest("expired-invite"))
	expect(t, request(t, h, "POST", "/api/auth/register", map[string]string{"name": "过期邀请", "password": "test-password-123", "invite": "expired-invite"}, nil), 403)
	var count int
	a.db.QueryRow(`SELECT count(*) FROM users`).Scan(&count)
	if count != 1 {
		t.Fatal("expired invite created an account")
	}
}
