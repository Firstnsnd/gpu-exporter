package main

import "C"
import (
	"flag"
	"fmt"
	"github.com/mindprince/gonvml"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"./nvidia-nvml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ps "github.com/vaniot-s/go-ps"
)

const (
	namespace = "nvidia_gpu"
)

var (
	addr = flag.String("web.listen-address", ":9445", "Address to listen on for web interface and telemetry.")

	labels            = []string{"minor_number", "uuid", "name"}
	plabels           = []string{"minor_number", "pod_name", "container", "namespace"}
	isFanSpeedEnabled = true
)

type Collector struct {
	sync.Mutex
	numDevices  prometheus.Gauge
	usedMemory  *prometheus.GaugeVec
	totalMemory *prometheus.GaugeVec
	dutyCycle   *prometheus.GaugeVec
	powerUsage  *prometheus.GaugeVec
	temperature *prometheus.GaugeVec
	fanSpeed    *prometheus.GaugeVec
	pUsedMemory *prometheus.GaugeVec
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
		fanSpeed: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "fanspeed_percent",
				Help:      "Fanspeed of the GPU device as a percent of its maximum",
			},
			labels,
		),
		pUsedMemory: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: namespace,
				Name:      "process",
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
	c.fanSpeed.Describe(ch)

	c.pUsedMemory.Describe(ch)
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
	c.fanSpeed.Reset()

	c.pUsedMemory.Reset()

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


		devStatus,err:=dev.Status()

		c.usedMemory.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Memory.Global.Used))

		dutyCycle, _, err := dev.UtilizationRates()
		if err != nil {
			log.Printf("UtilizationRates() error: %v", err)
		} else {
			c.dutyCycle.WithLabelValues(minor, uuid, name).Set(float64(dutyCycle))
		}


		c.powerUsage.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Power))
		c.temperature.WithLabelValues(minor, uuid, name).Set(float64(*devStatus.Temperature))


		//pids,mems,err := dev.GetGraphicsRunningProcesses()
		//if err != nil {
		//	log.Printf("process error: %v", err)
		//} else {
		//	for _, cproc := range grap {
		//		pid := cproc.PID()
		//		usedGpuMemory := cproc.Memory()
		//		p, err := ps.FindProcess(int(pid))
		//		if err != nil {
		//			fmt.Println("Error : ", err)
		//			os.Exit(-1)
		//		}
		//		pName := p.Executable()
		//		at := strings.Index(pName, "@")
		//		slash := strings.Index(pName, "/")
		//		container := pName[0:at]
		//		nameSpace := pName[at+1 : slash]
		//		pod := strings.Trim(string(pName[slash+1:len(pName)-1]), " ")
		//		c.pUsedMemory.WithLabelValues(minor, pod, container, nameSpace).Set(float64(usedGpuMemory))
		//	}
		//}

		if isFanSpeedEnabled {
			fanSpeed, err := dev.FanSpeed()
			if err != nil {
				log.Printf("FanSpeed() error: %v", err)
				isFanSpeedEnabled = false
			} else {
				c.fanSpeed.WithLabelValues(minor, uuid, name).Set(float64(fanSpeed))
			}

		}

	}
	c.usedMemory.Collect(ch)
	c.totalMemory.Collect(ch)
	c.dutyCycle.Collect(ch)
	c.powerUsage.Collect(ch)
	c.temperature.Collect(ch)
	c.fanSpeed.Collect(ch)

	c.pUsedMemory.Collect(ch)
}

func main() {
	flag.Parse()

	// 	clock,err := dev.Clock()
	// 	log.printf(clock)
	if err := nvml.Initialize(); err != nil {
		log.Fatalf("Couldn't initialize nvml: %v. Make sure NVML is in the shared library search path.", err)
	}
	defer nvml.Shutdown()

	if driverVersion, err := nvml.SystemDriverVersion(); err != nil {
		log.Printf("SystemDriverVersion() error: %v", err)
	} else {
		log.Printf("SystemDriverVersion(): %v", driverVersion)
	}

	prometheus.MustRegister(NewCollector())

	// Serve on all paths under addr
	log.Fatalf("ListenAndServe error: %v", http.ListenAndServe(*addr, promhttp.Handler()))
}
