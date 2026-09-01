package encoder

import (
	"errors"
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/oops1/go.264/internal/bits"
	"github.com/oops1/go.264/internal/frame"
	"github.com/oops1/go.264/internal/loopfilter"
	"github.com/oops1/go.264/internal/nal"
	"github.com/oops1/go.264/internal/syntax"
	"github.com/oops1/go.264/internal/transform"
)

var (
	ErrConfig    = errors.New("go264/encoder: invalid configuration")
	ErrFrameSize = errors.New("go264/encoder: frame does not match the configured size")
)

type Config struct {
	Width   int
	Height  int
	FPSNum  int
	FPSDen  int
	GOPSize int
	QP      int

	BitrateKbps int
	RefFrames   int
	BFrames     int

	CABAC         bool
	MotionSearch  MotionSearch
	ModeDecision  ModeDecision
	Slices        int
	Transform8x8  bool
	ScalingMatrix ScalingMatrix

	WeightedPrediction WeightedPrediction
	DirectMode         DirectMode

	IntraRefresh        int
	RepeatParameterSets bool

	Deblocking         DeblockMode
	DeblockAlphaOffset int
	DeblockBetaOffset  int

	VBVBufferKbits int
	VBVMaxrateKbps int
	CBR            bool
}

type DeblockMode uint8

const (
	DeblockingOn DeblockMode = iota
	DeblockingOff
	DeblockingNotAcrossSlices
)

type ModeDecision uint8

const (
	ModeDecisionFast ModeDecision = iota
	ModeDecisionExhaustive
)

func (c *Config) validate() error {
	if c.Width <= 0 || c.Height <= 0 {
		return fmt.Errorf("%w: size %dx%d", ErrConfig, c.Width, c.Height)
	}
	if c.Width%2 != 0 || c.Height%2 != 0 {
		return fmt.Errorf("%w: 4:2:0 requires even dimensions, got %dx%d", ErrConfig, c.Width, c.Height)
	}
	if c.Width > 16384 || c.Height > 16384 {
		return fmt.Errorf("%w: size %dx%d exceeds the supported maximum", ErrConfig, c.Width, c.Height)
	}
	if c.QP < 0 || c.QP > 51 {
		return fmt.Errorf("%w: QP %d outside 0..51", ErrConfig, c.QP)
	}
	if c.GOPSize <= 0 {
		c.GOPSize = 1
	}
	if c.FPSNum <= 0 || c.FPSDen <= 0 {
		c.FPSNum = 25
		c.FPSDen = 1
	}
	if c.RefFrames <= 0 {
		c.RefFrames = 1
	}
	if c.RefFrames > 16 {
		return fmt.Errorf("%w: RefFrames %d exceeds 16", ErrConfig, c.RefFrames)
	}
	if c.BFrames < 0 || c.BFrames > 7 {
		return fmt.Errorf("%w: BFrames %d outside 0..7", ErrConfig, c.BFrames)
	}
	if c.BFrames > 0 && c.RefFrames < 2 {
		c.RefFrames = 2
	}
	if c.IntraRefresh < 0 {
		return fmt.Errorf("%w: IntraRefresh %d is negative", ErrConfig, c.IntraRefresh)
	}
	if c.IntraRefresh > 0 && c.BFrames > 0 {
		return fmt.Errorf("%w: IntraRefresh cannot be combined with BFrames", ErrConfig)
	}
	if c.WeightedPrediction > WeightedPredictionImplicit {
		return fmt.Errorf("%w: WeightedPrediction %d outside 0..2", ErrConfig, c.WeightedPrediction)
	}
	if c.DirectMode > DirectTemporal {
		return fmt.Errorf("%w: DirectMode %d outside 0..1", ErrConfig, c.DirectMode)
	}
	if c.Deblocking > DeblockingNotAcrossSlices {
		return fmt.Errorf("%w: Deblocking %d outside 0..2", ErrConfig, c.Deblocking)
	}
	if c.DeblockAlphaOffset < -6 || c.DeblockAlphaOffset > 6 {
		return fmt.Errorf("%w: DeblockAlphaOffset %d outside -6..6", ErrConfig, c.DeblockAlphaOffset)
	}
	if c.DeblockBetaOffset < -6 || c.DeblockBetaOffset > 6 {
		return fmt.Errorf("%w: DeblockBetaOffset %d outside -6..6", ErrConfig, c.DeblockBetaOffset)
	}
	if c.VBVBufferKbits < 0 || c.VBVMaxrateKbps < 0 {
		return fmt.Errorf("%w: the buffer model cannot take negative sizes", ErrConfig)
	}
	if (c.VBVBufferKbits > 0) != (c.VBVMaxrateKbps > 0) {
		return fmt.Errorf("%w: VBVBufferKbits and VBVMaxrateKbps must be set together", ErrConfig)
	}
	if c.VBVMaxrateKbps > 0 {
		if c.BitrateKbps <= 0 {
			c.BitrateKbps = c.VBVMaxrateKbps
		}
		if c.BitrateKbps > c.VBVMaxrateKbps {
			return fmt.Errorf("%w: BitrateKbps %d exceeds VBVMaxrateKbps %d",
				ErrConfig, c.BitrateKbps, c.VBVMaxrateKbps)
		}
	}
	if c.CBR {
		if c.VBVMaxrateKbps <= 0 {
			return fmt.Errorf("%w: CBR needs VBVBufferKbits and VBVMaxrateKbps", ErrConfig)
		}
		c.BitrateKbps = c.VBVMaxrateKbps
	}
	return nil
}

