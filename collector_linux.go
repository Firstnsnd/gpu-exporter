//go:build linux

package main

import (
	"strconv"

	ps "github.com/vaniot-s/go-ps"
	"github.com/vaniot-s/nvml"
)

// NewCollector creates a Collector with real NVML and process lookup backends.
func NewCollector() *Collector {
	return newCollector(&realNVMLClient{}, &realProcessFinder{})
}

// --- Concrete NVML implementation ---

type realNVMLClient struct{}

func (c *realNVMLClient) GetDeviceCount() (uint, error) {
	return nvml.GetDeviceCount()
}

func (c *realNVMLClient) NewDevice(idx uint) (NVMLDevice, error) {
	dev, err := nvml.NewDevice(idx)
	if err != nil {
		return nil, err
	}
	return &realNVMLDevice{dev: dev}, nil
}

type realNVMLDevice struct {
	dev *nvml.Device
}

func (d *realNVMLDevice) GetMinor() string      { return strconv.Itoa(int(*d.dev.Minor)) }
func (d *realNVMLDevice) GetUUID() string       { return d.dev.UUID }
func (d *realNVMLDevice) GetModel() string      { return *d.dev.Model }
func (d *realNVMLDevice) GetTotalMemory() float64 { return float64(*d.dev.Memory) }

func (d *realNVMLDevice) Status() (*GPUDeviceStatus, error) {
	s, err := d.dev.Status()
	if err != nil {
		return nil, err
	}
	return &GPUDeviceStatus{
		UsedMemory:  float64(*s.Memory.Global.Used),
		DutyCycle:   float64(*s.Utilization.GPU),
		PowerUsage:  float64(*s.Power),
		Temperature: float64(*s.Temperature),
		EncUtil:     float64(*s.Utilization.Encoder),
		DecUtil:     float64(*s.Utilization.Decoder),
	}, nil
}

func (d *realNVMLDevice) GetGraphicsRunningProcesses() ([]uint, []uint64, error) {
	return d.dev.GetGraphicsRunningProcesses()
}

func (d *realNVMLDevice) GetProcessUtilization() ([]GPUProcessUtilization, error) {
	samples, err := d.dev.GetProcessUtilization()
	if err != nil {
		return nil, err
	}
	result := make([]GPUProcessUtilization, len(samples))
	for i, s := range samples {
		result[i] = GPUProcessUtilization{
			PID:     s.PID,
			DecUtil: s.DecUtil,
			EncUtil: s.EncUtil,
			MemUtil: s.MemUtil,
			SmUtil:  s.SmUtil,
		}
	}
	return result, nil
}

// --- Concrete process finder ---

type realProcessFinder struct{}

func (f *realProcessFinder) FindProcess(pid int) (ProcessInfo, error) {
	return ps.FindProcess(pid)
}
