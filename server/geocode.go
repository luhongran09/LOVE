package main

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type PlaceResult struct {
	Name      string  `json:"name"`
	Province  string  `json:"province"`
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

var geocoderGate = struct {
	sync.Mutex
	last time.Time
}{}
var geocodeClient = &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return errors.New("redirects disabled") }}

func fetchGeocode(c *gin.Context, endpoint string, params url.Values, target any) error {
	geocoderGate.Lock()
	if time.Since(geocoderGate.last) < time.Second {
		geocoderGate.Unlock()
		return errors.New("地点查询过于频繁，请稍后再试")
	}
	geocoderGate.last = time.Now()
	geocoderGate.Unlock()
	req, err := http.NewRequestWithContext(c.Request.Context(), "GET", "https://nominatim.openstreetmap.org/"+endpoint+"?"+params.Encode(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "FootprintPrivateAlbum/0.2 (self-hosted photo journal)")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.5")
	response, err := geocodeClient.Do(req)
	if err != nil {
		return errors.New("地点服务暂时不可用，你仍可以手动填写地点")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return errors.New("地点服务暂时不可用，请稍后重试")
	}
	return json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(target)
}

type nominatimPlace struct {
	DisplayName string            `json:"display_name"`
	Lat         string            `json:"lat"`
	Lon         string            `json:"lon"`
	Address     map[string]string `json:"address"`
}

func convertPlace(p nominatimPlace) PlaceResult {
	lat, _ := strconv.ParseFloat(p.Lat, 64)
	lon, _ := strconv.ParseFloat(p.Lon, 64)
	city := ""
	for _, key := range []string{"city", "town", "municipality", "county", "village"} {
		if p.Address[key] != "" {
			city = p.Address[key]
			break
		}
	}
	return PlaceResult{p.DisplayName, p.Address["state"], city, lat, lon}
}
func (a *App) geocodeRoutes(api *gin.RouterGroup) {
	api.GET("/places/search", func(c *gin.Context) {
		query := strings.TrimSpace(c.Query("q"))
		if len([]rune(query)) < 2 || len([]rune(query)) > 200 {
			fail(c, 400, "请输入 2–200 字的地名")
			return
		}
		var items []nominatimPlace
		err := fetchGeocode(c, "search", url.Values{"q": {query}, "format": {"jsonv2"}, "addressdetails": {"1"}, "limit": {"5"}, "accept-language": {"zh-CN"}}, &items)
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		places := []PlaceResult{}
		for _, p := range items {
			places = append(places, convertPlace(p))
		}
		c.JSON(200, gin.H{"places": places, "provider": "OpenStreetMap Nominatim"})
	})
	api.GET("/places/reverse", func(c *gin.Context) {
		lat, err1 := strconv.ParseFloat(c.Query("lat"), 64)
		lon, err2 := strconv.ParseFloat(c.Query("lon"), 64)
		if err1 != nil || err2 != nil || math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) || lat < -90 || lat > 90 || lon < -180 || lon > 180 {
			fail(c, 400, "经纬度无效")
			return
		}
		var place nominatimPlace
		err := fetchGeocode(c, "reverse", url.Values{"lat": {c.Query("lat")}, "lon": {c.Query("lon")}, "format": {"jsonv2"}, "addressdetails": {"1"}, "accept-language": {"zh-CN"}}, &place)
		if err != nil {
			fail(c, 502, err.Error())
			return
		}
		if place.DisplayName == "" {
			fail(c, 404, "没有找到这个坐标的地点名称")
			return
		}
		result := convertPlace(place)
		result.Latitude = lat
		result.Longitude = lon
		c.JSON(200, result)
	})
}
