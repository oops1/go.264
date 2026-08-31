package nvenc

import (
	"runtime"
	"testing"
)

func skipIfNoCUDA(t *testing.T) {
	t.Helper()
	if !CUDAAvailable() {
		t.Skip("cuda driver not available")
	}
}

func TestLoadCUDAIdempotent(t *testing.T) {
	skipIfNoCUDA(t)
	if err := LoadCUDA(); err != nil {
		t.Fatalf("LoadCUDA returned error %v on first call", err)
	}
	if err := LoadCUDA(); err != nil {
		t.Fatalf("LoadCUDA returned error %v on second call", err)
	}
}

func TestDriverVersion(t *testing.T) {
	skipIfNoCUDA(t)
	version, err := DriverVersion()
	if err != nil {
		t.Fatalf("DriverVersion returned error %v", err)
	}
	t.Logf("cuda driver version reported: %d", version)
	if version < 1000 || version > 100000 {
		t.Fatalf("DriverVersion returned implausible version %d", version)
	}
}

func TestDeviceCountAndNames(t *testing.T) {
	skipIfNoCUDA(t)
	count, err := DeviceCount()
	if err != nil {
		t.Fatalf("DeviceCount returned error %v", err)
	}
	if count < 1 {
		t.Fatalf("DeviceCount returned %d, want at least 1", count)
	}
	for i := 0; i < count; i++ {
		dev, err := OpenDevice(i)
		if err != nil {
			t.Fatalf("OpenDevice(%d) returned error %v", i, err)
		}
		name, err := DeviceName(dev)
		if err != nil {
			t.Fatalf("DeviceName(%d) returned error %v", dev, err)
		}
		t.Logf("device %d: %q", i, name)
		if name == "" {
			t.Fatalf("DeviceName(%d) returned an empty name", dev)
		}
	}
}

func TestOpenDeviceOutOfRange(t *testing.T) {
	skipIfNoCUDA(t)
	count, err := DeviceCount()
	if err != nil {
		t.Fatalf("DeviceCount returned error %v", err)
	}
	badOrdinal := count + 1000
	if dev, err := OpenDevice(badOrdinal); err == nil {
		t.Fatalf("OpenDevice(%d) returned device %v with no error, want an error for an out-of-range ordinal", badOrdinal, dev)
	}
}

func TestContextCreateDestroy(t *testing.T) {
	skipIfNoCUDA(t)
	dev, err := OpenDevice(0)
	if err != nil {
		t.Fatalf("OpenDevice(0) returned error %v", err)
	}
	ctx, err := CreateContext(dev)
	if err != nil {
		t.Fatalf("CreateContext returned error %v", err)
	}
	if ctx == 0 {
		t.Fatalf("CreateContext returned a zero context")
	}
	if err := ctx.Destroy(); err != nil {
		t.Fatalf("Destroy returned error %v on first call", err)
	}
	if err := ctx.Destroy(); err != nil {
		t.Logf("Destroy returned error %v on second call, which is acceptable as long as it does not panic", err)
	}
	var zero Context
	if err := zero.Destroy(); err != nil {
		t.Fatalf("Destroy on a zero context returned error %v, want nil", err)
	}
}

func TestContextPushPop(t *testing.T) {
	skipIfNoCUDA(t)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	dev, err := OpenDevice(0)
	if err != nil {
		t.Fatalf("OpenDevice(0) returned error %v", err)
	}
	ctx, err := CreateContext(dev)
	if err != nil {
		t.Fatalf("CreateContext returned error %v", err)
	}
	defer ctx.Destroy()

	if err := ctx.Push(); err != nil {
		t.Fatalf("Push returned error %v", err)
	}
	popped, err := PopContext()
	if err != nil {
		t.Fatalf("PopContext returned error %v", err)
	}
	if popped != ctx {
		t.Fatalf("PopContext returned context %v, want the pushed context %v", popped, ctx)
	}
}

func TestContextStress(t *testing.T) {
	skipIfNoCUDA(t)
	dev, err := OpenDevice(0)
	if err != nil {
		t.Fatalf("OpenDevice(0) returned error %v", err)
	}
	for i := 0; i < 50; i++ {
		ctx, err := CreateContext(dev)
		if err != nil {
			t.Fatalf("CreateContext returned error %v on iteration %d", err, i)
		}
		if err := ctx.Destroy(); err != nil {
			t.Fatalf("Destroy returned error %v on iteration %d", err, i)
		}
		runtime.GC()
	}
}
