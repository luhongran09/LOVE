package main

import (
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"github.com/gin-gonic/gin"
	"strconv"
	"time"
)

func (a *App) exportRoutes(api *gin.RouterGroup) {
	api.GET("/export/:format", func(c *gin.Context) {
		items, err := a.memories(current(c).SpaceID)
		if err != nil {
			fail(c, 500, "读取回忆失败")
			return
		}
		format := c.Param("format")
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="footprint-%s.%s"`, time.Now().Format("2006-01-02"), map[string]string{"json": "json", "csv": "csv", "gpx": "gpx", "geojson": "geojson"}[format]))
		switch format {
		case "json":
			c.JSON(200, gin.H{"version": 2, "memories": items})
		case "csv":
			c.Header("Content-Type", "text/csv; charset=utf-8")
			c.Writer.Write([]byte{0xef, 0xbb, 0xbf})
			w := csv.NewWriter(c.Writer)
			w.Write([]string{"标题", "类型", "日期", "地点", "省份", "城市", "备注", "照片数", "评分", "人均费用", "纬度", "经度"})
			safe := func(s string) string {
				if len(s) > 0 && (s[0] == '=' || s[0] == '+' || s[0] == '-' || s[0] == '@' || s[0] == '\t' || s[0] == '\r') {
					return "'" + s
				}
				return s
			}
			for _, m := range items {
				lat, lon := "", ""
				if m.Latitude != nil {
					lat = fmt.Sprint(*m.Latitude)
					lon = fmt.Sprint(*m.Longitude)
				}
				w.Write([]string{safe(m.Title), m.Mode, m.Date, safe(m.Place), safe(m.Province), safe(m.City), safe(m.Description), strconv.Itoa(len(m.Photos)), strconv.Itoa(m.Rating), fmt.Sprint(m.Price), lat, lon})
			}
			w.Flush()
		case "geojson":
			features := []any{}
			for _, m := range items {
				if m.Latitude != nil && m.Longitude != nil {
					features = append(features, gin.H{"type": "Feature", "geometry": gin.H{"type": "Point", "coordinates": []float64{*m.Longitude, *m.Latitude}}, "properties": gin.H{"title": m.Title, "date": m.Date, "place": m.Place, "mode": m.Mode, "description": m.Description}})
				}
			}
			c.JSON(200, gin.H{"type": "FeatureCollection", "features": features})
		case "gpx":
			type Point struct {
				Lat         float64 `xml:"lat,attr"`
				Lon         float64 `xml:"lon,attr"`
				Name        string  `xml:"name"`
				Description string  `xml:"desc"`
				Time        string  `xml:"time"`
			}
			type GPX struct {
				XMLName   xml.Name `xml:"gpx"`
				Version   string   `xml:"version,attr"`
				Creator   string   `xml:"creator,attr"`
				Namespace string   `xml:"xmlns,attr"`
				Points    []Point  `xml:"wpt"`
			}
			g := GPX{Version: "1.1", Creator: "Footprint", Namespace: "http://www.topografix.com/GPX/1/1", Points: []Point{}}
			for _, m := range items {
				if m.Latitude != nil && m.Longitude != nil {
					g.Points = append(g.Points, Point{*m.Latitude, *m.Longitude, m.Title, m.Description, m.Date + "T12:00:00Z"})
				}
			}
			c.Header("Content-Type", "application/gpx+xml; charset=utf-8")
			c.Writer.WriteString(xml.Header)
			xml.NewEncoder(c.Writer).Encode(g)
		default:
			c.Header("Content-Disposition", "")
			fail(c, 404, "不支持的导出格式")
		}
	})
}
