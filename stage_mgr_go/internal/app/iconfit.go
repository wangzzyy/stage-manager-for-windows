package app

import (
	"image"
	"image/draw"
)

// Icons are drawn at one size, but apps leave different margins around
// them, and the shell puts an icon it has no large image of small in the
// middle of a big tile: the strip trims every icon to what it shows.

// iconAlpha is how opaque a pixel must be to count as the icon, which
// leaves out the faint frame of the shell's tiles: up to 77 of 255.
const iconAlpha = 128

// iconBounds returns the part of an icon that shows.
func iconBounds(img *image.NRGBA) image.Rectangle {
	r := img.Bounds()
	minX, minY, maxX, maxY := r.Max.X, r.Max.Y, r.Min.X-1, r.Min.Y-1
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if img.Pix[img.PixOffset(x, y)+3] >= iconAlpha {
				minX, minY, maxX, maxY = min(minX, x), min(minY, y), max(maxX, x), max(maxY, y)
			}
		}
	}
	if maxX < minX {
		return image.Rectangle{}
	}
	return image.Rect(minX, minY, maxX+1, maxY+1)
}

// trimIcon crops an icon to what shows, centered in a square.
func trimIcon(img *image.NRGBA) *image.NRGBA {
	b := iconBounds(img)
	if b.Empty() || b == img.Bounds() && b.Dx() == b.Dy() {
		return img
	}
	side := max(b.Dx(), b.Dy())
	out := image.NewNRGBA(image.Rect(0, 0, side, side))
	at := image.Pt((side-b.Dx())/2, (side-b.Dy())/2)
	draw.Draw(out, image.Rectangle{Min: at, Max: at.Add(b.Size())}, img, b.Min, draw.Src)
	return out
}

// toNRGBA converts a decoded image.
func toNRGBA(img image.Image) *image.NRGBA {
	if n, ok := img.(*image.NRGBA); ok {
		return n
	}
	out := image.NewNRGBA(img.Bounds())
	draw.Draw(out, out.Bounds(), img, img.Bounds().Min, draw.Src)
	return out
}

// iconShadow makes a diffuse black shadow from the icon's alpha channel.
// It has no unblurred silhouette or outline: every shadow pixel comes from
// the blurred mask, which preserves transparent and irregular icon shapes.
func iconShadow(img *image.NRGBA) *image.NRGBA {
	const (
		radius        = 24
		passes        = 3
		offsetY       = 5
		blurStrength  = 50
		padding       = radius*passes + offsetY
	)
	bounds := img.Bounds()
	w, h := bounds.Dx()+padding*2, bounds.Dy()+padding*2
	alpha := make([]uint8, w*h)
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			src := img.PixOffset(bounds.Min.X+x, bounds.Min.Y+y)
			dst := (y+padding+offsetY)*w + x + padding
			alpha[dst] = img.Pix[src+3]
		}
	}
	for i := 0; i < passes; i++ {
		alpha = boxBlurAlpha(alpha, w, h, radius)
	}
	out := image.NewNRGBA(image.Rect(0, 0, w, h))
	for i, a := range alpha {
		out.Pix[i*4+3] = uint8(int(a) * blurStrength / 100)
	}
	return out
}

// boxBlurAlpha applies a separable box blur to an alpha mask. Repeating it
// three times gives a smooth, Gaussian-like falloff without a GPU effect or
// per-frame work.
func boxBlurAlpha(src []uint8, w, h, radius int) []uint8 {
	tmp := make([]uint8, len(src))
	out := make([]uint8, len(src))
	span := radius*2 + 1
	for y := 0; y < h; y++ {
		sum := 0
		for x := -radius; x <= radius; x++ {
			if x >= 0 && x < w {
				sum += int(src[y*w+x])
			}
		}
		for x := 0; x < w; x++ {
			tmp[y*w+x] = uint8(sum / span)
			left, right := x-radius, x+radius+1
			if left >= 0 {
				sum -= int(src[y*w+left])
			}
			if right < w {
				sum += int(src[y*w+right])
			}
		}
	}
	for x := 0; x < w; x++ {
		sum := 0
		for y := -radius; y <= radius; y++ {
			if y >= 0 && y < h {
				sum += int(tmp[y*w+x])
			}
		}
		for y := 0; y < h; y++ {
			out[y*w+x] = uint8(sum / span)
			top, bottom := y-radius, y+radius+1
			if top >= 0 {
				sum -= int(tmp[top*w+x])
			}
			if bottom < h {
				sum += int(tmp[bottom*w+x])
			}
		}
	}
	return out
}
