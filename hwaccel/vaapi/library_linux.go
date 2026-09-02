package vaapi

import (
	"errors"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

const (
	coreLibraryName = "libva.so.2"
	drmLibraryName  = "libva-drm.so.2"
)

var (
	libraryOnce sync.Once
	libraryErr  error

	getDisplayDRM func(int32) uintptr

	vaInitialize             func(uintptr, *int32, *int32) int32
	vaTerminate              func(uintptr) int32
	vaMaxNumEntrypoints      func(uintptr) int32
	vaQueryConfigEntrypoints func(uintptr, int32, unsafe.Pointer, *int32) int32
	vaCreateConfig           func(uintptr, int32, int32, unsafe.Pointer, int32, *uint32) int32
	vaDestroyConfig          func(uintptr, uint32) int32
	vaCreateSurfaces         func(uintptr, uint32, uint32, uint32, unsafe.Pointer, uint32, unsafe.Pointer, uint32) int32
	vaDestroySurfaces        func(uintptr, unsafe.Pointer, int32) int32
	vaCreateContext          func(uintptr, uint32, int32, int32, int32, unsafe.Pointer, int32, *uint32) int32
	vaDestroyContext         func(uintptr, uint32) int32
	vaCreateBuffer           func(uintptr, uint32, int32, uint32, uint32, unsafe.Pointer, *uint32) int32
	vaMapBuffer              func(uintptr, uint32, *unsafe.Pointer) int32
	vaUnmapBuffer            func(uintptr, uint32) int32
	vaDestroyBuffer          func(uintptr, uint32) int32
	vaBeginPicture           func(uintptr, uint32, uint32) int32
	vaRenderPicture          func(uintptr, uint32, unsafe.Pointer, int32) int32
	vaEndPicture             func(uintptr, uint32) int32
	vaSyncSurface            func(uintptr, uint32) int32
	vaMaxNumImageFormats     func(uintptr) int32
	vaQueryImageFormats      func(uintptr, unsafe.Pointer, *int32) int32
	vaCreateImage            func(uintptr, unsafe.Pointer, int32, int32, unsafe.Pointer) int32
	vaDestroyImage           func(uintptr, uint32) int32
	vaDeriveImage            func(uintptr, uint32, unsafe.Pointer) int32
	vaPutImage               func(uintptr, uint32, uint32, int32, int32, uint32, uint32, int32, int32, uint32, uint32) int32
)

func loadLibrary() error {
	libraryOnce.Do(func() {
		defer func() {
			if r := recover(); r != nil {
				libraryErr = errors.New("vaapi: failed to bind libva entry points")
			}
		}()

		core, err := purego.Dlopen(coreLibraryName, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			libraryErr = errors.New("vaapi: " + coreLibraryName + " is not available: " + err.Error())
			return
		}
		drm, err := purego.Dlopen(drmLibraryName, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			libraryErr = errors.New("vaapi: " + drmLibraryName + " is not available: " + err.Error())
			return
		}

		purego.RegisterLibFunc(&getDisplayDRM, drm, "vaGetDisplayDRM")

		purego.RegisterLibFunc(&vaInitialize, core, "vaInitialize")
		purego.RegisterLibFunc(&vaTerminate, core, "vaTerminate")
		purego.RegisterLibFunc(&vaMaxNumEntrypoints, core, "vaMaxNumEntrypoints")
		purego.RegisterLibFunc(&vaQueryConfigEntrypoints, core, "vaQueryConfigEntrypoints")
		purego.RegisterLibFunc(&vaCreateConfig, core, "vaCreateConfig")
		purego.RegisterLibFunc(&vaDestroyConfig, core, "vaDestroyConfig")
		purego.RegisterLibFunc(&vaCreateSurfaces, core, "vaCreateSurfaces")
		purego.RegisterLibFunc(&vaDestroySurfaces, core, "vaDestroySurfaces")
		purego.RegisterLibFunc(&vaCreateContext, core, "vaCreateContext")
		purego.RegisterLibFunc(&vaDestroyContext, core, "vaDestroyContext")
		purego.RegisterLibFunc(&vaCreateBuffer, core, "vaCreateBuffer")
		purego.RegisterLibFunc(&vaMapBuffer, core, "vaMapBuffer")
		purego.RegisterLibFunc(&vaUnmapBuffer, core, "vaUnmapBuffer")
		purego.RegisterLibFunc(&vaDestroyBuffer, core, "vaDestroyBuffer")
		purego.RegisterLibFunc(&vaBeginPicture, core, "vaBeginPicture")
		purego.RegisterLibFunc(&vaRenderPicture, core, "vaRenderPicture")
		purego.RegisterLibFunc(&vaEndPicture, core, "vaEndPicture")
		purego.RegisterLibFunc(&vaSyncSurface, core, "vaSyncSurface")
		purego.RegisterLibFunc(&vaMaxNumImageFormats, core, "vaMaxNumImageFormats")
		purego.RegisterLibFunc(&vaQueryImageFormats, core, "vaQueryImageFormats")
		purego.RegisterLibFunc(&vaCreateImage, core, "vaCreateImage")
		purego.RegisterLibFunc(&vaDestroyImage, core, "vaDestroyImage")
		purego.RegisterLibFunc(&vaDeriveImage, core, "vaDeriveImage")
		purego.RegisterLibFunc(&vaPutImage, core, "vaPutImage")
	})
	return libraryErr
}

func Available() bool { return loadLibrary() == nil }

func check(op string, st int32) error {
	if Status(st) == StatusSuccess {
		return nil
	}
	return &callFailure{Op: op, Status: Status(st)}
}

type callFailure struct {
	Op     string
	Status Status
}

func (f *callFailure) Error() string { return "vaapi: " + f.Op + ": " + f.Status.Error() }

func (f *callFailure) Unwrap() error { return f.Status }
