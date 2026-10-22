package main

import (
	"bytes"
	"github.com/gin-gonic/gin"
	"github.com/rwcarlsen/goexif/exif"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
)

var thumbnailGate = make(chan struct{}, 1)

func makeThumbnail(data []byte) ([]byte, error) {
	source, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	bounds := source.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	if width > 720 || height > 720 {
		if width >= height {
			height = height * 720 / width
			width = 720
		} else {
			width = width * 720 / height
			height = 720
		}
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	resized := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(resized, resized.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.ApproxBiLinear.Scale(resized, resized.Bounds(), source, bounds, draw.Over, nil)
	orientation := 1
	if metadata, err := exif.Decode(bytes.NewReader(data)); err == nil {
		if value, err := metadata.Get(exif.Orientation); err == nil {
			if n, err := value.Int(0); err == nil {
				orientation = n
			}
		}
	}
	var result image.Image = resized
	if orientation >= 2 && orientation <= 8 {
		w, h := width, height
		if orientation >= 5 {
			w, h = height, width
		}
		rotated := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < height; y++ {
			for x := 0; x < width; x++ {
				dx, dy := x, y
				switch orientation {
				case 2:
					dx = width - 1 - x
				case 3:
					dx, dy = width-1-x, height-1-y
				case 4:
					dy = height - 1 - y
				case 5:
					dx, dy = y, x
				case 6:
					dx, dy = height-1-y, x
				case 7:
					dx, dy = height-1-y, width-1-x
				case 8:
					dx, dy = y, width-1-x
				}
				rotated.Set(dx, dy, resized.At(x, y))
			}
		}
		result = rotated
	}
	var output bytes.Buffer
	err = jpeg.Encode(&output, result, &jpeg.Options{Quality: 82})
	return output.Bytes(), err
}
func (a *App) thumbnail(c *gin.Context) {
	p, ok := a.photo(c)
	if !ok {
		return
	}
	dir := filepath.Join(a.dir, "thumbs")
	file := filepath.Join(dir, p.ID+".jpg")
	c.Header("Cache-Control", "private, no-cache")
	c.Header("Vary", "Cookie")
	if _, err := os.Stat(file); err == nil {
		c.File(file)
		return
	}
	select {
	case thumbnailGate <- struct{}{}:
		defer func() { <-thumbnailGate }()
	case <-c.Request.Context().Done():
		return
	}
	if _, err := os.Stat(file); err == nil {
		c.File(file)
		return
	}
	data, err := os.ReadFile(filepath.Join(a.dir, "photos", p.Filename))
	if err != nil {
		fail(c, 404, "原图不存在")
		return
	}
	thumb, err := makeThumbnail(data)
	if err != nil {
		fail(c, 400, "无法生成此图片的缩略图")
		return
	}
	if os.MkdirAll(dir, 0700) != nil {
		fail(c, 500, "保存缩略图失败")
		return
	}
	temporary, err := os.CreateTemp(dir, "thumb-*.jpg")
	if err != nil {
		fail(c, 500, "保存缩略图失败")
		return
	}
	name := temporary.Name()
	defer os.Remove(name)
	_, writeErr := temporary.Write(thumb)
	closeErr := temporary.Close()
	if writeErr != nil || closeErr != nil || os.Rename(name, file) != nil {
		fail(c, 500, "保存缩略图失败")
		return
	}
	c.File(file)
}
