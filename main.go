package main

import "C"
import (
	"flag"
	"os"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ps "github.com/vaniot-s/go-ps"
	"github.com/vaniot-s/nvml"
	"log"
	"net/http"
	"strconv"
	"sync"
)

const (
	namespace = "nvidia_gpu"
)

var (
	addr = flag.String("web.listen-address", ":9445", "Address to listen on for web interface and telemetry.")

	labels  = []string{"minor_number", "uuid", "name"}
	plabels = []string{"minor_number", "pod_name", "container", "namespace"}
)

type Collector struct {
	sync.Mutex
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
}

func NewCollector() *Collector {
	return &Collector{
		numDevices: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "num_devices",
				Help:      "Number of GPU devices",
			},
		),
		usedMemory: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_used_bytes",
				Help:      "Memory used by the GPU device in bytes",
			},
			labels,
		),
		totalMemory: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "memory_total_bytes",
				Help:      "Total memory of the GPU device in bytes",
			},
			labels,
		),
		dutyCycle: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "duty_cycle",
				Help:      "Percent of time over the past sample period during which one or more kernels were executing on the GPU device",
			},
			labels,
		),
		powerUsage: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "power_usage_milliwatts",
				Help:      "Power usage of the GPU device in milliwatts",
			},
			labels,
		),

		temperature: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "temperature_celsius",
				Help:      "Temperature of the GPU device in celsius",
			},
			labels,
		),
		encUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "encUtil",
				Help:      "encUtil of the GPU device",
			},
			labels,
		),
		decUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "encUtil",
				Help:      "encUtil of the GPU device",
			},
			labels,
		),
		pUsedMemory: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process_graph",
				Help:      "process of the GPU device ",
			},
			plabels,
		),
		pDecUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process_decutil",
				Help:      "process of the GPU device ",
			},
			plabels,
		),
		pEncUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process_encutil",
				Help:      "process of the GPU device ",
			},
			plabels,
		),
		pMemUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process_memutil",
				Help:      "process of the GPU device ",
			},
			plabels,
		),
		pSmUtil: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process_smutil",
				Help:      "process of the GPU device ",
			},
			plabels,
		),
	}
}

func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.numDevices.Desc()
	c.usedMemory.Describe(ch)
	c.totalMemory.Describe(ch)
	c.dutyCycle.Describe(ch)
	c.powerUsage.Describe(ch)
	c.temperature.Describe(ch)
	c.encUtil.Describe(ch)
	c.decUtil.Describe(ch)
	c.pUsedMemory.Describe(ch)
	c.pDecUtil.Describe(ch)
	c.pEncUtil.Describe(ch)
	c.pMemUtil.Describe(ch)
	c.pSmUtil.Describe(ch)
}

