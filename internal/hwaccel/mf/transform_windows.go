package mf

import "unsafe"

type Transform struct {
	obj unknown
}

func (t *Transform) Release() {
	if t == nil {
		return
	}
	t.obj.release()
	t.obj = unknown{}
}

func (t *Transform) StreamIDs() (in, out uint32, err error) {
	var inCount, outCount uint32
	code := hr(t.obj.p, transformGetStreamCount,
		uintptr(unsafe.Pointer(&inCount)), uintptr(unsafe.Pointer(&outCount)))
	if err := check("IMFTransform::GetStreamCount", code); err != nil {
		return 0, 0, err
	}
	if inCount == 0 || outCount == 0 {
		return 0, 0, &Error{Op: "IMFTransform reported no streams", Code: sOK}
	}
	inIDs := make([]uint32, inCount)
	outIDs := make([]uint32, outCount)
	code = hr(t.obj.p, transformGetStreamIDs,
		uintptr(inCount), uintptr(unsafe.Pointer(&inIDs[0])),
		uintptr(outCount), uintptr(unsafe.Pointer(&outIDs[0])))
	if code.Failed() {
		return 0, 0, nil
	}
	return inIDs[0], outIDs[0], nil
}

func (t *Transform) ProcessMessage(message uint32, param uintptr) error {
	code := hr(t.obj.p, transformProcessMessage, uintptr(message), param)
	return check("IMFTransform::ProcessMessage", code)
}

func (t *Transform) OutputStreamInfo(stream uint32) (MFTOutputStreamInfo, error) {
	var info MFTOutputStreamInfo
	code := hr(t.obj.p, transformGetOutputStreamInfo,
		uintptr(stream), uintptr(unsafe.Pointer(&info)))
	return info, check("IMFTransform::GetOutputStreamInfo", code)
}

func (t *Transform) InputStreamInfo(stream uint32) (MFTInputStreamInfo, error) {
	var info MFTInputStreamInfo
	code := hr(t.obj.p, transformGetInputStreamInfo,
		uintptr(stream), uintptr(unsafe.Pointer(&info)))
	return info, check("IMFTransform::GetInputStreamInfo", code)
}

func (t *Transform) Attributes() (unknown, error) {
	var out unsafe.Pointer
	code := hr(t.obj.p, transformGetAttributes, uintptr(unsafe.Pointer(&out)))
	if err := check("IMFTransform::GetAttributes", code); err != nil {
		return unknown{}, err
	}
	return unknown{out}, nil
}
