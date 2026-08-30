package decoder

type Picture struct {
	Y       []byte
	Cb      []byte
	Cr      []byte
	StrideY int
	StrideC int
	Width   int
	Height  int

	POC      int
	FrameNum uint32
	IDR      bool

	widthMBs  int
	heightMBs int
}

const lumaMargin = 32

func NewPicture(widthMBs, heightMBs int) *Picture {
	w := widthMBs * 16
	h := heightMBs * 16
	strideY := w + 2*lumaMargin
	strideC := w/2 + lumaMargin
	p := &Picture{
		StrideY:   strideY,
		StrideC:   strideC,
		Width:     w,
		Height:    h,
		widthMBs:  widthMBs,
		heightMBs: heightMBs,
	}
	p.Y = make([]byte, strideY*(h+2*lumaMargin))
	p.Cb = make([]byte, strideC*(h/2+lumaMargin))
	p.Cr = make([]byte, strideC*(h/2+lumaMargin))
	return p
}

func (p *Picture) OriginY() int { return lumaMargin*p.StrideY + lumaMargin }

func (p *Picture) OriginC() int { return lumaMargin/2*p.StrideC + lumaMargin/2 }

func (p *Picture) LumaOffset(x, y int) int { return p.OriginY() + y*p.StrideY + x }

func (p *Picture) ChromaOffset(x, y int) int { return p.OriginC() + y*p.StrideC + x }

func (p *Picture) CopyOut(dst []byte) {
	n := 0
	for y := 0; y < p.Height; y++ {
		copy(dst[n:n+p.Width], p.Y[p.LumaOffset(0, y):])
		n += p.Width
	}
	cw := p.Width / 2
	ch := p.Height / 2
	for _, plane := range [][]byte{p.Cb, p.Cr} {
		for y := 0; y < ch; y++ {
			copy(dst[n:n+cw], plane[p.ChromaOffset(0, y):])
			n += cw
		}
	}
}

func (p *Picture) Size() int { return p.Width * p.Height * 3 / 2 }
