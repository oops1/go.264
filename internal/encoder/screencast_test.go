package encoder

import "math"

type canvas struct {
	w, h int
	y    []byte
	cb   []byte
	cr   []byte
}

func newCanvas(w, h int) *canvas {
	return &canvas{w: w, h: h, y: make([]byte, w*h),
		cb: make([]byte, w*h/4), cr: make([]byte, w*h/4)}
}

func (c *canvas) fill(x0, y0, x1, y1 int, luma, cb, cr byte) {
	if x0 < 0 {
		x0 = 0
	}
	if y0 < 0 {
		y0 = 0
	}
	if x1 > c.w {
		x1 = c.w
	}
	if y1 > c.h {
		y1 = c.h
	}
	for y := y0; y < y1; y++ {
		row := c.y[y*c.w:]
		for x := x0; x < x1; x++ {
			row[x] = luma
		}
	}
	cw := c.w / 2
	for y := y0 / 2; y < (y1+1)/2 && y < c.h/2; y++ {
		for x := x0 / 2; x < (x1+1)/2 && x < cw; x++ {
			c.cb[y*cw+x] = cb
			c.cr[y*cw+x] = cr
		}
	}
}

func (c *canvas) pixel(x, y int, luma byte) {
	if x < 0 || y < 0 || x >= c.w || y >= c.h {
		return
	}
	c.y[y*c.w+x] = luma
}

func (c *canvas) border(x0, y0, x1, y1 int, luma byte) {
	for x := x0; x < x1; x++ {
		c.pixel(x, y0, luma)
		c.pixel(x, y1-1, luma)
	}
	for y := y0; y < y1; y++ {
		c.pixel(x0, y, luma)
		c.pixel(x1-1, y, luma)
	}
}

const glyphW, glyphH, glyphAdvance, glyphLead = 5, 9, 7, 13

func glyphBits(code int) uint64 {
	v := uint64(code)*0x9E3779B97F4A7C15 + 0x123456789ABCDEF
	v ^= v >> 29
	v *= 0xBF58476D1CE4E5B9
	v ^= v >> 32
	return v
}

func (c *canvas) glyph(x, y, code int, ink byte) {
	bits := glyphBits(code)
	for gy := 0; gy < glyphH; gy++ {
		for gx := 0; gx < glyphW; gx++ {
			if bits&(1<<uint((gy*glyphW+gx)%64)) == 0 {
				continue
			}
			c.pixel(x+gx, y+gy, ink)
		}
	}
}

func (c *canvas) text(x, y, count, seed int, ink byte) {
	for i := 0; i < count; i++ {
		code := seed*7919 + i*31
		if (code>>3)%9 == 0 {
			continue
		}
		c.glyph(x+i*glyphAdvance, y, code, ink)
	}
}

func (c *canvas) paragraph(x, y, width, lines, seed int, ink byte) {
	perLine := width / glyphAdvance
	if perLine < 4 {
		return
	}
	for l := 0; l < lines; l++ {
		c.text(x, y+l*glyphLead, perLine-(seed+l*13)%(perLine/3+1), seed+l, ink)
	}
}

func (c *canvas) cursor(x, y int) {
	for dy := 0; dy < 17; dy++ {
		width := 11 - dy/2
		if dy > 11 {
			width = 4
		}
		for dx := 0; dx < width; dx++ {
			ink := byte(235)
			if dx == 0 || dx == width-1 || dy == 0 {
				ink = 16
			}
			c.pixel(x+dx, y+dy, ink)
		}
	}
}

func (c *canvas) i420() []byte {
	out := make([]byte, 0, c.w*c.h*3/2)
	out = append(out, c.y...)
	out = append(out, c.cb...)
	return append(out, c.cr...)
}

