package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// --- Mock types ---

type mockNVMLClient struct {
	deviceCount    uint
	deviceCountErr error
	devices        []mockNVMLDevice
}

func (m *mockNVMLClient) GetDeviceCount() (uint, error) {
	return m.deviceCount, m.deviceCountErr
}

func (m *mockNVMLClient) NewDevice(idx uint) (NVMLDevice, error) {
	if int(idx) >= len(m.devices) {
		return nil, fmt.Errorf("device index %d out of range", idx)
	}
	return &m.devices[idx], nil
}

type mockNVMLDevice struct {
	minor       string
	uuid        string
	model       string
	totalMemory float64
	status      *GPUDeviceStatus
	statusErr   error
	pids        []uint
	mems        []uint64
	procsErr    error
	procUtil    []GPUProcessUtilization
	procUtilErr error
}

func (d *mockNVMLDevice) GetMinor() string      { return d.minor }
func (d *mockNVMLDevice) GetUUID() string       { return d.uuid }
func (d *mockNVMLDevice) GetModel() string      { return d.model }
func (d *mockNVMLDevice) GetTotalMemory() float64 { return d.totalMemory }

func (d *mockNVMLDevice) Status() (*GPUDeviceStatus, error) {
	return d.status, d.statusErr
}

func (d *mockNVMLDevice) GetGraphicsRunningProcesses() ([]uint, []uint64, error) {
	return d.pids, d.mems, d.procsErr
}

func (d *mockNVMLDevice) GetProcessUtilization() ([]GPUProcessUtilization, error) {
	return d.procUtil, d.procUtilErr
}

type mockProcessFinder struct {
	processes map[int]*mockProcessInfo
	errors    map[int]error
}

func (f *mockProcessFinder) FindProcess(pid int) (ProcessInfo, error) {
	if err, ok := f.errors[pid]; ok {
		return nil, err
	}
	if p, ok := f.processes[pid]; ok {
		return p, nil
	}
	return nil, nil
}

type mockProcessInfo struct {
	executable string
}

func (m *mockProcessInfo) Executable() string { return m.executable }

// --- Helpers ---

func collectMetrics(c *Collector) []prometheus.Metric {
	ch := make(chan prometheus.Metric, 256)
	c.Collect(ch)
	close(ch)
	var metrics []prometheus.Metric
	for m := range ch {
		metrics = append(metrics, m)
	}
	return metrics
}

func getMetricValue(m prometheus.Metric) float64 {
	var metric dto.Metric
	m.Write(&metric)
	return metric.GetGauge().GetValue()
}

func getMetricLabels(m prometheus.Metric) map[string]string {
	var metric dto.Metric
	m.Write(&metric)
	labels := make(map[string]string)
	for _, lp := range metric.GetLabel() {
		labels[lp.GetName()] = lp.GetValue()
	}
	return labels
}

func findMetricByName(metrics []prometheus.Metric, name string) []prometheus.Metric {
	var result []prometheus.Metric
	for _, m := range metrics {
		desc := m.Desc()
		if desc != nil && desc.String() != "" {
			var metric dto.Metric
			m.Write(&metric)
			// Match by checking the description contains the metric name
			result = append(result, m)
		}
	}
	_ = result
	return nil
}

func makeTestCollector(client NVMLClient, finder ProcessFinder) *Collector {
	return newCollector(client, finder)
}

// --- parseContainerInfo tests ---

func TestParseContainerInfo_ValidFormat(t *testing.T) {
	c, ns, pod, ok := parseContainerInfo("mycontainer@myns/mypod")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c != "mycontainer" {
		t.Errorf("container = %q, want %q", c, "mycontainer")
	}
	if ns != "myns" {
		t.Errorf("namespace = %q, want %q", ns, "myns")
	}
	if pod != "mypod" {
		t.Errorf("pod = %q, want %q", pod, "mypod")
	}
}

func TestParseContainerInfo_PodWithSpaces(t *testing.T) {
	_, _, pod, ok := parseContainerInfo("c@ns/pod name ")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if pod != "pod name" {
		t.Errorf("pod = %q, want %q", pod, "pod name")
	}
}

func TestParseContainerInfo_EmptyPod(t *testing.T) {
	c, ns, pod, ok := parseContainerInfo("c@ns/")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c != "c" || ns != "ns" || pod != "" {
		t.Errorf("got c=%q ns=%q pod=%q", c, ns, pod)
	}
}

func TestParseContainerInfo_EmptyContainer(t *testing.T) {
	c, _, _, ok := parseContainerInfo("@ns/pod")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if c != "" {
		t.Errorf("container = %q, want empty", c)
	}
}

