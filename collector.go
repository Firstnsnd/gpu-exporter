package main

import (
	"log"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "nvidia_gpu"

	orphanContainer = "unknown"
	orphanNamespace = "unknown"
	orphanPod       = "unknown"
)

var (
	labels  = []string{"minor_number", "uuid", "name"}
	plabels = []string{"minor_number", "pod_name", "container", "namespace"}
)

// --- Interfaces for testability ---

type NVMLClient interface {
	GetDeviceCount() (uint, error)
	NewDevice(idx uint) (NVMLDevice, error)
}

type NVMLDevice interface {
	GetMinor() string
	GetUUID() string
	GetModel() string
	GetTotalMemory() float64
	Status() (*GPUDeviceStatus, error)
	GetGraphicsRunningProcesses() ([]uint, []uint64, error)
	GetProcessUtilization() ([]GPUProcessUtilization, error)
}

type GPUDeviceStatus struct {
	UsedMemory  float64
	DutyCycle   float64
	PowerUsage  float64
	Temperature float64
	EncUtil     float64
	DecUtil     float64
}

type GPUProcessUtilization struct {
	PID     uint
	DecUtil uint
	EncUtil uint
	MemUtil uint
	SmUtil  uint
}

type ProcessFinder interface {
	FindProcess(pid int) (ProcessInfo, error)
}

type ProcessInfo interface {
	Executable() string
}

// --- Collector ---

type Collector struct {
	sync.Mutex
	nvmlClient  NVMLClient
	procFinder  ProcessFinder
	numDevices  prometheus.Gauge
	usedMemory  *prometheus.GaugeVec
	totalMemory *prometheus.GaugeVec
	dutyCycle   *prometheus.GaugeVec
	powerUsage  *prometheus.GaugeVec
	temperature *prometheus.GaugeVec
	encUtil     *prometheus.GaugeVec
	decUtil     *prometheus.GaugeVec
	pUsedMemory *prometheus.GaugeVec
	pDecUtil    *prometheus.GaugeVec
	pEncUtil    *prometheus.GaugeVec
	pMemUtil    *prometheus.GaugeVec
	pSmUtil     *prometheus.GaugeVec
	allMetrics  []*prometheus.GaugeVec
	allPMetrics []*prometheus.GaugeVec
}

func newGaugeVec(name, help string, labels []string) *prometheus.GaugeVec {
	return prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      name,
			Help:      help,
		},
		labels,
	)
}

func newCollector(nvmlClient NVMLClient, procFinder ProcessFinder) *Collector {
	c := &Collector{
		nvmlClient: nvmlClient,
		procFinder: procFinder,
		numDevices: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "num_devices",
				Help:      "Number of GPU devices",
			},
		),
		usedMemory:   newGaugeVec("memory_used_bytes", "Memory used by the GPU device in bytes", labels),
		totalMemory:  newGaugeVec("memory_total_bytes", "Total memory of the GPU device in bytes", labels),
		dutyCycle:    newGaugeVec("duty_cycle", "Percent of time over the past sample period during which one or more kernels were executing on the GPU device", labels),
		powerUsage:   newGaugeVec("power_usage_milliwatts", "Power usage of the GPU device in milliwatts", labels),
		temperature:  newGaugeVec("temperature_celsius", "Temperature of the GPU device in celsius", labels),
		encUtil:      newGaugeVec("encoder_utilization", "Encoder utilization of the GPU device in percent", labels),
		decUtil:      newGaugeVec("decoder_utilization", "Decoder utilization of the GPU device in percent", labels),
		pUsedMemory:  newGaugeVec("process_memory_used_bytes", "Memory used by GPU process in bytes", plabels),
		pDecUtil:     newGaugeVec("process_decoder_utilization", "Decoder utilization of GPU process in percent", plabels),
		pEncUtil:     newGaugeVec("process_encoder_utilization", "Encoder utilization of GPU process in percent", plabels),
		pMemUtil:     newGaugeVec("process_memory_utilization", "Memory utilization of GPU process in percent", plabels),
		pSmUtil:      newGaugeVec("process_sm_utilization", "SM utilization of GPU process in percent", plabels),
	}
	c.allMetrics = []*prometheus.GaugeVec{
		c.usedMemory, c.totalMemory, c.dutyCycle,
		c.powerUsage, c.temperature, c.encUtil, c.decUtil,
	}
	c.allPMetrics = []*prometheus.GaugeVec{
		c.pUsedMemory, c.pDecUtil, c.pEncUtil, c.pMemUtil, c.pSmUtil,
	}
	return c
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.numDevices.Desc()
	for _, m := range append(c.allMetrics, c.allPMetrics...) {
		m.Describe(ch)
	}
}