type Encoder struct {
	cfg Config
	sps *syntax.SPS
	pps *syntax.PPS

	widthMBs  int
	heightMBs int

	level4x4 [6]scale4x4
	quant4x4 [6]scale4x4
	level8x8 [2]transform.LevelScale8x8
	quant8x8 [2]transform.QuantScale8x8

	src  *frame.Picture
	rec  *frame.Picture
	refs []*frame.Picture
	free []*frame.Picture

	refL0     []*frame.Picture
	refL1     []*frame.Picture
	colocated *frame.Picture

	grid []mbInfo

	rc         *rateControl
	frameNum   uint32
	frameIndex int
	headers    []byte
	forceKey   bool

	refresh        refreshPlan
	refreshPos     int
	refreshEnd     map[*frame.Picture]int
	refreshSeq     map[*frame.Picture]int
	refreshNextSeq int
	refreshBarrier int

	cpbFrame  int
	cpbAnchor int

	lastRec        *frame.Picture
	onPicture      func(display int, rec *frame.Picture)
	queue          []queuedFrame
	srcPool        []*frame.Picture
	displayIdx     int
	lastIDRDisplay int
}

type queuedFrame struct {
	pic     *frame.Picture
	hints   Hints
	display int
}

func New(cfg Config) (*Encoder, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	e := &Encoder{cfg: cfg}
	e.widthMBs = (cfg.Width + 15) / 16
	e.heightMBs = (cfg.Height + 15) / 16
	e.rc = newRateControl(cfg)
	e.buildParameterSets()
	e.buildScalingTables()
	e.src = frame.NewPicture(e.widthMBs, e.heightMBs)
	e.rec = frame.NewPicture(e.widthMBs, e.heightMBs)
	e.grid = make([]mbInfo, e.widthMBs*e.heightMBs)
	if cfg.IntraRefresh > 0 {
		e.refreshEnd = make(map[*frame.Picture]int)
		e.refreshSeq = make(map[*frame.Picture]int)
	}
	return e, nil
}

func (e *Encoder) SPS() *syntax.SPS { return e.sps }

func (e *Encoder) PPS() *syntax.PPS { return e.pps }

var levelLimits = []struct {
	level     uint8
	maxMBs    int
	maxMBS    int
	maxDpbMbs int
}{
	{10, 99, 1485, 396},
	{11, 396, 3000, 900},
	{12, 396, 6000, 2376},
	{13, 396, 11880, 2376},
	{20, 396, 11880, 2376},
	{21, 792, 19800, 4752},
	{22, 1620, 20250, 8100},
	{30, 1620, 40500, 8100},
	{31, 3600, 108000, 18000},
	{32, 5120, 216000, 20480},
	{40, 8192, 245760, 32768},
	{42, 8704, 522240, 34816},
	{50, 22080, 589824, 110400},
	{51, 36864, 983040, 184320},
	{52, 36864, 2073600, 184320},
}

