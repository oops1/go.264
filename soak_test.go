package go264

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/oops1/go.264/internal/testutil"
)

func diffSummary(got, want []byte) string {
	diff := 0
	first := -1
	for i := range got {
		if got[i] != want[i] {
			diff++
			if first < 0 {
				first = i
			}
		}
	}
	return fmt.Sprintf("%d of %d bytes differ, first at offset %d (got %d, want %d)",
		diff, len(got), first, got[first], want[first])
}

type soakGOP struct {
	start int
	bytes []byte
}

func TestSoakSustainedEncodeDecode(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test skipped in -short mode")
	}

	const (
		width, height = 640, 480
		totalFrames   = 480
		gopSize       = 24
		settleFrames  = 48
		maxQueuedGOPs = 6
	)

	cfg := EncoderConfig{
		Width: width, Height: height,
		FPSNum: 30, FPSDen: 1,
		GOPSize: gopSize, QP: 30, RefFrames: 1,
		ForceSoftware: true,
	}
	enc, err := NewEncoder(cfg)
	if err != nil {
		t.Fatalf("NewEncoder: %v", err)
	}
	defer enc.Close()
	if enc.Backend() != "cpu" {
		t.Fatalf("ForceSoftware still selected backend %q", enc.Backend())
	}

	dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
	defer dec.Close()
	if dec.Backend() != "cpu" {
		t.Fatalf("ForceSoftware still selected backend %q", dec.Backend())
	}

	pending := make(map[int][]byte)
	var queue []soakGOP
	var gopBuf []byte
	gopStart := 0

	drain := func(force bool) {
		for len(queue) > 0 {
			front := queue[0]
			ready := true
			for idx := front.start; idx < front.start+gopSize; idx++ {
				if _, ok := pending[idx]; !ok {
					ready = false
					break
				}
			}
			if !ready && !force {
				break
			}
			if !ready && force {
				t.Fatalf("GOP starting at frame %d never produced %d sustained-decoder outputs to compare", front.start, gopSize)
			}

			fresh := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
			frames, err := fresh.Decode(front.bytes)
			if err != nil {
				t.Fatalf("fresh decode of GOP starting at frame %d: %v", front.start, err)
			}
			rest, err := fresh.Flush()
			if err != nil {
				t.Fatalf("fresh decode flush of GOP starting at frame %d: %v", front.start, err)
			}
			frames = append(frames, rest...)
			fresh.Close()

			if len(frames) != gopSize {
				t.Fatalf("GOP starting at frame %d: a fresh decoder produced %d frames, want %d", front.start, len(frames), gopSize)
			}
			for j, f := range frames {
				idx := front.start + j
				want := pending[idx]
				got := f.AppendI420(nil)
				if !bytes.Equal(got, want) {
					t.Fatalf("frame %d: sustained decoder output diverged from a fresh decoder started at the same key frame: %s",
						idx, diffSummary(got, want))
				}
				delete(pending, idx)
			}
			queue = queue[1:]
		}
		if len(queue) > maxQueuedGOPs {
			t.Fatalf("%d GOPs are queued for drift verification without their sustained-decoder output ever completing", len(queue))
		}
	}

	var prevDecoded []byte
	decodedCount := 0
	var baseline, final runtime.MemStats
	baselineTaken := false

	start := time.Now()
	for i := 0; i < totalFrames; i++ {
		src := pattern(width, height, i)
		pkt, err := enc.Encode(src)
		if err != nil {
			t.Fatalf("frame %d: Encode: %v", i, err)
		}
		gopBuf = append(gopBuf, pkt...)

		frames, err := dec.Decode(pkt)
		if err != nil {
			t.Fatalf("frame %d: Decode: %v", i, err)
		}
		for _, f := range frames {
			if f.Width != width || f.Height != height {
				t.Fatalf("decoded frame %d: size %dx%d, want %dx%d", decodedCount, f.Width, f.Height, width, height)
			}
			got := f.AppendI420(nil)
			if prevDecoded != nil && bytes.Equal(got, prevDecoded) {
				t.Fatalf("decoded frame %d is byte-identical to frame %d despite moving source content", decodedCount, decodedCount-1)
			}
			prevDecoded = got
			pending[decodedCount] = got
			decodedCount++
		}

		if (i+1)%gopSize == 0 {
			queue = append(queue, soakGOP{start: gopStart, bytes: gopBuf})
			gopBuf = nil
			gopStart = i + 1
			drain(false)
		}

		if i+1 == settleFrames {
			runtime.GC()
			runtime.ReadMemStats(&baseline)
			baselineTaken = true
		}
	}

	rest, err := dec.Flush()
	if err != nil {
		t.Fatalf("Flush: %v", err)
	}
	for _, f := range rest {
		if f.Width != width || f.Height != height {
			t.Fatalf("flushed frame %d: size %dx%d, want %dx%d", decodedCount, f.Width, f.Height, width, height)
		}
		got := f.AppendI420(nil)
		if prevDecoded != nil && bytes.Equal(got, prevDecoded) {
			t.Fatalf("flushed frame %d is byte-identical to frame %d despite moving source content", decodedCount, decodedCount-1)
		}
		prevDecoded = got
		pending[decodedCount] = got
		decodedCount++
	}
	drain(true)

	if decodedCount != totalFrames {
		t.Fatalf("decoded %d frames total, want %d", decodedCount, totalFrames)
	}
	if len(pending) != 0 {
		t.Fatalf("%d decoded frames were never consumed by a drift check", len(pending))
	}
	if !baselineTaken {
		t.Fatalf("never reached the %d-frame settling point to take a memory baseline", settleFrames)
	}

	runtime.GC()
	runtime.ReadMemStats(&final)

	elapsed := time.Since(start)
	t.Logf("encoded+decoded %d frames of %dx%d in %v (%.2f frames/s)", totalFrames, width, height, elapsed, float64(totalFrames)/elapsed.Seconds())
	t.Logf("heap alloc after frame %d: %d bytes; after frame %d: %d bytes (delta %+d)",
		settleFrames, baseline.HeapAlloc, totalFrames, final.HeapAlloc, int64(final.HeapAlloc)-int64(baseline.HeapAlloc))

	const maxHeapGrowth = 64 << 20
	if final.HeapAlloc > baseline.HeapAlloc && final.HeapAlloc-baseline.HeapAlloc > maxHeapGrowth {
		t.Fatalf("live heap grew by %d bytes between frame %d and frame %d, exceeding the %d byte bound",
			final.HeapAlloc-baseline.HeapAlloc, settleFrames, totalFrames, maxHeapGrowth)
	}
}