func (c *canvas) desktop(seed int) {
	c.fill(0, 0, c.w, c.h, 58, 128, 118)
	for i := 0; i < 6; i++ {
		x := 24 + i*(c.w/7)
		c.fill(x, c.h-96, x+56, c.h-40, 92, 140, 110)
		c.border(x, c.h-96, x+56, c.h-40, 150)
		c.text(x+4, c.h-36, 6, seed+i, 210)
	}
	c.fill(0, 0, c.w, 34, 38, 128, 128)
	c.text(12, 12, 14, seed, 225)
	c.text(c.w-140, 12, 12, seed+3, 225)
}

func (c *canvas) window(x0, y0, x1, y1, title int) {
	c.fill(x0, y0, x1, y0+30, 74, 132, 122)
	c.text(x0+10, y0+10, 16, title, 235)
	c.fill(x0, y0+30, x1, y1, 235, 128, 128)
	c.border(x0, y0, x1, y1, 24)
}

func idleDesktopClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		c := newCanvas(w, h)
		c.desktop(1)
		c.window(w/8, h/8, w/8+w/2, h/8+h/2, 5)
		c.paragraph(w/8+16, h/8+48, w/2-32, (h/2-64)/glyphLead, 11, 40)
		c.cursor(w/2+i%3, h/2+i%2)
		out[i] = c.i420()
	}
	return out
}

func typingClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	x0, y0 := w/10, h/10
	x1, y1 := w-w/10, h-h/10
	perLine := (x1 - x0 - 32) / glyphAdvance
	for i := range out {
		c := newCanvas(w, h)
		c.desktop(1)
		c.window(x0, y0, x1, y1, 7)
		line, col := i*2/perLine, i*2%perLine
		for l := 0; l < line; l++ {
			c.text(x0+16, y0+48+l*glyphLead, perLine, 17+l, 40)
		}
		c.text(x0+16, y0+48+line*glyphLead, col, 17+line, 40)
		caret := x0 + 16 + col*glyphAdvance
		if i%8 < 4 {
			c.fill(caret, y0+48+line*glyphLead, caret+2, y0+48+line*glyphLead+glyphH, 24, 128, 128)
		}
		c.cursor(caret+4, y0+48+line*glyphLead)
		out[i] = c.i420()
	}
	return out
}

func scrollingTextClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	x0, y0 := w/12, h/12
	x1, y1 := w-w/12, h-h/12
	lines := (y1 - y0 - 64) / glyphLead
	perLine := (x1 - x0 - 32) / glyphAdvance
	for i := range out {
		c := newCanvas(w, h)
		c.desktop(1)
		c.window(x0, y0, x1, y1, 9)
		first, offset := i*11/glyphLead, i*11%glyphLead
		for l := 0; l <= lines; l++ {
			y := y0 + 44 + l*glyphLead - offset
			if y < y0+34 || y+glyphH > y1 {
				continue
			}
			c.text(x0+16, y, perLine-(first+l)%17, 23+first+l, 40)
		}
		c.cursor(x1-60, y0+90)
		out[i] = c.i420()
	}
	return out
}

func windowDragClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		c := newCanvas(w, h)
		c.desktop(1)
		c.window(w/16, h/3, w/16+w/3, h/3+h/3, 13)
		c.paragraph(w/16+16, h/3+48, w/3-32, (h/3-64)/glyphLead, 29, 40)
		dx := w/20 + i*13%(w/2)
		dy := h/12 + i*7%(h/3)
		c.window(dx, dy, dx+w/3, dy+h/3, 15)
		c.paragraph(dx+16, dy+48, w/3-32, (h/3-64)/glyphLead, 31, 40)
		c.cursor(dx+w/6, dy+14)
		out[i] = c.i420()
	}
	return out
}

func videoWindowClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	x0, y0 := w/6, h/6
	x1, y1 := w-w/6, h-h/6
	for i := range out {
		c := newCanvas(w, h)
		c.desktop(1)
		c.window(x0, y0, x1, y1, 17)
		cw := c.w / 2
		for y := y0 + 30; y < y1-1; y++ {
			row := c.y[y*c.w:]
			for x := x0 + 1; x < x1-1; x++ {
				u, v := x+i*9, y-i*5
				row[x] = byte(40 + (u*u/97+v*v/71+u/2+v*3+i*i/3)%200)
			}
		}
		for y := (y0 + 30) / 2; y < (y1-1)/2; y++ {
			for x := (x0 + 1) / 2; x < (x1-1)/2; x++ {
				u := 2*x + i*9
				c.cb[y*cw+x] = byte(70 + (u/3+y/2+i)%116)
				c.cr[y*cw+x] = byte(180 - (u/4+y/3+i*2)%116)
			}
		}
		c.cursor(x1+20, y1-40)
		out[i] = c.i420()
	}
	return out
}

func windowSwitchClip(w, h, n int) [][]byte {
	out := make([][]byte, n)
	for i := range out {
		c := newCanvas(w, h)
		switch i / 12 % 3 {
		case 0:
			c.desktop(1)
			c.window(w/8, h/8, w/8+w/2, h/8+h/2, 5)
			c.paragraph(w/8+16, h/8+48, w/2-32, (h/2-64)/glyphLead, 11, 40)
			c.cursor(w/2, h/2)
		case 1:
			c.desktop(41)
			c.window(w/5, h/6, w-w/5, h-h/6, 19)
			c.paragraph(w/5+16, h/6+48, w-2*(w/5)-32, (h-2*(h/6)-64)/glyphLead, 43, 40)
			c.cursor(w/3, h/3)
		default:
			c.desktop(77)
			c.window(w/20, h/20, w-w/20, h-h/20, 23)
			cw := c.w / 2
			for y := h/20 + 30; y < h-h/20-1; y++ {
				row := c.y[y*c.w:]
				for x := w/20 + 1; x < w-w/20-1; x++ {
					row[x] = byte(30 + (x*x/113+y*y/89+x/3)%190)
				}
			}
			for y := (h/20 + 30) / 2; y < (h-h/20-1)/2; y++ {
				for x := (w/20 + 1) / 2; x < (w-w/20-1)/2; x++ {
					c.cb[y*cw+x] = byte(60 + (x/2+y/3)%130)
					c.cr[y*cw+x] = byte(190 - (x/3+y/2)%130)
				}
			}
			c.cursor(2*w/3, 2*h/3)
		}
		out[i] = c.i420()
	}
	return out
}

type screencast struct {
	name   string
	frames func(w, h, n int) [][]byte
}

func screencastClips() []screencast {
	return []screencast{
		{"idle-desktop", idleDesktopClip},
		{"typing", typingClip},
		{"scrolling-text", scrollingTextClip},
		{"window-drag", windowDragClip},
		{"video-in-window", videoWindowClip},
		{"window-switch", windowSwitchClip},
	}
}

func ssimLuma(a, b []byte, w, h int) float64 {
	const c1 = 0.01 * 0.01 * 255 * 255 * 64
	const c2 = 0.03 * 0.03 * 255 * 255 * 64 * 63
	total, windows := 0.0, 0
	for y := 0; y+8 <= h; y += 4 {
		for x := 0; x+8 <= w; x += 4 {
			var s1, s2, ss1, ss2, s12 float64
			for dy := 0; dy < 8; dy++ {
				ra := a[(y+dy)*w+x:]
				rb := b[(y+dy)*w+x:]
				for dx := 0; dx < 8; dx++ {
					p, q := float64(ra[dx]), float64(rb[dx])
					s1 += p
					s2 += q
					ss1 += p * p
					ss2 += q * q
					s12 += p * q
				}
			}
			vars := (ss1+ss2)*64 - s1*s1 - s2*s2
			covar := s12*64 - s1*s2
			total += (2*s1*s2 + c1) * (2*covar + c2) /
				((s1*s1 + s2*s2 + c1) * (vars + c2))
			windows++
		}
	}
	if windows == 0 {
		return 1
	}
	return total / float64(windows)
}

func meanAndDeviation(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	mean := sum / float64(len(values))
	spread := 0.0
	for _, v := range values {
		spread += (v - mean) * (v - mean)
	}
	return mean, math.Sqrt(spread / float64(len(values)))
}
