package main

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func testInvitation(t *testing.T, h http.Handler, cookie *http.Cookie) string {
	t.Helper()
	w := request(t, h, "POST", "/api/invite", nil, cookie)
	expect(t, w, 200)
	var result struct {
		Invite string `json:"invite"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result.Invite
}
func testPairPreview(t *testing.T, h http.Handler, cookie *http.Cookie, invite string) pairPreview {
	t.Helper()
	w := request(t, h, "POST", "/api/auth/pair/preview", map[string]string{"invite": invite}, cookie)
	expect(t, w, 200)
	var result pairPreview
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func testFeature(t *testing.T, h http.Handler, cookie *http.Cookie, key string, data any) {
	t.Helper()
	w := request(t, h, "GET", "/api/features/"+key, nil, cookie)
	expect(t, w, 200)
	var result struct {
		Revision string `json:"revision"`
	}
	json.Unmarshal(w.Body.Bytes(), &result)
	expect(t, request(t, h, "PUT", "/api/features/"+key, map[string]any{"data": data, "revision": result.Revision}, cookie), 200)
}
func TestExistingAccountsPairPreservesPhotosAndLists(t *testing.T) {
	a, h := testApp(t)
	first := registerTest(t, h, "已有账号甲", "")
	second := registerTest(t, h, "已有账号乙", testInvitation(t, h, first))
	expect(t, request(t, h, "POST", "/api/auth/unbind", map[string]any{}, second), 200)
	expect(t, uploadMany(t, h, first, 2, ""), 201)
	expect(t, uploadMany(t, h, second, 3, ""), 201)
	source := memoryList(t, h, second)[0]
	target := memoryList(t, h, first)[0]
	source.Title = "迁入的回忆"
	source.CoverID = source.Photos[2].ID
	source.Tags = []string{"重要"}
	ids := []string{source.Photos[2].ID, source.Photos[0].ID, source.Photos[1].ID}
	expect(t, request(t, h, "PUT", "/api/memories/"+source.ID, memoryInput{Memory: source, PhotoIDs: ids}, second), 200)
	same := map[string]any{"id": "same", "title": "共同条目"}
	testFeature(t, h, first, "wishes", []any{same, map[string]any{"id": "changed", "title": "甲的修改"}})
	testFeature(t, h, second, "wishes", []any{same, map[string]any{"id": "changed", "title": "乙的修改"}})
	testFeature(t, h, first, "space_settings", map[string]any{"together_date": "2020-05-20"})
	testFeature(t, h, second, "space_settings", map[string]any{"together_date": "2021-05-20", "note": "保留新字段"})
	testFeature(t, h, second, "packing_list", []any{map[string]any{"id": "packing", "title": "相机"}})
	invite := testInvitation(t, h, first)
	preview := testPairPreview(t, h, second, invite)
	if preview.Source.Photos != 3 || preview.Target.Photos != 2 || preview.Target.Name != "已有账号甲" {
		t.Fatalf("wrong preview: %+v", preview)
	}
	input := map[string]any{"invite": invite, "password": "wrong", "source_revision": preview.Source.Revision, "target_revision": preview.Target.Revision}
	expect(t, request(t, h, "POST", "/api/auth/pair", input, second), 403)
	input["password"] = "test-password-123"
	testFeature(t, h, first, "anniversaries", []any{map[string]any{"id": "day", "title": "一起出发"}})
	expect(t, request(t, h, "POST", "/api/auth/pair", input, second), 409)
	if len(memoryList(t, h, second)) != 1 {
		t.Fatal("stale confirmation changed data")
	}
	preview = testPairPreview(t, h, second, invite)
	input["source_revision"] = preview.Source.Revision
	input["target_revision"] = preview.Target.Revision
	expect(t, request(t, h, "POST", "/api/auth/pair", input, second), 200)
	for _, cookie := range []*http.Cookie{first, second} {
		items := memoryList(t, h, cookie)
		if len(items) != 2 {
			t.Fatal("both users must see both memories")
		}
		for _, m := range items {
			if m.ID == source.ID {
				if len(m.Photos) != 3 || m.CoverID != ids[0] || m.Photos[0].ID != ids[0] || m.Title != "迁入的回忆" {
					t.Fatal("metadata or photo order changed")
				}
			}
		}
		expect(t, request(t, h, "GET", "/api/photos/"+source.Photos[0].ID+"/file", nil, cookie), 200)
		expect(t, request(t, h, "GET", "/api/photos/"+target.Photos[0].ID+"/file", nil, cookie), 200)
	}
	w := request(t, h, "GET", "/api/features/wishes", nil, first)
	expect(t, w, 200)
	var feature struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &feature)
	if len(feature.Data) != 3 || feature.Data[1]["id"] == feature.Data[2]["id"] {
		t.Fatalf("list entries lost or share conflicting IDs: %s", w.Body.String())
	}
	w = request(t, h, "GET", "/api/features/space_settings", nil, second)
	var settings struct {
		Data map[string]any `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &settings)
	if settings.Data["together_date"] != "2020-05-20" || settings.Data["note"] != "保留新字段" {
		t.Fatal("settings merge policy mismatch")
	}
	expect(t, request(t, h, "POST", "/api/auth/pair/preview", map[string]string{"invite": invite}, second), 403)
	var count int
	a.db.QueryRow(`SELECT count(*) FROM users WHERE space_id=(SELECT space_id FROM users WHERE name='已有账号甲')`).Scan(&count)
	if count != 2 {
		t.Fatal("wrong membership")
	}
	expect(t, request(t, h, "POST", "/api/auth/unbind", map[string]any{}, second), 200)
	if len(memoryList(t, h, second)) != 1 || len(memoryList(t, h, first)) != 1 {
		t.Fatal("re-pair then unbind lost photo ownership")
	}
	expect(t, request(t, h, "GET", "/api/photos/"+target.Photos[0].ID+"/file", nil, second), 404)
}
func TestPairingValidatesInviteMembershipAndRollback(t *testing.T) {
	a, h := testApp(t)
	first := registerTest(t, h, "校验账号甲", "")
	second := registerTest(t, h, "校验账号乙", testInvitation(t, h, first))
	expect(t, request(t, h, "POST", "/api/auth/unbind", map[string]any{}, second), 200)
	invite := testInvitation(t, h, first)
	expect(t, request(t, h, "POST", "/api/auth/pair/preview", map[string]string{"invite": invite}, nil), 401)
	expect(t, request(t, h, "POST", "/api/auth/pair/preview", map[string]string{"invite": invite}, first), 400)
	expect(t, request(t, h, "POST", "/api/auth/pair", map[string]any{"invite": invite, "password": "test-password-123"}, second), 400)
	expect(t, uploadMany(t, h, second, 1, ""), 201)
	// Malformed list data is rejected atomically rather than discarded.
	testFeature(t, h, first, "wishes", []any{})
	testFeature(t, h, second, "wishes", map[string]any{"unexpected": "preserve me"})
	preview := testPairPreview(t, h, second, invite)
	input := map[string]any{"invite": invite, "password": "test-password-123", "source_revision": preview.Source.Revision, "target_revision": preview.Target.Revision}
	expect(t, request(t, h, "POST", "/api/auth/pair", input, second), 409)
	if len(memoryList(t, h, first)) != 0 || len(memoryList(t, h, second)) != 1 {
		t.Fatal("failed merge changed photo spaces")
	}
	// Invite remains usable after rollback, but expiry is checked at confirmation.
	a.db.Exec(`UPDATE spaces SET invite_expires=? WHERE invite_hash=?`, time.Now().Add(-time.Second).Unix(), digest(invite))
	expect(t, request(t, h, "POST", "/api/auth/pair", input, second), 403)
}