func TestSoakRestartResilience(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test skipped in -short mode")
	}

	const (
		width, height = 64, 64
		iterations    = 500
		settleAt      = 50
	)
	src := pattern(width, height, 0)

	var baseline, final runtime.MemStats
	baselineTaken := false

	start := time.Now()
	for i := 0; i < iterations; i++ {
		enc, err := NewEncoder(EncoderConfig{
			Width: width, Height: height,
			FPSNum: 25, FPSDen: 1,
			GOPSize: 5, QP: 26,
			ForceSoftware: true,
		})
		if err != nil {
			t.Fatalf("iteration %d: NewEncoder: %v", i, err)
		}
		pkt, err := enc.Encode(src)
		if err != nil {
			t.Fatalf("iteration %d: Encode: %v", i, err)
		}
		if err := enc.Close(); err != nil {
			t.Fatalf("iteration %d: Encoder.Close: %v", i, err)
		}
		if err := enc.Close(); err != nil {
			t.Fatalf("iteration %d: second Encoder.Close: %v", i, err)
		}

		dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
		frames, err := dec.Decode(pkt)
		if err != nil {
			t.Fatalf("iteration %d: Decode: %v", i, err)
		}
		rest, err := dec.Flush()
		if err != nil {
			t.Fatalf("iteration %d: Flush: %v", i, err)
		}
		frames = append(frames, rest...)
		if len(frames) != 1 {
			t.Fatalf("iteration %d: decoded %d frames, want 1", i, len(frames))
		}
		if frames[0].Width != width || frames[0].Height != height {
			t.Fatalf("iteration %d: decoded size %dx%d, want %dx%d", i, frames[0].Width, frames[0].Height, width, height)
		}
		if err := dec.Close(); err != nil {
			t.Fatalf("iteration %d: Decoder.Close: %v", i, err)
		}

		runtime.GC()
		if i+1 == settleAt {
			runtime.ReadMemStats(&baseline)
			baselineTaken = true
		}
	}
	if !baselineTaken {
		t.Fatalf("never reached the %d-iteration settling point to take a memory baseline", settleAt)
	}
	runtime.ReadMemStats(&final)
	elapsed := time.Since(start)

	t.Logf("%d encoder+decoder open/close cycles in %v (%.1f cycles/s)", iterations, elapsed, float64(iterations)/elapsed.Seconds())
	t.Logf("heap alloc after cycle %d: %d bytes; after cycle %d: %d bytes (delta %+d)",
		settleAt, baseline.HeapAlloc, iterations, final.HeapAlloc, int64(final.HeapAlloc)-int64(baseline.HeapAlloc))

	const maxHeapGrowth = 16 << 20
	if final.HeapAlloc > baseline.HeapAlloc && final.HeapAlloc-baseline.HeapAlloc > maxHeapGrowth {
		t.Fatalf("live heap grew by %d bytes over %d restart cycles, exceeding the %d byte bound",
			final.HeapAlloc-baseline.HeapAlloc, iterations-settleAt, maxHeapGrowth)
	}
}

