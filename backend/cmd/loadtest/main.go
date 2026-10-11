// 命令 loadtest 是人工使用的压测工具，不进 CI（开发方案 §2.2）。
// 用法：go run ./cmd/loadtest -url http://127.0.0.1:8080/api/v1/health -c 50 -n 2000
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	url := flag.String("url", "http://127.0.0.1:8080/api/v1/health", "目标地址")
	concurrency := flag.Int("c", 50, "并发数")
	total := flag.Int("n", 2000, "总请求数")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}
	var (
		failures  atomic.Int64
		latencies = make([]time.Duration, 0, *total)
		latMu     sync.Mutex
		wg        sync.WaitGroup
	)

	start := time.Now()
	jobs := make(chan struct{})
	for i := 0; i < *concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range jobs {
				begin := time.Now()
				resp, err := client.Get(*url)
				elapsed := time.Since(begin)
				if err != nil {
					failures.Add(1)
					continue
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode >= 400 {
					failures.Add(1)
				}
				latMu.Lock()
				latencies = append(latencies, elapsed)
				latMu.Unlock()
			}
		}()
	}

	for i := 0; i < *total; i++ {
		jobs <- struct{}{}
	}
	close(jobs)
	wg.Wait()

	took := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	fmt.Printf("请求数 %d  并发 %d  耗时 %s\n", *total, *concurrency, took.Round(time.Millisecond))
	fmt.Printf("吞吐 %.1f req/s  失败 %d  错误率 %.3f%%\n",
		float64(*total)/took.Seconds(), failures.Load(),
		float64(failures.Load())*100/float64(*total))
	if len(latencies) > 0 {
		fmt.Printf("延迟 P50 %s  P95 %s  P99 %s  Max %s\n",
			percentile(latencies, 0.50), percentile(latencies, 0.95),
			percentile(latencies, 0.99), latencies[len(latencies)-1])
	}

	if failures.Load() > 0 {
		os.Exit(1)
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p * float64(len(sorted)-1))
	return sorted[idx]
}
