package mf

type DecodedPicture struct {
	I420   []byte
	Width  int
	Height int
}

type Decoder struct {
	name     string
	hardware bool

	transform *Transform
	inStream  uint32
	outStream uint32

	width  int
	height int
	stride int

	providesSamples bool
	outSize         int
	outAlign        int

	nv12  []byte
	ready []*DecodedPicture

	started bool
	drained bool
	time    int64

	thread *comThread
}

func (d *Decoder) Name() string { return d.name }

func (d *Decoder) Hardware() bool { return d.hardware }

func OpenDecoder(hardwareOnly bool) (*Decoder, error) {
	thread := newCOMThread()
	var dec *Decoder
	var err error
	thread.run(func() { dec, err = openDecoderHere(hardwareOnly) })
	if err != nil {
		thread.stop()
		return nil, err
	}
	dec.thread = thread
	return dec, nil
}

func openDecoderHere(hardwareOnly bool) (*Decoder, error) {
	if err := Startup(); err != nil {
		return nil, err
	}
	inInfo, _ := H264DecoderTypes()
	accept := func(t TransformDescription) bool { return !hardwareOnly || t.Hardware }
	transform, chosen, err := openTransform(MFTCategoryVideoDecoder, &inInfo, nil, accept)
	if err != nil {
		Shutdown()
		return nil, err
	}
	d := &Decoder{name: chosen.Name, hardware: chosen.Hardware, transform: transform}
	if err := d.configure(); err != nil {
		d.closeHere()
		return nil, err
	}
	return d, nil
}

func (d *Decoder) configure() error {
	if err := d.transform.Unlock(); err != nil {
		return err
	}
	if d.transform.IsAsync() {
		return &Error{Op: "asynchronous decoder transforms are not driven yet", Code: sOK}
	}

	in, out, err := d.transform.StreamIDs()
	if err != nil {
		return err
	}
	d.inStream, d.outStream = in, out

	inType, err := NewMediaType()
	if err != nil {
		return err
	}
	defer inType.Release()
	if err := inType.SetGUID(&MFMTMajorType, MFMediaTypeVideo); err != nil {
		return err
	}
	if err := inType.SetGUID(&MFMTSubtype, MFVideoFormatH264); err != nil {
		return err
	}
	if err := inType.SetUint32(&MFMTInterlaceMode, MFVideoInterlaceProgressive); err != nil {
		return err
	}
	if err := d.transform.SetInputType(d.inStream, inType.obj.p, 0); err != nil {
		return err
	}
	if err := d.selectOutputType(); err != nil {
		return err
	}
	return d.transform.ProcessMessage(MFTMessageNotifyBeginStreaming, 0)
}

func (d *Decoder) selectOutputType() error {
	for i := uint32(0); ; i++ {
		raw, code := d.transform.OutputAvailableType(d.outStream, i)
		if code == MFENoMoreTypes {
			return &Error{Op: "the decoder offers no planar output type", Code: MFENoMoreTypes}
		}
		if err := check("IMFTransform::GetOutputAvailableType", code); err != nil {
			return err
		}
		candidate := &MediaType{obj: unknown{raw}}
		subtype, err := candidate.GUID(&MFMTSubtype)
		if err != nil || subtype != MFVideoFormatNV12 {
			candidate.Release()
			continue
		}
		err = d.transform.SetOutputType(d.outStream, candidate.obj.p, 0)
		candidate.Release()
		if err != nil {
			return err
		}
		return d.readOutputGeometry()
	}
}

func (d *Decoder) readOutputGeometry() error {
	raw, err := d.transform.OutputCurrentType(d.outStream)
	if err != nil {
		return err
	}
	current := &MediaType{obj: unknown{raw}}
	defer current.Release()

	w, h, err := current.Size(&MFMTFrameSize)
	if err != nil {
		return err
	}
	d.width, d.height = int(w), int(h)
	d.stride = d.width
	if v, err := current.Uint32(&MFMTDefaultStride); err == nil && int32(v) > 0 {
		d.stride = int(int32(v))
	}

	info, err := d.transform.OutputStreamInfo(d.outStream)
	if err != nil {
		return err
	}
	d.providesSamples = info.DwFlags&(outputStreamProvidesSamples|outputStreamCanProvideSamples) != 0
	d.outSize = int(info.CbSize)
	if d.outSize < NV12Size(d.stride, d.height) {
		d.outSize = NV12Size(d.stride, d.height)
	}
	d.outAlign = int(info.CbAlignment)
	d.nv12 = make([]byte, NV12Size(d.stride, d.height))
	return nil
}