func TestSoakCorpusRepeat(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test skipped in -short mode")
	}

	type clipData struct {
		clip   testutil.Clip
		stream []byte
		yuv    []byte
	}

	var all []testutil.Clip
	all = append(all, testutil.Corpus...)
	all = append(all, testutil.MainCorpus...)

	clips := make([]clipData, 0, len(all))
	for _, c := range all {
		clips = append(clips, clipData{
			clip:   c,
			stream: testutil.LoadStream(t, c),
			yuv:    testutil.LoadReferenceYUV(t, c),
		})
	}

	const budget = 90 * time.Second
	deadline := time.Now().Add(budget)
	start := time.Now()
	passes := 0
	for time.Now().Before(deadline) {
		for _, cd := range clips {
			dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
			frames, err := dec.Decode(cd.stream)
			if err != nil {
				t.Fatalf("pass %d clip %s: Decode: %v", passes, cd.clip.Name, err)
			}
			rest, err := dec.Flush()
			if err != nil {
				t.Fatalf("pass %d clip %s: Flush: %v", passes, cd.clip.Name, err)
			}
			frames = append(frames, rest...)
			dec.Close()

			if len(frames) != cd.clip.Frames {
				t.Fatalf("pass %d clip %s: decoded %d frames, want %d", passes, cd.clip.Name, len(frames), cd.clip.Frames)
			}
			for i, f := range frames {
				if f.Width != cd.clip.Width || f.Height != cd.clip.Height {
					t.Fatalf("pass %d clip %s frame %d: size %dx%d, want %dx%d", passes, cd.clip.Name, i, f.Width, f.Height, cd.clip.Width, cd.clip.Height)
				}
				want := cd.clip.Frame(cd.yuv, i)
				got := f.AppendI420(nil)
				if !bytes.Equal(got, want) {
					t.Fatalf("pass %d clip %s frame %d: decoded output no longer matches the reference YUV exactly: %s",
						passes, cd.clip.Name, i, diffSummary(got, want))
				}
			}
		}
		passes++
	}
	elapsed := time.Since(start)
	t.Logf("decoded the %d-clip conformance corpus %d times in %v", len(clips), passes, elapsed)
	if passes < 2 {
		t.Fatalf("only completed %d pass(es) of the corpus in %v, too few to catch a state bug that appears after repetition", passes, budget)
	}
}

func TestSoakConcurrent(t *testing.T) {
	if testing.Short() {
		t.Skip("soak test skipped in -short mode")
	}

	const (
		goroutines         = 8
		framesPerGoroutine = 150
		width, height      = 176, 144
		gopSize            = 15
	)

	var wg sync.WaitGroup
	errs := make(chan error, goroutines)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			enc, err := NewEncoder(EncoderConfig{
				Width: width, Height: height,
				FPSNum: 25, FPSDen: 1,
				GOPSize: gopSize, QP: 26, RefFrames: 1,
				ForceSoftware: true,
			})
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: NewEncoder: %w", id, err)
				return
			}
			defer enc.Close()

			dec := NewDecoderWithConfig(DecoderConfig{ForceSoftware: true})
			defer dec.Close()

			var prev []byte
			decoded := 0
			for i := 0; i < framesPerGoroutine; i++ {
				src := pattern(width, height, id*100000+i)
				pkt, err := enc.Encode(src)
				if err != nil {
					errs <- fmt.Errorf("goroutine %d frame %d: Encode: %w", id, i, err)
					return
				}
				frames, err := dec.Decode(pkt)
				if err != nil {
					errs <- fmt.Errorf("goroutine %d frame %d: Decode: %w", id, i, err)
					return
				}
				for _, f := range frames {
					if f.Width != width || f.Height != height {
						errs <- fmt.Errorf("goroutine %d: decoded size %dx%d, want %dx%d", id, f.Width, f.Height, width, height)
						return
					}
					got := f.AppendI420(nil)
					if prev != nil && bytes.Equal(got, prev) {
						errs <- fmt.Errorf("goroutine %d: decoded frame %d repeats the previous frame despite moving content", id, decoded)
						return
					}
					prev = got
					decoded++
				}
			}
			rest, err := dec.Flush()
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: Flush: %w", id, err)
				return
			}
			decoded += len(rest)
			if decoded != framesPerGoroutine {
				errs <- fmt.Errorf("goroutine %d: decoded %d frames, want %d", id, decoded, framesPerGoroutine)
				return
			}
		}(g)
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