func (e *Encoder) pickLevel() uint8 {
	frameMBs := e.widthMBs * e.heightMBs
	rate := frameMBs * e.cfg.FPSNum / e.cfg.FPSDen
	for _, l := range levelLimits {
		dpbFrames := l.maxDpbMbs / frameMBs
		if dpbFrames > 16 {
			dpbFrames = 16
		}
		if frameMBs <= l.maxMBs && rate <= l.maxMBS && e.cfg.RefFrames <= dpbFrames {
			return l.level
		}
	}
	return 52
}

func (e *Encoder) buildParameterSets() {
	profile := uint8(syntax.ProfileBaseline)
	constraints := uint8(0xC0)
	if e.cfg.CABAC || e.cfg.BFrames > 0 || e.cfg.WeightedPrediction != WeightedPredictionOff {
		profile = syntax.ProfileMain
		constraints = 0x40
	}
	if e.cfg.Transform8x8 || e.cfg.ScalingMatrix != ScalingMatrixFlat {
		profile = syntax.ProfileHigh
		constraints = 0
	}
	sps := &syntax.SPS{
		ProfileIDC:                profile,
		ConstraintSet:             constraints,
		LevelIDC:                  e.pickLevel(),
		ID:                        0,
		ChromaFormatIDC:           syntax.Chroma420,
		Log2MaxFrameNumMinus4:     4,
		PicOrderCntType:           2,
		MaxNumRefFrames:           uint32(e.cfg.RefFrames),
		PicWidthInMbsMinus1:       uint32(e.widthMBs - 1),
		PicHeightInMapUnitsMinus1: uint32(e.heightMBs - 1),
		FrameMbsOnly:              true,
		Direct8x8Inference:        true,
	}
	if e.cfg.BFrames > 0 {
		sps.PicOrderCntType = 0
		sps.Log2MaxPicOrderCntLsbMinus4 = 8
	}
	cropRight := (e.widthMBs*16 - e.cfg.Width) / 2
	cropBottom := (e.heightMBs*16 - e.cfg.Height) / 2
	if cropRight != 0 || cropBottom != 0 {
		sps.FrameCropping = true
		sps.FrameCropRightOffset = uint32(cropRight)
		sps.FrameCropBottomOffset = uint32(cropBottom)
	}
	sps.VUIPresent = true
	sps.VUI.TimingInfoPresent = true
	sps.VUI.NumUnitsInTick = uint32(e.cfg.FPSDen)
	sps.VUI.TimeScale = uint32(e.cfg.FPSNum) * 2
	sps.VUI.FixedFrameRate = true
	e.applyHRD(sps)

	pps := &syntax.PPS{
		ID:                             0,
		SPSID:                          0,
		NumRefIdxL0DefaultActiveMinus1: uint32(e.cfg.RefFrames - 1),
		NumRefIdxL1DefaultActiveMinus1: 0,
		PicInitQPMinus26:               int32(e.cfg.QP - 26),
		DeblockingFilterControlPresent: true,
		CABAC:                          e.cfg.CABAC,
	}
	switch e.cfg.WeightedPrediction {
	case WeightedPredictionExplicit:
		pps.WeightedPred = true
	case WeightedPredictionImplicit:
		pps.WeightedPred = true
		if e.cfg.BFrames > 0 {
			pps.WeightedBipredIDC = 2
		}
	}
	if sps.ProfileIDC == syntax.ProfileHigh {
		sps.BitDepthLumaMinus8 = 0
		sps.BitDepthChromaMinus8 = 0
		sps.QpprimeYZeroTransformBypass = false
		pps.HasExtension = true
		pps.Transform8x8Mode = e.cfg.Transform8x8
	}
	if e.cfg.ScalingMatrix == ScalingMatrixJVT {
		pps.PicScalingMatrixPresent = true
		for i := 0; i < 6; i++ {
			pps.ScalingList4x4Present[i] = true
			pps.UseDefaultScaling4x4[i] = true
		}
		if pps.Transform8x8Mode {
			for i := 0; i < 2; i++ {
				pps.ScalingList8x8Present[i] = true
				pps.UseDefaultScaling8x8[i] = true
			}
		}
	}
	e.sps = sps
	e.pps = pps
}

