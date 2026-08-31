package nvenc

import (
	"fmt"
	"sync"

	"github.com/ebitengine/purego"
)

type CUDAError uint32

const (
	CUDASuccess             CUDAError = 0
	CUDAErrorInvalidValue   CUDAError = 1
	CUDAErrorOutOfMemory    CUDAError = 2
	CUDAErrorNotInitialized CUDAError = 3
	CUDAErrorNoDevice       CUDAError = 100
	CUDAErrorInvalidDevice  CUDAError = 101
	CUDAErrorInvalidContext CUDAError = 201
)

var cudaErrorNames = map[CUDAError]string{
	CUDASuccess:             "CUDA_SUCCESS",
	CUDAErrorInvalidValue:   "CUDA_ERROR_INVALID_VALUE",
	CUDAErrorOutOfMemory:    "CUDA_ERROR_OUT_OF_MEMORY",
	CUDAErrorNotInitialized: "CUDA_ERROR_NOT_INITIALIZED",
	CUDAErrorNoDevice:       "CUDA_ERROR_NO_DEVICE",
	CUDAErrorInvalidDevice:  "CUDA_ERROR_INVALID_DEVICE",
	CUDAErrorInvalidContext: "CUDA_ERROR_INVALID_CONTEXT",
}

func (e CUDAError) Error() string {
	if name, ok := cudaErrorNames[e]; ok {
		return fmt.Sprintf("nvenc: %s (cuda result %d)", name, uint32(e))
	}
	return fmt.Sprintf("nvenc: cuda result %d", uint32(e))
}

func cudaCheck(result int32) error {
	if CUDAError(result) == CUDASuccess {
		return nil
	}
	return CUDAError(result)
}

const (
	CUCtxSchedAuto         uint32 = 0x00
	CUCtxSchedSpin         uint32 = 0x01
	CUCtxSchedYield        uint32 = 0x02
	CUCtxSchedBlockingSync uint32 = 0x04
)

type Device int32

type Context uintptr

var (
	cuInitFn             func(uint32) int32
	cuDriverGetVersionFn func(*int32) int32
	cuDeviceGetCountFn   func(*int32) int32
	cuDeviceGetFn        func(*int32, int32) int32
	cuDeviceGetNameFn    func(*byte, int32, int32) int32
	cuCtxCreateFn        func(*uintptr, uint32, int32) int32
	cuCtxDestroyFn       func(uintptr) int32
	cuCtxPushCurrentFn   func(uintptr) int32
	cuCtxPopCurrentFn    func(*uintptr) int32
)

var (
	loadOnce sync.Once
	loadErr  error
)

func LoadCUDA() error {
	loadOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				loadErr = fmt.Errorf("nvenc: failed to bind cuda driver entry points: %v", r)
			}
		}()

		handle, err := purego.Dlopen("libcuda.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			loadErr = fmt.Errorf("nvenc: cuda driver not available: %w", err)
			return
		}

		purego.RegisterLibFunc(&cuInitFn, handle, "cuInit")
		purego.RegisterLibFunc(&cuDriverGetVersionFn, handle, "cuDriverGetVersion")
		purego.RegisterLibFunc(&cuDeviceGetCountFn, handle, "cuDeviceGetCount")
		purego.RegisterLibFunc(&cuDeviceGetFn, handle, "cuDeviceGet")
		purego.RegisterLibFunc(&cuDeviceGetNameFn, handle, "cuDeviceGetName")
		purego.RegisterLibFunc(&cuCtxCreateFn, handle, "cuCtxCreate_v2")
		purego.RegisterLibFunc(&cuCtxDestroyFn, handle, "cuCtxDestroy_v2")
		purego.RegisterLibFunc(&cuCtxPushCurrentFn, handle, "cuCtxPushCurrent_v2")
		purego.RegisterLibFunc(&cuCtxPopCurrentFn, handle, "cuCtxPopCurrent_v2")

		if err := cudaCheck(cuInitFn(0)); err != nil {
			loadErr = fmt.Errorf("nvenc: cuInit failed: %w", err)
		}
	})
	return loadErr
}

func CUDAAvailable() bool {
	return LoadCUDA() == nil
}

func DriverVersion() (int, error) {
	if err := LoadCUDA(); err != nil {
		return 0, err
	}
	var version int32
	if err := cudaCheck(cuDriverGetVersionFn(&version)); err != nil {
		return 0, err
	}
	return int(version), nil
}

func DeviceCount() (int, error) {
	if err := LoadCUDA(); err != nil {
		return 0, err
	}
	var count int32
	if err := cudaCheck(cuDeviceGetCountFn(&count)); err != nil {
		return 0, err
	}
	return int(count), nil
}

func DeviceName(d Device) (string, error) {
	if err := LoadCUDA(); err != nil {
		return "", err
	}
	buf := make([]byte, 256)
	if err := cudaCheck(cuDeviceGetNameFn(&buf[0], int32(len(buf)), int32(d))); err != nil {
		return "", err
	}
	end := len(buf)
	for i, b := range buf {
		if b == 0 {
			end = i
			break
		}
	}
	return string(buf[:end]), nil
}

func OpenDevice(ordinal int) (Device, error) {
	if err := LoadCUDA(); err != nil {
		return 0, err
	}
	var dev int32
	if err := cudaCheck(cuDeviceGetFn(&dev, int32(ordinal))); err != nil {
		return 0, err
	}
	return Device(dev), nil
}

func CreateContext(d Device) (Context, error) {
	if err := LoadCUDA(); err != nil {
		return 0, err
	}
	var ctx uintptr
	if err := cudaCheck(cuCtxCreateFn(&ctx, CUCtxSchedAuto, int32(d))); err != nil {
		return 0, err
	}
	return Context(ctx), nil
}

func (c Context) Destroy() error {
	if c == 0 {
		return nil
	}
	if err := LoadCUDA(); err != nil {
		return err
	}
	return cudaCheck(cuCtxDestroyFn(uintptr(c)))
}

func (c Context) Push() error {
	if err := LoadCUDA(); err != nil {
		return err
	}
	return cudaCheck(cuCtxPushCurrentFn(uintptr(c)))
}

func PopContext() (Context, error) {
	if err := LoadCUDA(); err != nil {
		return 0, err
	}
	var ctx uintptr
	if err := cudaCheck(cuCtxPopCurrentFn(&ctx)); err != nil {
		return 0, err
	}
	return Context(ctx), nil
}
