package main

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type mockRoundTrip func(*http.Request) (*http.Response, error)

func (f mockRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestGeocodeNormalizedAndInvalidCoordinates(t *testing.T) {
	_, h := testApp(t)
	cookie := registerTest(t, h, "地点测试", "")
	old := geocodeClient
	defer func() { geocodeClient = old }()
	geocodeClient = &http.Client{Transport: mockRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "nominatim.openstreetmap.org" || r.URL.Path != "/reverse" {
			t.Fatal("unexpected geocoder destination")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"display_name":"西湖, 杭州市, 浙江省","lat":"30.25","lon":"120.15","address":{"city":"杭州市","state":"浙江省"}}`)), Header: make(http.Header)}, nil
	})}
	geocoderGate.Lock()
	geocoderGate.last = time.Time{}
	geocoderGate.Unlock()
	w := request(t, h, "GET", "/api/places/reverse?lat=30.25&lon=120.15", nil, cookie)
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), "杭州市") {
		t.Fatal("city not normalized")
	}
	expect(t, request(t, h, "GET", "/api/places/reverse?lat=NaN&lon=120", nil, cookie), 400)
}
func TestLocalStoryNeverRequiresExternalModel(t *testing.T) {
	t.Setenv("LLM_API_KEY", "")
	_, h := testApp(t)
	cookie := registerTest(t, h, "故事测试", "")
	expect(t, uploadMany(t, h, cookie, 1, ""), 201)
	w := request(t, h, "POST", "/api/story", map[string]any{}, cookie)
	expect(t, w, 200)
	if !strings.Contains(w.Body.String(), "第一次一起看海") || !strings.Contains(w.Body.String(), `"source":"local"`) {
		t.Fatal("story did not use real memories")
	}
}