func (e *Encoder) parameterSetBytes() ([]byte, error) {
	if e.headers != nil {
		return e.headers, nil
	}
	spsRBSP, err := syntax.WriteSPS(e.sps)
	if err != nil {
		return nil, err
	}
	ppsRBSP, err := syntax.WritePPS(e.pps, func(uint32) *syntax.SPS { return e.sps })
	if err != nil {
		return nil, err
	}
	out := nal.AppendAnnexB(nil, nal.Unit{
		Header: nal.Header{RefIDC: 3, Type: nal.TypeSPS},
		RBSP:   spsRBSP,
	}, true)
	out = nal.AppendAnnexB(out, nal.Unit{
		Header: nal.Header{RefIDC: 3, Type: nal.TypePPS},
		RBSP:   ppsRBSP,
	}, true)
	e.headers = out
	return out, nil
}

func (e *Encoder) Headers() ([]byte, error) { return e.parameterSetBytes() }

func (e *Encoder) Encode(yuv []byte) ([]byte, error) {
	return e.EncodeWithHints(yuv, Hints{})
}

func (e *Encoder) ForceKeyFrame() { e.forceKey = true }

func (e *Encoder) EncodeWithHints(yuv []byte, h Hints) ([]byte, error) {
	want := e.cfg.Width * e.cfg.Height * 3 / 2
	if len(yuv) != want {
		return nil, fmt.Errorf("%w: got %d bytes, want %d", ErrFrameSize, len(yuv), want)
	}
	if e.cfg.BFrames > 0 {
		return e.encodeReordered(yuv, h)
	}
	e.loadSourceInto(e.src, yuv)

	idr := e.frameIndex%e.cfg.GOPSize == 0 || e.forceKey
	if e.cfg.IntraRefresh > 0 {
		idr = e.forceKey
	}
	e.forceKey = false
	if len(e.refs) == 0 {
		idr = true
	}
	sliceType := syntax.SliceP
	if idr {
		sliceType = syntax.SliceI
	}
	out, err := e.encodePicture(picture{
		src:       e.src,
		hints:     h,
		poc:       2 * e.frameIndex,
		display:   e.frameIndex,
		sliceType: sliceType,
		idr:       idr,
		reference: true,
	})
	if err != nil {
		return nil, err
	}
	e.frameIndex++
	return out, nil
}

func (e *Encoder) Flush() ([]byte, error) {
	if e.cfg.BFrames == 0 || len(e.queue) == 0 {
		return nil, nil
	}
	return e.drainQueueAsP()
}

type picture struct {
	src       *frame.Picture
	hints     Hints
	poc       int
	display   int
	sliceType syntax.SliceType
	idr       bool
	reference bool
}

func (e *Encoder) acquireSource() *frame.Picture {
	if n := len(e.srcPool); n > 0 {
		p := e.srcPool[n-1]
		e.srcPool = e.srcPool[:n-1]
		return p
	}
	return frame.NewPicture(e.widthMBs, e.heightMBs)
}

func (e *Encoder) encodeQueued(q queuedFrame, sliceType syntax.SliceType, idr bool) ([]byte, error) {
	out, err := e.encodePicture(picture{
		src:       q.pic,
		hints:     q.hints,
		poc:       2 * (q.display - e.lastIDRDisplay),
		display:   q.display,
		sliceType: sliceType,
		idr:       idr,
		reference: !sliceType.IsB(),
	})
	e.srcPool = append(e.srcPool, q.pic)
	return out, err
}

func (e *Encoder) drainQueueAsP() ([]byte, error) {
	var out []byte
	for _, q := range e.queue {
		pkt, err := e.encodeQueued(q, syntax.SliceP, false)
		if err != nil {
			return nil, err
		}
		out = append(out, pkt...)
	}
	e.queue = e.queue[:0]
	return out, nil
}

func (e *Encoder) encodeReordered(yuv []byte, h Hints) ([]byte, error) {
	display := e.displayIdx
	e.displayIdx++
	idr := display%e.cfg.GOPSize == 0 || e.forceKey
	e.forceKey = false
	if len(e.refs) == 0 {
		idr = true
	}
	q := queuedFrame{pic: e.acquireSource(), hints: h, display: display}
	e.loadSourceInto(q.pic, yuv)

	if idr {
		out, err := e.drainQueueAsP()
		if err != nil {
			return nil, err
		}
		e.lastIDRDisplay = display
		pkt, err := e.encodeQueued(q, syntax.SliceI, true)
		if err != nil {
			return nil, err
		}
		return append(out, pkt...), nil
	}

	e.queue = append(e.queue, q)
	if len(e.queue) < e.cfg.BFrames+1 {
		return nil, nil
	}
	last := len(e.queue) - 1
	out, err := e.encodeQueued(e.queue[last], syntax.SliceP, false)
	if err != nil {
		return nil, err
	}
	for i := 0; i < last; i++ {
		pkt, err := e.encodeQueued(e.queue[i], syntax.SliceB, false)
		if err != nil {
			return nil, err
		}
		out = append(out, pkt...)
	}
	e.queue = e.queue[:0]
	return out, nil
}