func (d *Decoder) pushUnits(annexB []byte) error {
	sample, err := NewSample()
	if err != nil {
		return err
	}
	defer sample.Release()
	buf, err := NewMemoryBuffer(len(annexB))
	if err != nil {
		return err
	}
	defer buf.Release()
	if err := buf.Write(annexB); err != nil {
		return err
	}
	if err := sample.AddBuffer(buf); err != nil {
		return err
	}
	if err := sample.SetTime(d.time); err != nil {
		return err
	}
	d.time++
	code := d.transform.ProcessInput(d.inStream, sample.obj.p)
	return check("IMFTransform::ProcessInput", code)
}

func (d *Decoder) collectSample(sample *Sample) error {
	buf, err := sample.ConvertToContiguousBuffer()
	if err != nil {
		return err
	}
	defer buf.Release()
	n, err := buf.Read(d.nv12)
	if err != nil {
		return err
	}
	if n < NV12Size(d.stride, d.height) {
		return nil
	}
	pic := &DecodedPicture{
		I420:   make([]byte, I420Size(d.width, d.height)),
		Width:  d.width,
		Height: d.height,
	}
	NV12ToI420(pic.I420, d.nv12, d.stride, d.width, d.height)
	d.ready = append(d.ready, pic)
	return nil
}

func (d *Decoder) pullOutput() (bool, error) {
	var buffers [1]MFTOutputDataBuffer
	buffers[0].DwStreamID = d.outStream

	var owned *Sample
	if !d.providesSamples {
		sample, err := NewSample()
		if err != nil {
			return false, err
		}
		buf, err := NewAlignedMemoryBuffer(d.outSize, d.outAlign)
		if err != nil {
			sample.Release()
			return false, err
		}
		if err := sample.AddBuffer(buf); err != nil {
			buf.Release()
			sample.Release()
			return false, err
		}
		buf.Release()
		owned = sample
		buffers[0].PSample = sample.obj.p
	}

	_, code := d.transform.ProcessOutput(buffers[:])
	produced := buffers[0].PSample

	switch {
	case code == MFETransformNeedMoreInput:
		owned.Release()
		return false, nil
	case code == MFETransformStreamChange:
		owned.Release()
		if err := d.selectOutputType(); err != nil {
			return false, err
		}
		return true, nil
	}
	if err := check("IMFTransform::ProcessOutput", code); err != nil {
		owned.Release()
		return false, err
	}

	sample := owned
	if sample == nil {
		if produced == nil {
			return false, nil
		}
		sample = &Sample{obj: unknown{produced}}
	}
	defer sample.Release()
	if buffers[0].PEvents != nil {
		(unknown{buffers[0].PEvents}).release()
	}
	if err := d.collectSample(sample); err != nil {
		return false, err
	}
	return true, nil
}

func (d *Decoder) pullAll() error {
	for {
		more, err := d.pullOutput()
		if err != nil || !more {
			return err
		}
	}
}

func (d *Decoder) take() []*DecodedPicture {
	out := d.ready
	d.ready = nil
	return out
}

func (d *Decoder) decodeHere(annexB []byte) ([]*DecodedPicture, error) {
	if d.transform == nil {
		return nil, &Error{Op: "Decode on a closed decoder", Code: sOK}
	}
	if len(annexB) == 0 {
		return nil, nil
	}
	if !d.started {
		if err := d.transform.ProcessMessage(MFTMessageNotifyStartOfStream, 0); err != nil {
			return nil, err
		}
		d.started = true
	}
	if err := d.pushUnits(annexB); err != nil {
		return nil, err
	}
	if err := d.pullAll(); err != nil {
		return nil, err
	}
	return d.take(), nil
}

func (d *Decoder) flushHere() ([]*DecodedPicture, error) {
	if d.transform == nil || d.drained {
		return nil, nil
	}
	d.drained = true
	if err := d.transform.ProcessMessage(MFTMessageNotifyEndOfStream, 0); err != nil {
		return nil, err
	}
	if err := d.transform.ProcessMessage(MFTMessageCommandDrain, 0); err != nil {
		return nil, err
	}
	if err := d.pullAll(); err != nil {
		return nil, err
	}
	return d.take(), nil
}

func (d *Decoder) closeHere() error {
	if d.transform == nil {
		return nil
	}
	if _, err := d.flushHere(); err != nil {
		return err
	}
	d.transform.ProcessMessage(MFTMessageCommandFlush, 0)
	d.transform.ProcessMessage(MFTMessageNotifyEndStreaming, 0)
	d.transform.Release()
	d.transform = nil
	return Shutdown()
}

func (d *Decoder) Decode(annexB []byte) (out []*DecodedPicture, err error) {
	d.thread.run(func() { out, err = d.decodeHere(annexB) })
	return out, err
}

func (d *Decoder) Flush() (out []*DecodedPicture, err error) {
	d.thread.run(func() { out, err = d.flushHere() })
	return out, err
}

func (d *Decoder) Close() error {
	var err error
	d.thread.run(func() { err = d.closeHere() })
	d.thread.stop()
	d.thread = nil
	return err
}