func TestParseContainerInfo_EmptyNamespace(t *testing.T) {
	_, ns, _, ok := parseContainerInfo("c@/pod")
	if !ok {
		t.Fatal("expected ok=true")
	}
	if ns != "" {
		t.Errorf("namespace = %q, want empty", ns)
	}
}

func TestParseContainerInfo_MissingAt(t *testing.T) {
	_, _, _, ok := parseContainerInfo("container-ns/pod")
	if ok {
		t.Fatal("expected ok=false for missing @")
	}
}

func TestParseContainerInfo_MissingSlash(t *testing.T) {
	_, _, _, ok := parseContainerInfo("c@ns-pod")
	if ok {
		t.Fatal("expected ok=false for missing /")
	}
}

func TestParseContainerInfo_EmptyString(t *testing.T) {
	_, _, _, ok := parseContainerInfo("")
	if ok {
		t.Fatal("expected ok=false for empty string")
	}
}

// --- Collector.Collect tests ---

func TestCollect_NoDevices(t *testing.T) {
	client := &mockNVMLClient{deviceCount: 0}
	finder := &mockProcessFinder{}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric (numDevices), got %d", len(metrics))
	}
	val := getMetricValue(metrics[0])
	if val != 0 {
		t.Errorf("numDevices = %v, want 0", val)
	}
}

func TestCollect_SingleDevice(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor:       "0",
				uuid:        "gpu-uuid-0",
				model:       "NVIDIA Tesla V100",
				totalMemory: 16384,
				status: &GPUDeviceStatus{
					UsedMemory:  8192,
					DutyCycle:   50,
					PowerUsage:  250000,
					Temperature: 65,
					EncUtil:     10,
					DecUtil:     20,
				},
				pids: []uint{1001, 1002},
				mems: []uint64{4096, 2048},
				procUtil: []GPUProcessUtilization{
					{PID: 1001, SmUtil: 30, MemUtil: 40, EncUtil: 5, DecUtil: 10},
					{PID: 1002, SmUtil: 15, MemUtil: 20, EncUtil: 3, DecUtil: 7},
				},
			},
		},
	}
	finder := &mockProcessFinder{
		processes: map[int]*mockProcessInfo{
			1001: {executable: "container-a@kube-system/pod-1"},
			1002: {executable: "container-b@default/pod-2"},
		},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	// 1 (numDevices) + 7 (device metrics) + 2*5 (process metrics) = 18
	if len(metrics) != 18 {
		t.Fatalf("expected 18 metrics, got %d", len(metrics))
	}

	// Verify numDevices
	if v := getMetricValue(metrics[0]); v != 1 {
		t.Errorf("numDevices = %v, want 1", v)
	}
}

func TestCollect_MultipleDevices(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 2,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 100, DutyCycle: 10, PowerUsage: 100, Temperature: 50, EncUtil: 5, DecUtil: 5},
			},
			{
				minor: "1", uuid: "gpu-1", model: "V100",
				totalMemory: 32768,
				status:      &GPUDeviceStatus{UsedMemory: 200, DutyCycle: 20, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
			},
		},
	}
	c := makeTestCollector(client, &mockProcessFinder{})

	metrics := collectMetrics(c)

	// 1 (numDevices) + 7*2 (device metrics, no processes) = 15
	if len(metrics) != 15 {
		t.Fatalf("expected 15 metrics, got %d", len(metrics))
	}
}

func TestCollect_DeviceStatusError(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				statusErr:   errors.New("status error"),
			},
		},
	}
	c := makeTestCollector(client, &mockProcessFinder{})

	metrics := collectMetrics(c)

	// numDevices + totalMemory (set before Status() call) + remaining device metrics with zero values
	// from Collect(). totalMemory is set, then Status fails so other device metrics are skipped,
	// but allMetrics still gets Collected.
	if len(metrics) != 2 {
		t.Fatalf("expected 2 metrics (numDevices + totalMemory), got %d", len(metrics))
	}
}

func TestCollect_NewDeviceError(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices:     []mockNVMLDevice{}, // empty, so index 0 fails
	}
	c := makeTestCollector(client, &mockProcessFinder{})

	metrics := collectMetrics(c)

	// Only numDevices, device query fails
	if len(metrics) != 1 {
		t.Fatalf("expected 1 metric, got %d", len(metrics))
	}
}

func TestCollect_GetDeviceCountError(t *testing.T) {
	client := &mockNVMLClient{
		deviceCountErr: errors.New("no nvml"),
	}
	c := makeTestCollector(client, &mockProcessFinder{})

	metrics := collectMetrics(c)

	// No metrics at all — Collect returns early
	if len(metrics) != 0 {
		t.Fatalf("expected 0 metrics, got %d", len(metrics))
	}
}