func (e *Encoder) setReferenceLists(p picture) {
	e.refL0 = e.refL0[:0]
	e.refL1 = e.refL1[:0]
	e.colocated = nil
	if p.idr {
		return
	}
	if !p.sliceType.IsB() {
		e.refL0 = append(e.refL0, e.refs[:e.activeRefs()]...)
		return
	}
	var before, after []*frame.Picture
	for _, r := range e.refs {
		if r.POC < p.poc {
			before = append(before, r)
		} else {
			after = append(after, r)
		}
	}
	sort.SliceStable(before, func(i, j int) bool { return before[i].POC > before[j].POC })
	sort.SliceStable(after, func(i, j int) bool { return after[i].POC < after[j].POC })
	if len(before) != 0 {
		e.refL0 = append(e.refL0, before[0])
	} else if len(after) != 0 {
		e.refL0 = append(e.refL0, after[0])
	}
	if len(after) != 0 {
		e.refL1 = append(e.refL1, after[0])
	} else if len(before) != 0 {
		e.refL1 = append(e.refL1, before[0])
	}
	if len(e.refL1) != 0 {
		e.colocated = e.refL1[0]
	}
}

func (e *Encoder) motionField() *frame.Motion {
	mo := frame.NewMotion(e.widthMBs, e.heightMBs)
	for mby := 0; mby < e.heightMBs; mby++ {
		for mbx := 0; mbx < e.widthMBs; mbx++ {
			m := e.at(mbx, mby)
			if m == nil || m.Intra {
				continue
			}
			for blk := 0; blk < 16; blk++ {
				i := mo.Index(mbx*4+blockX[blk]/4, mby*4+blockY[blk]/4)
				if r := m.refIdx[blk]; r >= 0 {
					mo.Mv[0][i] = m.MvL0[blk]
					mo.RefIdx[0][i] = r
					if p := m.RefPicL0[blk]; p != nil {
						mo.RefPOC[0][i] = int32(p.POC)
					}
				}
				if r := m.refIdxL1[blk]; r >= 0 {
					mo.Mv[1][i] = m.MvL1[blk]
					mo.RefIdx[1][i] = r
					if p := m.RefPicL1[blk]; p != nil {
						mo.RefPOC[1][i] = int32(p.POC)
					}
				}
			}
		}
	}
	return mo
}