func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	// Only one Collect call in progress at a time.
	c.Lock()
	defer c.Unlock()

	c.usedMemory.Reset()
	c.totalMemory.Reset()
	c.dutyCycle.Reset()
	c.powerUsage.Reset()
	c.temperature.Reset()
	c.encUtil.Reset()
	c.decUtil.Reset()
	c.pUsedMemory.Reset()
	c.pDecUtil.Reset()
	c.pEncUtil.Reset()
	c.pMemUtil.Reset()
	c.pSmUtil.Reset()

	numDevices, err := nvml.GetDeviceCount()
	if err != nil {
		log.Printf("DeviceCount() error: %v", err)
		return
	} else {
		c.numDevices.Set(float64(numDevices))
		ch <- c.numDevices
	}

	for i := 0; i < int(numDevices); i++ {
		dev, err := nvml.NewDevice(uint(i))
		if err != nil {
			log.Printf("DeviceHandleByIndex(%d) error: %v", i, err)
			continue
		}

		minor := strconv.Itoa(int(*dev.Minor))
		uuid := dev.UUID
		name := *dev.Model

		totalMemory := int(*dev.Memory)

		c.totalMemory.WithLabelValues(minor, uuid, name).Set(float64(totalMemory))

		devStatus, err := dev.Status()

		c.usedMemory.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Memory.Global.Used))

		c.dutyCycle.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Utilization.GPU))

		c.powerUsage.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Power))

		c.temperature.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Temperature))
		c.encUtil.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Encoder))
		c.decUtil.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Decoder))
		//process graph
		pids, mem, err := dev.GetGraphicsRunningProcesses()
		if err != nil {
			log.Printf("GetGraphicsRunningProcesses()error: %v", err)
			continue
		} else {
			for i := 0; i < len(pids); i++ {
				p, err := ps.FindProcess(int(pids[i]))
				pName := p.Executable()
				if err != nil {
					log.Printf("Error : ", err)
					os.Exit(-1)
				}
				at := strings.Index(pName, "@")
				slash := strings.Index(pName, "/")
				container := pName[0:at]
				nameSpace := pName[at+1 : slash]
				pod := strings.Trim(string(pName[slash+1:len(pName)-1]), " ")
				c.pUsedMemory.WithLabelValues(minor, pod, container, nameSpace).Set(float64(mem[i]))
			}
		}

		// process unlization
		processUtilization, err := dev.GetProcessUtilization()
		if err != nil {
			log.Printf("GetProcessUtilization()error: %v", err)
			continue
		} else {
			storage := make([]nvml.ProcessUtilization, len(pids))
			for i := 0; i < len(storage); i++ {
				storage[i].PID = pids[i]
			}
			for j := 0; j < len(processUtilization); j++ {
				if int(processUtilization[j].PID) != 0 {
					for k := 0; k < len(storage); k++ {
						if int(storage[k].PID) == int(processUtilization[j].PID) {
							storage[k].DecUtil = processUtilization[j].DecUtil
							storage[k].EncUtil = processUtilization[j].EncUtil
							storage[k].MemUtil = processUtilization[j].MemUtil
							storage[k].SmUtil = processUtilization[j].SmUtil
						}
					}
				}
			}
			for l := 0; l < len(storage); l++ {
				p, err := ps.FindProcess(int(storage[l].PID))
				if err != nil {
					log.Printf("Error : ", err)
					os.Exit(-1)
				}
				pName := p.Executable()

				at := strings.Index(pName, "@")
				slash := strings.Index(pName, "/")
				container := pName[0:at]
				nameSpace := pName[at+1 : slash]
				pod := strings.Trim(string(pName[slash+1:len(pName)-1]), " ")
				c.pDecUtil.WithLabelValues(minor, pod, container, nameSpace).Set(float64(storage[l].DecUtil))
				c.pEncUtil.WithLabelValues(minor, pod, container, nameSpace).Set(float64(storage[l].EncUtil))
				c.pMemUtil.WithLabelValues(minor, pod, container, nameSpace).Set(float64(storage[l].MemUtil))
				c.pSmUtil.WithLabelValues(minor, pod, container, nameSpace).Set(float64(storage[l].SmUtil))
			}
		}
	}
	c.usedMemory.Collect(ch)
	c.totalMemory.Collect(ch)
	c.dutyCycle.Collect(ch)
	c.powerUsage.Collect(ch)
	c.temperature.Collect(ch)
	c.pUsedMemory.Collect(ch)
	c.pDecUtil.Collect(ch)
	c.pEncUtil.Collect(ch)
	c.pMemUtil.Collect(ch)
	c.pSmUtil.Collect(ch)
}

func main() {
	flag.Parse()

	// 	clock,err := dev.Clock()
	// 	log.printf(clock)
	if err := nvml.Init(); err != nil {
		log.Fatalf("Couldn't initialize nvml: %v. Make sure NVML is in the shared library search path.", err)
	}
	defer nvml.Shutdown()

	prometheus.MustRegister(NewCollector())

	// Serve on all paths under addr
	log.Fatalf("ListenAndServe error: %v", http.ListenAndServe(*addr, promhttp.Handler()))
}
