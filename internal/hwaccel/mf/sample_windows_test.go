package mf

import (
	"bytes"
	"errors"
	"testing"
)

func pattern(size int) []byte {
	p := make([]byte, size)
	for i := range p {
		p[i] = byte(i*31 + 7)
	}
	return p
}

func TestNewMemoryBufferMaxLengthIsAtLeastRequestedSize(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	for _, size := range []int{1, 100, 4096 + 173} {
		buf, err := NewMemoryBuffer(size)
		if err != nil {
			t.Fatalf("NewMemoryBuffer(%d): %v", size, err)
		}
		max, err := buf.MaxLength()
		if err != nil {
			t.Fatalf("MaxLength: %v", err)
		}
		if max < size {
			t.Fatalf("buffer requested with size %d reports max length %d, want at least %d", size, max, size)
		}
		buf.Release()
	}
}

func TestBufferWriteThenReadReturnsTheSameBytes(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	for _, size := range []int{1, 2, 100, 4096 + 173} {
		buf, err := NewMemoryBuffer(size)
		if err != nil {
			t.Fatalf("NewMemoryBuffer(%d): %v", size, err)
		}
		want := pattern(size)
		if err := buf.Write(want); err != nil {
			t.Fatalf("Write(size %d): %v", size, err)
		}
		got := make([]byte, size)
		n, err := buf.Read(got)
		if err != nil {
			t.Fatalf("Read(size %d): %v", size, err)
		}
		if n != size {
			t.Fatalf("Read(size %d) copied %d bytes, want %d", size, n, size)
		}
		if !bytes.Equal(got, want) {
			t.Fatalf("Read(size %d) returned bytes differing from what Write sent", size)
		}
		buf.Release()
	}
}

func TestBufferReadIntoAShorterDestinationCopiesOnlyWhatFits(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	buf, err := NewMemoryBuffer(100)
	if err != nil {
		t.Fatalf("NewMemoryBuffer: %v", err)
	}
	defer buf.Release()
	want := pattern(100)
	if err := buf.Write(want); err != nil {
		t.Fatalf("Write: %v", err)
	}
	dst := make([]byte, 10)
	n, err := buf.Read(dst)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if n != len(dst) {
		t.Fatalf("Read into a %d-byte destination reported %d bytes copied, want %d", len(dst), n, len(dst))
	}
	if !bytes.Equal(dst, want[:10]) {
		t.Fatalf("Read into a shorter destination did not copy the leading bytes of the buffer")
	}
}

func TestBufferSetCurrentLengthThenCurrentLengthRoundTrips(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	buf, err := NewMemoryBuffer(64)
	if err != nil {
		t.Fatalf("NewMemoryBuffer: %v", err)
	}
	defer buf.Release()

	fresh, err := buf.CurrentLength()
	if err != nil {
		t.Fatalf("CurrentLength on a fresh buffer: %v", err)
	}
	if fresh != 0 {
		t.Fatalf("a freshly created buffer reports current length %d, want 0", fresh)
	}

	if err := buf.SetCurrentLength(30); err != nil {
		t.Fatalf("SetCurrentLength(30): %v", err)
	}
	got, err := buf.CurrentLength()
	if err != nil {
		t.Fatalf("CurrentLength after SetCurrentLength(30): %v", err)
	}
	if got != 30 {
		t.Fatalf("CurrentLength after SetCurrentLength(30) reports %d, want 30", got)
	}
}

func TestWithLockedSliceLengthIsCurrentLengthAndCapacityReachesMaxLength(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	const size = 128
	buf, err := NewMemoryBuffer(size)
	if err != nil {
		t.Fatalf("NewMemoryBuffer: %v", err)
	}
	defer buf.Release()
	if err := buf.SetCurrentLength(20); err != nil {
		t.Fatalf("SetCurrentLength(20): %v", err)
	}

	var gotLen, gotCap, gotMaxLen int
	err = buf.WithLocked(func(data []byte, maxLen int) error {
		gotLen = len(data)
		gotCap = cap(data)
		gotMaxLen = maxLen
		return nil
	})
	if err != nil {
		t.Fatalf("WithLocked: %v", err)
	}
	if gotMaxLen != size {
		t.Fatalf("WithLocked reported maxLen %d, want %d", gotMaxLen, size)
	}
	if gotLen != 20 {
		t.Fatalf("WithLocked handed a slice of length %d, want the current length 20", gotLen)
	}
	if gotCap != size {
		t.Fatalf("WithLocked handed a slice of capacity %d, want the max length %d", gotCap, size)
	}

	if _, err := buf.CurrentLength(); err != nil {
		t.Fatalf("buffer unusable after WithLocked: CurrentLength: %v", err)
	}
}

