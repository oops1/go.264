package nvenc

import (
	"errors"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const encodeLibraryName = "libnvidia-encode.so.1"

var (
	libraryOnce sync.Once
	libraryErr  error

	createInstance    func(*NvEncodeAPIFunctionList) NvEncStatus
	maxSupportedQuery func(*uint32) NvEncStatus
)

func loadEncodeLibrary() error {
	libraryOnce.Do(func() {
		h, err := purego.Dlopen(encodeLibraryName, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			libraryErr = errors.New("nvenc: " + encodeLibraryName + " is not available: " + err.Error())
			return
		}
		purego.RegisterLibFunc(&createInstance, h, "NvEncodeAPICreateInstance")
		purego.RegisterLibFunc(&maxSupportedQuery, h, "NvEncodeAPIGetMaxSupportedVersion")
	})
	return libraryErr
}

func Available() bool { return loadEncodeLibrary() == nil && CUDAAvailable() }

func MaxSupportedVersion() (major, minor uint32, err error) {
	if err := loadEncodeLibrary(); err != nil {
		return 0, 0, err
	}
	var v uint32
	if st := maxSupportedQuery(&v); st != NvEncSuccess {
		return 0, 0, st
	}
	return v >> 4, v & 0xF, nil
}

type Profile uint32

const (
	ProfileBaseline Profile = iota
	ProfileMain
	ProfileHigh
)

func (p Profile) guid() GUID {
	switch p {
	case ProfileBaseline:
		return NvEncH264ProfileBaselineGUID
	case ProfileHigh:
		return NvEncH264ProfileHighGUID
	}
	return NvEncH264ProfileMainGUID
}

type Config struct {
	Width                int
	Height               int
	FPSNum               int
	FPSDen               int
	BitrateBitsPerSecond int
	GOPLength            int
	Profile              Profile
	DeviceOrdinal        int
}

func (c Config) valid() error {
	switch {
	case c.Width <= 0 || c.Height <= 0 || c.Width%2 != 0 || c.Height%2 != 0:
		return errors.New("nvenc: the picture must have even positive dimensions")
	case c.FPSNum <= 0 || c.FPSDen <= 0:
		return errors.New("nvenc: the frame rate must be positive")
	}
	return nil
}

type call struct {
	fn   func()
	done chan struct{}
}

type encoderThread struct {
	calls chan call
	dead  chan struct{}
}

func newEncoderThread() *encoderThread {
	t := &encoderThread{calls: make(chan call), dead: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		defer close(t.dead)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		close(ready)
		for c := range t.calls {
			c.fn()
			close(c.done)
		}
	}()
	<-ready
	return t
}

func (t *encoderThread) run(fn func()) {
	if t == nil || t.calls == nil {
		fn()
		return
	}
	done := make(chan struct{})
	t.calls <- call{fn: fn, done: done}
	<-done
}

func (t *encoderThread) stop() {
	if t == nil || t.calls == nil {
		return
	}
	close(t.calls)
	t.calls = nil
	<-t.dead
}

type Encoder struct {
	thread  *encoderThread
	context Context
	device  Device
	name    string

	fns     NvEncodeAPIFunctionList
	session uintptr

	input  uintptr
	output uintptr

	width  int
	height int
	nv12   []byte

	timestamp uint64
	duration  uint64

	drained bool
	closed  bool
}

func (e *Encoder) Name() string { return e.name }

func Open(cfg Config) (*Encoder, error) {
	if err := cfg.valid(); err != nil {
		return nil, err
	}
	if err := loadEncodeLibrary(); err != nil {
		return nil, err
	}
	if err := LoadCUDA(); err != nil {
		return nil, err
	}

	thread := newEncoderThread()
	var enc *Encoder
	var err error
	thread.run(func() { enc, err = openHere(cfg) })
	if err != nil {
		thread.stop()
		return nil, err
	}
	enc.thread = thread
	return enc, nil
}

func openHere(cfg Config) (*Encoder, error) {
	device, err := OpenDevice(cfg.DeviceOrdinal)
	if err != nil {
		return nil, err
	}
	name, err := DeviceName(device)
	if err != nil {
		return nil, err
	}
	context, err := CreateContext(device)
	if err != nil {
		return nil, err
	}

	e := &Encoder{
		context: context,
		device:  device,
		name:    name,
		width:   cfg.Width,
		height:  cfg.Height,
	}
	if err := e.configure(cfg); err != nil {
		e.releaseHere()
		return nil, err
	}
	return e, nil
}

type callFailure struct {
	Op     string
	Status NvEncStatus
}

func (f *callFailure) Error() string { return "nvenc: " + f.Op + ": " + f.Status.Error() }

func (f *callFailure) Unwrap() error { return f.Status }

func (e *Encoder) fn(entry uintptr, name string) (func(...uintptr) error, error) {
	if entry == 0 {
		return nil, errors.New("nvenc: the driver exposes no " + name)
	}
	return func(args ...uintptr) error {
		r, _, _ := purego.SyscallN(entry, args...)
		if st := NvEncStatus(int32(uint32(r))); st != NvEncSuccess {
			return &callFailure{Op: name, Status: st}
		}
		return nil
	}, nil
}

func guidWords(g *GUID) (uintptr, uintptr) {
	p := (*[2]uint64)(unsafe.Pointer(g))
	return uintptr(p[0]), uintptr(p[1])
}

func (e *Encoder) configure(cfg Config) error {
	e.fns.Version = NvEncodeAPIFunctionListVersion
	if st := createInstance(&e.fns); st != NvEncSuccess {
		return st
	}

	openSession, err := e.fn(e.fns.NvEncOpenEncodeSessionEx, "session entry point")
	if err != nil {
		return err
	}
	params := NvEncOpenEncodeSessionExParams{
		Version:    NvEncOpenEncodeSessionExParamsVersion,
		DeviceType: NvEncDeviceTypeCUDA,
		Device:     uintptr(e.context),
		APIVersion: NvEncAPIVersion,
	}
	if err := openSession(uintptr(unsafe.Pointer(&params)), uintptr(unsafe.Pointer(&e.session))); err != nil {
		return err
	}

	preset, err := e.presetConfig()
	if err != nil {
		return err
	}
	e.applyRateControl(&preset.PresetCfg, cfg)

	initialize := NvEncInitializeParams{
		Version:      NvEncInitializeParamsVersion,
		EncodeGUID:   NvEncCodecH264GUID,
		PresetGUID:   NvEncPresetP4GUID,
		EncodeWidth:  uint32(cfg.Width),
		EncodeHeight: uint32(cfg.Height),
		DarWidth:     uint32(cfg.Width),
		DarHeight:    uint32(cfg.Height),
		FrameRateNum: uint32(cfg.FPSNum),
		FrameRateDen: uint32(cfg.FPSDen),
		EnablePTD:    1,
		EncodeConfig: uintptr(unsafe.Pointer(&preset.PresetCfg)),
		TuningInfo:   NvEncTuningInfoHighQuality,
		BufferFormat: NvEncBufferFormatNV12,
	}
	initEncoder, err := e.fn(e.fns.NvEncInitializeEncoder, "initialisation entry point")
	if err != nil {
		return err
	}
	if err := initEncoder(e.session, uintptr(unsafe.Pointer(&initialize))); err != nil {
		return err
	}
	runtime.KeepAlive(&preset)

	if err := e.createBuffers(); err != nil {
		return err
	}
	e.nv12 = make([]byte, NV12Size(cfg.Width, cfg.Height))
	e.duration = uint64(cfg.FPSDen) * 1000 / uint64(cfg.FPSNum)
	return nil
}

func (e *Encoder) presetConfig() (*NvEncPresetConfig, error) {
	getPreset, err := e.fn(e.fns.NvEncGetEncodePresetConfigEx, "preset entry point")
	if err != nil {
		return nil, err
	}
	preset := &NvEncPresetConfig{Version: NvEncPresetConfigVersion}
	preset.PresetCfg.Version = NvEncConfigVersion
	codecLow, codecHigh := guidWords(&NvEncCodecH264GUID)
	presetLow, presetHigh := guidWords(&NvEncPresetP4GUID)
	if err := getPreset(e.session,
		codecLow, codecHigh,
		presetLow, presetHigh,
		uintptr(NvEncTuningInfoHighQuality),
		uintptr(unsafe.Pointer(preset))); err != nil {
		return nil, err
	}
	return preset, nil
}

func (e *Encoder) applyRateControl(c *NvEncConfig, cfg Config) {
	c.Version = NvEncConfigVersion
	c.ProfileGUID = cfg.Profile.guid()
	if cfg.GOPLength > 0 {
		c.GopLength = uint32(cfg.GOPLength)
	}
	c.FrameIntervalP = 1
	c.RCParams.Version = NvEncRCParamsVersion
	if cfg.BitrateBitsPerSecond > 0 {
		c.RCParams.RateControlMode = NvEncParamsRCVBR
		c.RCParams.AverageBitRate = uint32(cfg.BitrateBitsPerSecond)
		c.RCParams.MaxBitRate = uint32(cfg.BitrateBitsPerSecond) * 2
	}
	h264 := c.EncodeCodecConfig.H264Config()
	h264.IDRPeriod = c.GopLength
	h264.SetOutputAUD(false)
	h264.SetRepeatSPSPPS(true)
	h264.SetDisableSPSPPS(false)
}

func (e *Encoder) createBuffers() error {
	createInput, err := e.fn(e.fns.NvEncCreateInputBuffer, "input buffer entry point")
	if err != nil {
		return err
	}
	in := NvEncCreateInputBuffer{
		Version:   NvEncCreateInputBufferVersion,
		Width:     uint32(e.width),
		Height:    uint32(e.height),
		BufferFmt: NvEncBufferFormatNV12,
	}
	if err := createInput(e.session, uintptr(unsafe.Pointer(&in))); err != nil {
		return err
	}
	e.input = in.InputBuffer

	createOutput, err := e.fn(e.fns.NvEncCreateBitstreamBuffer, "bitstream buffer entry point")
	if err != nil {
		return err
	}
	out := NvEncCreateBitstreamBuffer{Version: NvEncCreateBitstreamBufferVersion}
	if err := createOutput(e.session, uintptr(unsafe.Pointer(&out))); err != nil {
		return err
	}
	e.output = out.BitstreamBuffer
	return nil
}

func (e *Encoder) writeFrame(i420 []byte) error {
	lock, err := e.fn(e.fns.NvEncLockInputBuffer, "input lock entry point")
	if err != nil {
		return err
	}
	unlock, err := e.fn(e.fns.NvEncUnlockInputBuffer, "input unlock entry point")
	if err != nil {
		return err
	}
	locked := NvEncLockInputBuffer{Version: NvEncLockInputBufferVersion, InputBuffer: e.input}
	if err := lock(e.session, uintptr(unsafe.Pointer(&locked))); err != nil {
		return err
	}
	pitch := int(locked.Pitch)
	if pitch < e.width {
		pitch = e.width
	}
	need := NV12Size(pitch, e.height)
	if len(e.nv12) < need {
		e.nv12 = make([]byte, need)
	}
	I420ToNV12(e.nv12, pitch, i420, e.width, e.height)
	dst := unsafe.Slice((*byte)(locked.BufferDataPtr), need)
	copy(dst, e.nv12[:need])
	return unlock(e.session, e.input)
}

func (e *Encoder) readBitstream() ([]byte, error) {
	lock, err := e.fn(e.fns.NvEncLockBitstream, "bitstream lock entry point")
	if err != nil {
		return nil, err
	}
	unlock, err := e.fn(e.fns.NvEncUnlockBitstream, "bitstream unlock entry point")
	if err != nil {
		return nil, err
	}
	locked := NvEncLockBitstream{Version: NvEncLockBitstreamVersion, OutputBitstream: e.output}
	if err := lock(e.session, uintptr(unsafe.Pointer(&locked))); err != nil {
		return nil, err
	}
	n := int(locked.BitstreamSizeInBytes)
	var out []byte
	if n > 0 && locked.BitstreamBufferPtr != nil {
		out = make([]byte, n)
		copy(out, unsafe.Slice((*byte)(locked.BitstreamBufferPtr), n))
	}
	return out, unlock(e.session, e.output)
}

func (e *Encoder) submit(flags uint32, withInput bool) error {
	encode, err := e.fn(e.fns.NvEncEncodePicture, "encode entry point")
	if err != nil {
		return err
	}
	pic := NvEncPicParams{
		Version:         NvEncPicParamsVersion,
		EncodePicFlags:  flags,
		OutputBitstream: e.output,
		InputTimeStamp:  e.timestamp,
		InputDuration:   e.duration,
	}
	if withInput {
		pic.InputWidth = uint32(e.width)
		pic.InputHeight = uint32(e.height)
		pic.InputBuffer = e.input
		pic.BufferFmt = NvEncBufferFormatNV12
		pic.PictureStruct = NvEncPicStructFrame
	}
	return encode(e.session, uintptr(unsafe.Pointer(&pic)))
}

func (e *Encoder) encodeHere(i420 []byte) ([]byte, error) {
	if err := e.writeFrame(i420); err != nil {
		return nil, err
	}
	err := e.submit(0, true)
	e.timestamp += e.duration
	if err == nil {
		return e.readBitstream()
	}
	var failure *callFailure
	if errors.As(err, &failure) && failure.Status == NvEncErrNeedMoreInput {
		return nil, nil
	}
	return nil, err
}

func (e *Encoder) Encode(i420 []byte) (out []byte, err error) {
	if e.closed {
		return nil, errors.New("nvenc: encode on a closed encoder")
	}
	if len(i420) < I420Size(e.width, e.height) {
		return nil, errors.New("nvenc: the frame is shorter than the configured picture")
	}
	e.thread.run(func() { out, err = e.encodeHere(i420) })
	return out, err
}

func (e *Encoder) drainHere() ([]byte, error) {
	if e.drained {
		return nil, nil
	}
	e.drained = true
	if err := e.submit(uint32(NvEncPicFlagEOS), false); err != nil {
		return nil, err
	}
	return e.readBitstream()
}

func (e *Encoder) Drain() (out []byte, err error) {
	if e.closed {
		return nil, nil
	}
	e.thread.run(func() { out, err = e.drainHere() })
	return out, err
}

func (e *Encoder) releaseHere() {
	if e.session != 0 {
		if destroyInput, err := e.fn(e.fns.NvEncDestroyInputBuffer, "input release"); err == nil && e.input != 0 {
			destroyInput(e.session, e.input)
			e.input = 0
		}
		if destroyOutput, err := e.fn(e.fns.NvEncDestroyBitstreamBuffer, "bitstream release"); err == nil && e.output != 0 {
			destroyOutput(e.session, e.output)
			e.output = 0
		}
		if destroy, err := e.fn(e.fns.NvEncDestroyEncoder, "encoder release"); err == nil {
			destroy(e.session)
		}
		e.session = 0
	}
	if e.context != 0 {
		e.context.Destroy()
		e.context = 0
	}
}

func (e *Encoder) Close() error {
	if e.closed {
		return nil
	}
	e.closed = true
	e.thread.run(func() {
		e.drainHere()
		e.releaseHere()
	})
	e.thread.stop()
	e.thread = nil
	return nil
}
