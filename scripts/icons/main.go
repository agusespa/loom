// Command icons derives the app's PNG icons from the square master,
// web/static/icon-maskable-512.png (a full-bleed render of icon.svg).
// Run with `make icons`; the PNGs are committed.
//
//   - icon-192.png, icon-512.png: rounded corners on a transparent
//     background, for the manifest's "any" purpose and the README.
//   - apple-touch-icon.png: opaque square, since iOS rounds it itself.
package main

import (
	"image"
	"image/color"
	"image/png"
	"log"
	"math"
	"os"
	"path/filepath"
)

// Corner radius as a share of the side, matching rx="230" in icon.svg.
const cornerRatio = 230.0 / 1024.0

func main() {
	dir := filepath.Join("web", "static")
	master := load(filepath.Join(dir, "icon-maskable-512.png"))

	for name, size := range map[string]int{"icon-192.png": 192, "icon-512.png": 512} {
		save(filepath.Join(dir, name), round(resize(master, size)))
	}
	save(filepath.Join(dir, "apple-touch-icon.png"), resize(master, 180))
}

func load(path string) image.Image {
	f, err := os.Open(path)
	if err != nil {
		log.Fatal(err)
	}
	img, err := png.Decode(f)
	if err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	return img
}

func save(path string, img image.Image) {
	f, err := os.Create(path)
	if err != nil {
		log.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

// resize scales src to size x size by averaging the source area under
// each output pixel.
func resize(src image.Image, size int) *image.NRGBA {
	b := src.Bounds()
	scale := float64(b.Dx()) / float64(size)
	out := image.NewNRGBA(image.Rect(0, 0, size, size))
	for y := range size {
		for x := range size {
			x0, x1 := float64(x)*scale, float64(x+1)*scale
			y0, y1 := float64(y)*scale, float64(y+1)*scale
			var r, g, bl, a, w float64
			for sy := int(y0); sy < int(math.Ceil(y1)); sy++ {
				wy := math.Min(y1, float64(sy+1)) - math.Max(y0, float64(sy))
				for sx := int(x0); sx < int(math.Ceil(x1)); sx++ {
					wx := math.Min(x1, float64(sx+1)) - math.Max(x0, float64(sx))
					c := color.NRGBAModel.Convert(src.At(b.Min.X+sx, b.Min.Y+sy)).(color.NRGBA)
					k := wx * wy
					r += float64(c.R) * k
					g += float64(c.G) * k
					bl += float64(c.B) * k
					a += float64(c.A) * k
					w += k
				}
			}
			out.SetNRGBA(x, y, color.NRGBA{uint8(r/w + 0.5), uint8(g/w + 0.5), uint8(bl/w + 0.5), uint8(a/w + 0.5)})
		}
	}
	return out
}

// round makes the corners transparent, antialiasing the edge.
func round(img *image.NRGBA) *image.NRGBA {
	size := float64(img.Bounds().Dx())
	r := size * cornerRatio
	for y := range img.Bounds().Dy() {
		for x := range img.Bounds().Dx() {
			px, py := float64(x)+0.5, float64(y)+0.5
			cx := math.Max(r, math.Min(px, size-r))
			cy := math.Max(r, math.Min(py, size-r))
			d := math.Hypot(px-cx, py-cy)
			cover := math.Max(0, math.Min(1, r-d+0.5))
			c := img.NRGBAAt(x, y)
			c.A = uint8(float64(c.A)*cover + 0.5)
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}