func TestWithLockedUnlocksEvenWhenTheFunctionReturnsAnError(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	buf, err := NewMemoryBuffer(16)
	if err != nil {
		t.Fatalf("NewMemoryBuffer: %v", err)
	}
	defer buf.Release()

	sentinel := errors.New("boom")
	err = buf.WithLocked(func(data []byte, maxLen int) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("WithLocked returned %v, want the sentinel error from the callback", err)
	}

	err = buf.WithLocked(func(data []byte, maxLen int) error {
		return nil
	})
	if err != nil {
		t.Fatalf("locking again after a failed callback: %v, want success proving the earlier lock was released", err)
	}
}

func TestSampleWithTwoBuffersReportsCountTwoAndConvertsToOneContiguousBuffer(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	sample, err := NewSample()
	if err != nil {
		t.Fatalf("NewSample: %v", err)
	}
	defer sample.Release()

	payload1 := pattern(50)
	payload2 := pattern(70)

	b1, err := NewMemoryBuffer(len(payload1))
	if err != nil {
		t.Fatalf("NewMemoryBuffer(payload1): %v", err)
	}
	defer b1.Release()
	if err := b1.Write(payload1); err != nil {
		t.Fatalf("Write(payload1): %v", err)
	}

	b2, err := NewMemoryBuffer(len(payload2))
	if err != nil {
		t.Fatalf("NewMemoryBuffer(payload2): %v", err)
	}
	defer b2.Release()
	if err := b2.Write(payload2); err != nil {
		t.Fatalf("Write(payload2): %v", err)
	}

	if err := sample.AddBuffer(b1); err != nil {
		t.Fatalf("AddBuffer(b1): %v", err)
	}
	if err := sample.AddBuffer(b2); err != nil {
		t.Fatalf("AddBuffer(b2): %v", err)
	}

	count, err := sample.BufferCount()
	if err != nil {
		t.Fatalf("BufferCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("sample with two added buffers reports BufferCount %d, want 2", count)
	}

	contig, err := sample.ConvertToContiguousBuffer()
	if err != nil {
		t.Fatalf("ConvertToContiguousBuffer: %v", err)
	}
	defer contig.Release()

	want := append(append([]byte{}, payload1...), payload2...)
	got := make([]byte, len(want))
	n, err := contig.Read(got)
	if err != nil {
		t.Fatalf("Read(contig): %v", err)
	}
	if n != len(want) {
		t.Fatalf("the contiguous buffer yielded %d bytes, want %d (the two payloads end to end)", n, len(want))
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("the contiguous buffer did not hold the two payloads end to end in order")
	}
}

func TestSampleTimeAndDurationRoundTrip(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	sample, err := NewSample()
	if err != nil {
		t.Fatalf("NewSample: %v", err)
	}
	defer sample.Release()

	const wantTime = int64(123456789)
	const wantDuration = int64(987654321)

	if err := sample.SetTime(wantTime); err != nil {
		t.Fatalf("SetTime: %v", err)
	}
	if err := sample.SetDuration(wantDuration); err != nil {
		t.Fatalf("SetDuration: %v", err)
	}

	gotTime, err := sample.Time()
	if err != nil {
		t.Fatalf("Time: %v", err)
	}
	if gotTime != wantTime {
		t.Fatalf("Time returned %d, want %d", gotTime, wantTime)
	}

	gotDuration, err := sample.Duration()
	if err != nil {
		t.Fatalf("Duration: %v", err)
	}
	if gotDuration != wantDuration {
		t.Fatalf("Duration returned %d, want %d", gotDuration, wantDuration)
	}
}

func TestSampleAndBufferReleaseAreSafeTwiceAndOnNil(t *testing.T) {
	if !Loaded() {
		t.Skip("Media Foundation is not present on this machine")
	}
	if err := Startup(); err != nil {
		t.Fatalf("Startup: %v", err)
	}
	defer Shutdown()

	var nilSample *Sample
	nilSample.Release()

	var nilBuffer *Buffer
	nilBuffer.Release()

	sample, err := NewSample()
	if err != nil {
		t.Fatalf("NewSample: %v", err)
	}
	sample.Release()
	sample.Release()

	buf, err := NewMemoryBuffer(16)
	if err != nil {
		t.Fatalf("NewMemoryBuffer: %v", err)
	}
	buf.Release()
	buf.Release()
}