func TestCollect_OrphanProcess_NilFromFinder(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
				pids:        []uint{9999},
				mems:        []uint64{4096},
				procUtil:    []GPUProcessUtilization{{PID: 9999, SmUtil: 50}},
			},
		},
	}
	// PID 9999 is not in processes map, so FindProcess returns nil
	finder := &mockProcessFinder{}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	// Should still emit process metrics with unknown labels
	found := false
	for _, m := range metrics {
		labels := getMetricLabels(m)
		if labels["namespace"] == "unknown" && labels["pod_name"] == "unknown" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphan process metric with unknown labels")
	}
}

func TestCollect_OrphanProcess_ErrorFromFinder(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
				pids:        []uint{8888},
				mems:        []uint64{2048},
				procUtil:    []GPUProcessUtilization{{PID: 8888, SmUtil: 30}},
			},
		},
	}
	finder := &mockProcessFinder{
		errors: map[int]error{8888: errors.New("process gone")},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	found := false
	for _, m := range metrics {
		labels := getMetricLabels(m)
		if labels["namespace"] == "unknown" && labels["container"] == "unknown" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphan process metric with unknown labels")
	}
}

func TestCollect_UnexpectedProcessName(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
				pids:        []uint{7777},
				mems:        []uint64{1024},
				procUtil:    []GPUProcessUtilization{{PID: 7777, SmUtil: 20}},
			},
		},
	}
	finder := &mockProcessFinder{
		processes: map[int]*mockProcessInfo{
			7777: {executable: "invalid-name-no-at-or-slash"},
		},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	found := false
	for _, m := range metrics {
		labels := getMetricLabels(m)
		if labels["namespace"] == "unknown" && labels["pod_name"] == "unknown" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphan process metric for unparseable name")
	}
}

func TestCollect_GetGraphicsRunningProcessesError(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
				procsErr:    errors.New("rpc error"),
			},
		},
	}
	c := makeTestCollector(client, &mockProcessFinder{})

	metrics := collectMetrics(c)

	// numDevices + 7 device metrics = 8, no process metrics
	if len(metrics) != 8 {
		t.Fatalf("expected 8 metrics, got %d", len(metrics))
	}
}

func TestCollect_GetProcessUtilizationError(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 8192, DutyCycle: 50, PowerUsage: 200, Temperature: 60, EncUtil: 10, DecUtil: 10},
				pids:        []uint{1001},
				mems:        []uint64{4096},
				procUtilErr: errors.New("util error"),
			},
		},
	}
	finder := &mockProcessFinder{
		processes: map[int]*mockProcessInfo{
			1001: {executable: "c@ns/pod"},
		},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	// numDevices + 7 device metrics + 1 process memory = 9, no utilization metrics
	if len(metrics) != 9 {
		t.Fatalf("expected 9 metrics, got %d", len(metrics))
	}
}

func TestCollect_ZeroPIDInUtilization(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 100, DutyCycle: 10, PowerUsage: 100, Temperature: 50, EncUtil: 5, DecUtil: 5},
				pids:        []uint{1001},
				mems:        []uint64{50},
				procUtil: []GPUProcessUtilization{
					{PID: 0, SmUtil: 99}, // should be skipped
					{PID: 1001, SmUtil: 30, MemUtil: 20, EncUtil: 5, DecUtil: 5},
				},
			},
		},
	}
	finder := &mockProcessFinder{
		processes: map[int]*mockProcessInfo{
			1001: {executable: "c@ns/pod"},
		},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	// numDevices(1) + device(7) + process memory(1) + process util(4) = 13
	if len(metrics) != 13 {
		t.Fatalf("expected 13 metrics, got %d", len(metrics))
	}
}

func TestCollect_UtilizationPIDNotInRunningProcesses(t *testing.T) {
	client := &mockNVMLClient{
		deviceCount: 1,
		devices: []mockNVMLDevice{
			{
				minor: "0", uuid: "gpu-0", model: "V100",
				totalMemory: 16384,
				status:      &GPUDeviceStatus{UsedMemory: 100, DutyCycle: 10, PowerUsage: 100, Temperature: 50, EncUtil: 5, DecUtil: 5},
				pids:        []uint{1001},
				mems:        []uint64{50},
				procUtil: []GPUProcessUtilization{
					{PID: 9999, SmUtil: 30}, // not in pids, should be skipped
				},
			},
		},
	}
	finder := &mockProcessFinder{
		processes: map[int]*mockProcessInfo{
			1001: {executable: "c@ns/pod"},
		},
	}
	c := makeTestCollector(client, finder)

	metrics := collectMetrics(c)

	// numDevices(1) + device(7) + process memory(1) = 9, no util for PID 9999
	if len(metrics) != 9 {
		t.Fatalf("expected 9 metrics, got %d", len(metrics))
	}
}