func (e *Encoder) encodePicture(p picture) ([]byte, error) {
	e.src = p.src
	if p.idr {
		e.frameNum = 0
		e.free = append(e.free, e.refs...)
		e.refs = e.refs[:0]
	}
	e.refresh = e.planRefresh(p.idr)
	if e.refresh.sweep {
		e.openSweep()
	}
	var prefix []byte
	if p.idr || (e.cfg.RepeatParameterSets && e.refresh.sweep) {
		hdrs, err := e.parameterSetBytes()
		if err != nil {
			return nil, err
		}
		prefix = append(prefix, hdrs...)
	}
	sei, err := e.pictureSEI(p.idr || e.refresh.sweep, e.refresh.sweep)
	if err != nil {
		return nil, err
	}
	prefix = append(prefix, sei...)
	hints := e.prepareHints(p.hints)
	if p.idr {
		hints = nil
	}
	e.setReferenceLists(p)

	nalType := nal.TypeSliceNonIDR
	if p.idr {
		nalType = nal.TypeSliceIDR
	}
	refIDC := uint8(1)
	if !p.reference {
		refIDC = 0
	}

	qp := e.rc.frameQP(p.idr)
	active := len(e.refL0)
	if active < 1 {
		active = 1
	}
	activeL1 := len(e.refL1)
	if activeL1 < 1 {
		activeL1 = 1
	}
	e.rec.POC = p.poc
	e.rec.FrameNum = e.frameNum
	e.rec.IDR = p.idr
	e.rec.LongTerm = false

	bounds := e.sliceBounds()
	reference := p.reference
	dropped := false
	var out []byte
	for attempt := 0; ; attempt++ {
		for i := range e.grid {
			e.grid[i] = mbInfo{}
		}
		jobs := make([]sliceJob, len(bounds))
		for i, b := range bounds {
			jobs[i] = sliceJob{id: i, count: len(bounds), firstMB: b[0], endMB: b[1],
				sliceType: p.sliceType, qp: qp, active: active, activeL1: activeL1,
				idr: p.idr, refIDC: refIDC, poc: p.poc, skipAll: dropped}
		}
		payloads, err := e.encodeSlices(jobs, hints)
		if err != nil {
			return nil, err
		}
		out = append(out[:0], prefix...)
		for _, rbsp := range payloads {
			out = nal.AppendAnnexB(out, nal.Unit{
				Header: nal.Header{RefIDC: refIDC, Type: nalType},
				RBSP:   rbsp,
			}, true)
		}
		if !e.rc.overBudget(len(out)*8) || attempt >= 8 {
			break
		}
		if qp < 51 {
			qp = e.rc.shrinkQP(len(out)*8, qp, attempt)
			continue
		}
		if dropped || !e.mayDropPicture(p) {
			break
		}
		dropped = true
		reference = false
		refIDC = 0
	}
	if n := e.rc.padBytes(len(out) * 8); n > 0 {
		out = appendFiller(out, n-fillerOverhead)
	}

	loopfilter.Apply(e.rec, e.widthMBs, e.heightMBs, func(mbx, mby int) *loopfilter.MB {
		m := e.at(mbx, mby)
		if m == nil {
			return nil
		}
		return &m.MB
	})
	e.rec.ExtendBorders()
	e.lastRec = e.rec
	if e.onPicture != nil {
		e.onPicture(p.display, e.rec)
	}
	if reference {
		e.rec.Motion = e.motionField()
		e.recordRefreshEnd(e.rec, e.refresh)
		e.rotateReferences()
		e.frameNum = (e.frameNum + 1) % e.sps.MaxFrameNum()
	} else {
		e.rec.Motion = nil
	}
	e.rc.update(len(out)*8, qp, p.idr, dropped)
	e.cpbFrame++
	return out, nil
}

func (e *Encoder) sliceBounds() [][2]int {
	n := e.cfg.Slices
	if n < 0 {
		n = runtime.GOMAXPROCS(0)
	}
	total := e.widthMBs * e.heightMBs
	if n <= 1 || e.heightMBs < 2 {
		return [][2]int{{0, total}}
	}
	if n > e.heightMBs {
		n = e.heightMBs
	}
	bounds := make([][2]int, 0, n)
	for i := 0; i < n; i++ {
		firstRow := i * e.heightMBs / n
		endRow := (i + 1) * e.heightMBs / n
		if firstRow == endRow {
			continue
		}
		bounds = append(bounds, [2]int{firstRow * e.widthMBs, endRow * e.widthMBs})
	}
	return bounds
}

func (e *Encoder) sliceHeader(p sliceJob) *syntax.SliceHeader {
	hdr := &syntax.SliceHeader{
		FirstMBInSlice:             uint32(p.firstMB),
		SliceType:                  p.sliceType + 5,
		PPSID:                      0,
		FrameNum:                   e.frameNum,
		IDR:                        p.idr,
		NalRefIDC:                  p.refIDC,
		SliceQPDelta:               int32(p.qp - e.cfg.QP),
		DisableDeblockingFilterIDC: uint32(e.cfg.Deblocking),
		SliceAlphaC0OffsetDiv2:     int32(e.cfg.DeblockAlphaOffset),
		SliceBetaOffsetDiv2:        int32(e.cfg.DeblockBetaOffset),
	}
	if e.cfg.Deblocking == DeblockingOff {
		hdr.SliceAlphaC0OffsetDiv2 = 0
		hdr.SliceBetaOffsetDiv2 = 0
	}
	if e.sps.PicOrderCntType == 0 {
		hdr.PicOrderCntLsb = uint32(p.poc) & (e.sps.MaxPicOrderCntLsb() - 1)
	}
	switch {
	case p.sliceType.IsB():
		hdr.DirectSpatialMvPred = e.cfg.DirectMode != DirectTemporal
		hdr.NumRefIdxActiveOverride = true
		hdr.NumRefIdxL0ActiveMinus1 = uint32(p.active - 1)
		hdr.NumRefIdxL1ActiveMinus1 = uint32(p.activeL1 - 1)
	case !p.idr && p.active != e.cfg.RefFrames:
		hdr.NumRefIdxActiveOverride = true
		hdr.NumRefIdxL0ActiveMinus1 = uint32(p.active - 1)
	}
	if e.weightModeFor(p.sliceType) == weightExplicit {
		e.fillPredWeightTable(hdr, p)
	}
	return hdr
}

