package main

import (
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func BenchmarkParseContainerInfo(b *testing.B) {
	inputs := []string{
		"container-name@kube-system/pod-name-1234",
		"invalid-name-no-delimiters",
		"@/",
		"c@ns/pod",
		"",
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseContainerInfo(inputs[i%len(inputs)])
	}
}

func BenchmarkCollect_SingleDevice(b *testing.B) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "NVIDIA Tesla V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 250000, Temperature: 65, EncUtil: 10, DecUtil: 20},
				pids:        makePids(10),
				mems:        makeMems(10),
				procUtil:    makeProcUtil(10),
			},
		},
	}
	finder := makeMockFinder(10)
	c := makeTestCollector(client, finder)
	ch := make(chan prometheus.Metric, 256)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Collect(ch)
		drainChannel(ch)
	}
}

func BenchmarkCollect_MultipleDevices(b *testing.B) {
	numDevices := 8
	devices := make([]mockNVMLDevice, numDevices)
	for i := 0; i < numDevices; i++ {
		devices[i] = mockNVMLDevice{
			minor:       fmt.Sprintf("%d", i),
			uuid:        fmt.Sprintf("gpu-%d", i),
			model:       "NVIDIA Tesla V100",
			totalMemory: 16384,
			status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 250000, Temperature: 65, EncUtil: 10, DecUtil: 20},
			pids:        makePids(10),
			mems:        makeMems(10),
			procUtil:    makeProcUtil(10),
		}
	}
	client := &mockNVMLClient{deviceCount: uint(numDevices), devices: devices}
	finder := makeMockFinder(numDevices * 10)
	c := makeTestCollector(client, finder)
	ch := make(chan prometheus.Metric, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Collect(ch)
		drainChannel(ch)
	}
}

func BenchmarkCollect_ManyProcesses(b *testing.B) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "NVIDIA Tesla V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 250000, Temperature: 65, EncUtil: 10, DecUtil: 20},
				pids:        makePids(100),
				mems:        makeMems(100),
				procUtil:    makeProcUtil(100),
			},
		},
	}
	finder := makeMockFinder(100)
	c := makeTestCollector(client, finder)
	ch := make(chan prometheus.Metric, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Collect(ch)
		drainChannel(ch)
	}
}

func BenchmarkCollect_WithOrphans(b *testing.B) {
	pids := makePids(50)
	mems := makeMems(50)

	// Only first 30 processes can be found, rest are orphans
	finder := &mockProcessFinder{
		processes: make(map[int]*mockProcessInfo),
	}
	for i := 0; i < 30; i++ {
		finder.processes[int(pids[i])] = &mockProcessInfo{
			executable: fmt.Sprintf("c%d@ns%d/pod%d", i, i, i),
		}
	}

	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "NVIDIA Tesla V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 250000, Temperature: 65, EncUtil: 10, DecUtil: 20},
				pids:     pids,
				mems:    mems,
				procUtil: makeProcUtil(50),
			},
		},
	}
	c := makeTestCollector(client, finder)
	ch := make(chan prometheus.Metric, 1024)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Collect(ch)
		drainChannel(ch)
	}
}

// --- Bench helpers ---

func makePids(n int) []uint {
	pids := make([]uint, n)
	for i := 0; i < n; i++ {
		pids[i] = uint(1000 + i)
	}
	return pids
}

func makeMems(n int) []uint64 {
	mems := make([]uint64, n)
	for i := 0; i < n; i++ {
		mems[i] = uint64(100 * (i + 1))
	}
	return mems
}

func makeProcUtil(n int) []GPUProcessUtilization {
	utils := make([]GPUProcessUtilization, n)
	for i := 0; i < n; i++ {
		utils[i] = GPUProcessUtilization{
			PID:     uint(1000 + i),
			SmUtil:  uint(i * 2),
			MemUtil: uint(i),
			EncUtil: uint(i / 2),
			DecUtil: uint(i / 3),
		}
	}
	return utils
}

func makeMockFinder(n int) *mockProcessFinder {
	finder := &mockProcessFinder{
		processes: make(map[int]*mockProcessInfo),
	}
	for i := 0; i < n; i++ {
		pid := 1000 + i
		finder.processes[pid] = &mockProcessInfo{
			executable: fmt.Sprintf("c%d@ns%d/pod%d", i, i, i),
		}
	}
	return finder
}

func drainChannel(ch chan prometheus.Metric) {
	for len(ch) > 0 {
		<-ch
	}
}
