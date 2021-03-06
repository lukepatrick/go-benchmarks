//go:linkname nanotime runtime.nanotime
package queues

import (
	"runtime"
	"sync"
	"testing"
	_ "unsafe"

	"code.cloudfoundry.org/go-diodes"
	"github.com/codahale/hdrhistogram"
	"github.com/loov/hrtime"
	"github.com/pltr/onering"
	"github.com/tidwall/fastlane"
	"github.com/tylertreat/hdrhistogram-writer"
	"unsafe"
)

func nanotime() int64
func mknumslice(n int) []int64 {
	var s = make([]int64, n)
	for i := range s {
		s[i] = int64(i)
	}
	return s
}
func Benchmark1Producer1ConsumerChannel(b *testing.B) {
	startNanos := make([]int64, b.N)
	endNanos := make([]int64, b.N)

	q := make(chan *int64, 8192)
	var numbers = mknumslice(b.N)
	var wg sync.WaitGroup
	wg.Add(2)

	go func(n int) {
		runtime.LockOSThread()
		for i := 0; i < n; i++ {
			q <- &numbers[i]
		}
		wg.Done()
	}(b.N)

	b.ResetTimer()
	go func(n int) {
		runtime.LockOSThread()
		for i := 0; i < n; i++ {
			startNanos[i] = nanotime()
			<-q
			endNanos[i] = nanotime()
			b.SetBytes(1)
		}
		wg.Done()
	}(b.N)

	wg.Wait()

	b.StopTimer()
	recordLatencyDistribution("channel", b.N, startNanos, endNanos)
}

func Benchmark1Producer1ConsumerDiode(b *testing.B) {
	startNanos := make([]int64, b.N)
	endNanos := make([]int64, b.N)

	d := diodes.NewPoller(diodes.NewOneToOne(b.N, diodes.AlertFunc(func(missed int) {
		panic("Oops...")
	})))
	var numbers = mknumslice(b.N)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		for i := 0; i < b.N; i++ {
			d.Set(diodes.GenericDataType(&numbers[i]))
		}
		wg.Done()
	}()

	b.ResetTimer()
	go func(n int) {
		for i := 0; i < b.N; i++ {
			startNanos[i] = nanotime()
			d.Next()
			endNanos[i] = nanotime()
			b.SetBytes(1)
		}
		wg.Done()
	}(b.N)

	wg.Wait()

	b.StopTimer()
	recordLatencyDistribution("diode", b.N, startNanos, endNanos)
}

func Benchmark1Producer1ConsumerFastlane(b *testing.B) {
	startNanos := make([]int64, b.N)
	endNanos := make([]int64, b.N)
	var numbers = mknumslice(b.N)
	var ch fastlane.ChanPointer
	var wg sync.WaitGroup
	wg.Add(2)
	go func(n int) {
		for i := 0; i < n; i++ {
			startNanos[i] = nanotime()
			ch.Recv()
			endNanos[i] = nanotime()
			b.SetBytes(1)
		}
		wg.Done()
	}(b.N)

	b.ResetTimer()
	go func(n int) {
		for i := 0; i < n; i++ {
			ch.Send(unsafe.Pointer(&numbers[i]))
		}
		wg.Done()
	}(b.N)

	wg.Wait()

	b.StopTimer()
	recordLatencyDistribution("fastlane", b.N, startNanos, endNanos)
}

func Benchmark1Producer1ConsumerOneRing(b *testing.B) {
	startNanos := make([]int64, b.N)
	endNanos := make([]int64, b.N)
	var numbers = mknumslice(b.N)
	var ring = onering.New{Size:8192}.SPSC()
	var wg sync.WaitGroup
	wg.Add(2)

	go func(n int) {
		runtime.LockOSThread()
		for i := 0; i < n; i++ {
			ring.Put(&numbers[i])
		}
		ring.Close()
		wg.Done()
	}(b.N)

	b.ResetTimer()
	go func(n int64) {
		runtime.LockOSThread()
		var i int64
		var v *int64
		//startNanos[i] = time.Now().UnixNano()
		startNanos[i] = nanotime()
		for ring.Get(&v) {
			//endNanos[i] = time.Now().UnixNano()
			endNanos[i] = nanotime()
			i++
			b.SetBytes(1)

			if i < n {
				//startNanos[i] = time.Now().UnixNano()
				startNanos[i] = nanotime()
			}
		}
		wg.Done()
	}(int64(b.N))

	wg.Wait()

	b.StopTimer()
	recordLatencyDistribution("onering", b.N, startNanos, endNanos)
}

func Benchmark1Producer1ConsumerOneRing2(b *testing.B) {
	bench := hrtime.NewBenchmarkTSC(b.N)
	var numbers = mknumslice(b.N)
	var ring = onering.New{Size:8192}.SPSC()
	var wg sync.WaitGroup
	wg.Add(2)
	go func(n int) {
		runtime.LockOSThread()
		for i := 0; i < n; i++ {
			ring.Put(&numbers[i])
		}
		ring.Close()
		wg.Done()
	}(b.N)

	b.ResetTimer()
	go func(n int64) {
		runtime.LockOSThread()
		var v *int64
		for bench.Next() {
			ring.Get(&v)
			b.SetBytes(1)
		}
		wg.Done()
	}(int64(b.N))

	wg.Wait()

	b.StopTimer()
	recordLatencyDistributionBenchmark("onering-hrtime", bench)
}

func BenchmarkNanotimeOverhead(b *testing.B) {
	startNanos := make([]int64, b.N)
	endNanos := make([]int64, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		startNanos[i] = nanotime()
		endNanos[i] = nanotime()
	}

	b.StopTimer()
	recordLatencyDistribution("nanotime", b.N, startNanos, endNanos)
}

func BenchmarkHrtimeOverhead(b *testing.B) {
	bench := hrtime.NewBenchmarkTSC(b.N)

	b.ResetTimer()
	for bench.Next() {
	}

	b.StopTimer()
	recordLatencyDistributionBenchmark("hrtime", bench)
}

func recordLatencyDistribution(name string, count int, startNanos []int64, endNanos []int64) {
	histogram := hdrhistogram.New(1, 1000000, 5)
	for i := 0; i < count; i++ {
		diff := endNanos[i] - startNanos[i]
		histogram.RecordValue(diff)
	}
	histwriter.WriteDistributionFile(histogram, nil, 1.0, "../results/"+name+".histogram")
}

func recordLatencyDistributionBenchmark(name string, bench *hrtime.BenchmarkTSC) {
	histogram := hdrhistogram.New(1, 1000000, 5)
	for _, lap := range bench.Laps() {
		histogram.RecordValue(int64(lap))
	}
	histwriter.WriteDistributionFile(histogram, nil, 1.0, "../results/"+name+".histogram")
}