// parseContainerInfo parses process name in format "container@namespace/pod"
// and returns (container, namespace, pod). Returns false if format is invalid.
func parseContainerInfo(pName string) (container, namespace, pod string, ok bool) {
	at := strings.Index(pName, "@")
	if at < 0 {
		return "", "", "", false
	}
	slash := strings.Index(pName[at:], "/")
	if slash < 0 {
		return "", "", "", false
	}
	slash += at
	container = pName[:at]
	namespace = pName[at+1 : slash]
	pod = strings.TrimSpace(pName[slash+1:])
	return container, namespace, pod, true
}

type pidMeta struct {
	container, namespace, pod string
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.Lock()
	defer c.Unlock()

	for _, m := range append(c.allMetrics, c.allPMetrics...) {
		m.Reset()
	}

	numDevices, err := c.nvmlClient.GetDeviceCount()
	if err != nil {
		log.Printf("DeviceCount() error: %v", err)
		return
	}
	c.numDevices.Set(float64(numDevices))
	ch <- c.numDevices

	for i := 0; i < int(numDevices); i++ {
		dev, err := c.nvmlClient.NewDevice(uint(i))
		if err != nil {
			log.Printf("DeviceHandleByIndex(%d) error: %v", i, err)
			continue
		}

		minor := dev.GetMinor()
		uuid := dev.GetUUID()
		name := dev.GetModel()
		lv := []string{minor, uuid, name}

		c.totalMemory.WithLabelValues(lv...).Set(dev.GetTotalMemory())

		devStatus, err := dev.Status()
		if err != nil {
			log.Printf("Status() error for device %s: %v", uuid, err)
			continue
		}

		c.usedMemory.WithLabelValues(lv...).Set(devStatus.UsedMemory)
		c.dutyCycle.WithLabelValues(lv...).Set(devStatus.DutyCycle)
		c.powerUsage.WithLabelValues(lv...).Set(devStatus.PowerUsage)
		c.temperature.WithLabelValues(lv...).Set(devStatus.Temperature)
		c.encUtil.WithLabelValues(lv...).Set(devStatus.EncUtil)
		c.decUtil.WithLabelValues(lv...).Set(devStatus.DecUtil)

		pids, mem, err := dev.GetGraphicsRunningProcesses()
		if err != nil {
			log.Printf("GetGraphicsRunningProcesses() error: %v", err)
			continue
		}

		pidInfo := make(map[int]pidMeta)
		for idx, pid := range pids {
			p, err := c.procFinder.FindProcess(int(pid))
			if err != nil || p == nil {
				log.Printf("FindProcess(%d) failed, recording as orphan", pid)
				pidInfo[int(pid)] = pidMeta{orphanContainer, orphanNamespace, orphanPod}
				c.pUsedMemory.WithLabelValues(minor, orphanPod, orphanContainer, orphanNamespace).
					Set(float64(mem[idx]))
				continue
			}
			container, namespace, pod, ok := parseContainerInfo(p.Executable())
			if !ok {
				log.Printf("Unexpected process name format for PID %d: %s", pid, p.Executable())
				pidInfo[int(pid)] = pidMeta{orphanContainer, orphanNamespace, orphanPod}
				c.pUsedMemory.WithLabelValues(minor, orphanPod, orphanContainer, orphanNamespace).
					Set(float64(mem[idx]))
				continue
			}
			pidInfo[int(pid)] = pidMeta{container, namespace, pod}
			c.pUsedMemory.WithLabelValues(minor, pod, container, namespace).Set(float64(mem[idx]))
		}

		processUtilization, err := dev.GetProcessUtilization()
		if err != nil {
			log.Printf("GetProcessUtilization() error: %v", err)
			continue
		}

		for _, pu := range processUtilization {
			if pu.PID == 0 {
				continue
			}
			info, ok := pidInfo[int(pu.PID)]
			if !ok {
				continue
			}
			c.pDecUtil.WithLabelValues(minor, info.pod, info.container, info.namespace).Set(float64(pu.DecUtil))
			c.pEncUtil.WithLabelValues(minor, info.pod, info.container, info.namespace).Set(float64(pu.EncUtil))
			c.pMemUtil.WithLabelValues(minor, info.pod, info.container, info.namespace).Set(float64(pu.MemUtil))
			c.pSmUtil.WithLabelValues(minor, info.pod, info.container, info.namespace).Set(float64(pu.SmUtil))
		}
	}

	for _, m := range c.allMetrics {
		m.Collect(ch)
	}
	for _, m := range c.allPMetrics {
		m.Collect(ch)
	}
}