func (e *Encoder) encodeOneSlice(p sliceJob, hints *frameHints) ([]byte, error) {
	hdr := e.sliceHeader(p)
	w := bits.NewWriterSize(e.cfg.Width*e.cfg.Height/2/p.count + 64)
	if err := syntax.WriteSliceHeader(w, hdr, e.sps, e.pps); err != nil {
		return nil, err
	}
	if err := e.encodeSlice(w, hdr, p, hints); err != nil {
		return nil, err
	}
	w.WriteRBSPTrailingBits()
	if err := w.Err(); err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

type sliceJob struct {
	id        int
	count     int
	firstMB   int
	endMB     int
	sliceType syntax.SliceType
	qp        int
	active    int
	activeL1  int
	idr       bool
	refIDC    uint8
	poc       int
	skipAll   bool
}

func (e *Encoder) mayDropPicture(p picture) bool {
	return e.rc.vbv && e.cfg.IntraRefresh == 0 && !p.idr &&
		p.sliceType.IsP() && len(e.refL0) != 0
}

func (e *Encoder) encodeSlices(jobs []sliceJob, hints *frameHints) ([][]byte, error) {
	payloads := make([][]byte, len(jobs))
	if len(jobs) == 1 {
		rbsp, err := e.encodeOneSlice(jobs[0], hints)
		if err != nil {
			return nil, err
		}
		payloads[0] = rbsp
		return payloads, nil
	}
	errs := make([]error, len(jobs))
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(i int, job sliceJob) {
			defer wg.Done()
			payloads[i], errs[i] = e.encodeOneSlice(job, hints)
		}(i, job)
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return payloads, nil
}

func (e *Encoder) loadSourceInto(dst *frame.Picture, yuv []byte) {
	w, h := e.cfg.Width, e.cfg.Height
	pw, ph := e.widthMBs*16, e.heightMBs*16
	n := 0
	for y := 0; y < h; y++ {
		row := dst.Y[dst.LumaOffset(0, y):]
		copy(row[:w], yuv[n:n+w])
		for x := w; x < pw; x++ {
			row[x] = row[w-1]
		}
		n += w
	}
	for y := h; y < ph; y++ {
		copy(dst.Y[dst.LumaOffset(0, y):dst.LumaOffset(0, y)+pw],
			dst.Y[dst.LumaOffset(0, h-1):dst.LumaOffset(0, h-1)+pw])
	}
	cw, ch := w/2, h/2
	pcw, pch := pw/2, ph/2
	for _, plane := range [][]byte{dst.Cb, dst.Cr} {
		for y := 0; y < ch; y++ {
			row := plane[dst.ChromaOffset(0, y):]
			copy(row[:cw], yuv[n:n+cw])
			for x := cw; x < pcw; x++ {
				row[x] = row[cw-1]
			}
			n += cw
		}
		for y := ch; y < pch; y++ {
			copy(plane[dst.ChromaOffset(0, y):dst.ChromaOffset(0, y)+pcw],
				plane[dst.ChromaOffset(0, ch-1):dst.ChromaOffset(0, ch-1)+pcw])
		}
	}
}

func (e *Encoder) rotateReferences() {
	e.refs = append([]*frame.Picture{e.rec}, e.refs...)
	for len(e.refs) > e.cfg.RefFrames {
		e.free = append(e.free, e.refs[len(e.refs)-1])
		e.refs = e.refs[:len(e.refs)-1]
	}
	if n := len(e.free); n > 0 {
		e.rec = e.free[n-1]
		e.free = e.free[:n-1]
		return
	}
	e.rec = frame.NewPicture(e.widthMBs, e.heightMBs)
}

func (e *Encoder) activeRefs() int {
	if len(e.refs) < e.cfg.RefFrames {
		return len(e.refs)
	}
	return e.cfg.RefFrames
}

func (e *Encoder) width() int { return e.widthMBs * 16 }

func (e *Encoder) height() int { return e.heightMBs * 16 }
