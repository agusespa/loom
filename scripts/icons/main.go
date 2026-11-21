// Command icons draws the app icons into web/static.
// Run with `make icons`; the PNGs are committed.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"os"
	"path/filepath"
)

var (
	bg    = color.RGBA{0x0f, 0x6e, 0x56, 0xff}
	solid = color.RGBA{0xff, 0xff, 0xff, 0xff}
	faint = color.RGBA{0x5d, 0xca, 0xa5, 0xff}
)

// A 3x3 habit grid, the same pattern as icon.svg: 1 = done, 0 = not.
var pattern = [3][3]int{{1, 1, 0}, {1, 0, 1}, {1, 1, 1}}

func draw(size int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			img.Set(x, y, bg)
		}
	}
	// Content sits in the middle 60% so maskable crops never cut it.
	cell := float64(size) * 0.16
	gap := float64(size) * 0.06
	origin := (float64(size) - (3*cell + 2*gap)) / 2
	radius := cell * 0.22
	for r := range 3 {
		for c := range 3 {
			col := faint
			if pattern[r][c] == 1 {
				col = solid
			}
			x0 := origin + float64(c)*(cell+gap)
			y0 := origin + float64(r)*(cell+gap)
			roundRect(img, x0, y0, cell, radius, col)
		}
	}
	return img
}

func roundRect(img *image.RGBA, x0, y0, side, r float64, col color.RGBA) {
	for y := int(y0); y < int(y0+side+1); y++ {
		for x := int(x0); x < int(x0+side+1); x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			if px < x0 || py < y0 || px > x0+side || py > y0+side {
				continue
			}
			cx := clamp(px, x0+r, x0+side-r)
			cy := clamp(py, y0+r, y0+side-r)
			if (px-cx)*(px-cx)+(py-cy)*(py-cy) <= r*r {
				img.Set(x, y, col)
			}
		}
	}
}

func clamp(v, lo, hi float64) float64 {
	return max(lo, min(v, hi))
}

func main() {
	dir := filepath.Join("web", "static")
	for name, size := range map[string]int{"icon-192.png": 192, "icon-512.png": 512, "apple-touch-icon.png": 180} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			log.Fatal(err)
		}
		if err := png.Encode(f, draw(size)); err != nil {
			log.Fatal(err)
		}
		if err := f.Close(); err != nil {
			log.Fatal(err)
		}
	}
}
