# GPU Exporter

Prometheus exporter for NVIDIA GPU metrics, designed for Kubernetes environments. Collects device-level and per-process GPU utilization metrics via NVML.

## Metrics

### Device-level

| Metric | Description |
|--------|-------------|
| `nvidia_gpu_num_devices` | Number of GPU devices |
| `nvidia_gpu_memory_used_bytes` | Memory used by GPU device |
| `nvidia_gpu_memory_total_bytes` | Total memory of GPU device |
| `nvidia_gpu_duty_cycle` | GPU compute utilization (%) |
| `nvidia_gpu_power_usage_milliwatts` | Power usage (mW) |
| `nvidia_gpu_temperature_celsius` | Temperature (°C) |
| `nvidia_gpu_encoder_utilization` | Encoder utilization (%) |
| `nvidia_gpu_decoder_utilization` | Decoder utilization (%) |

### Process-level

| Metric | Description |
|--------|-------------|
| `nvidia_gpu_process_memory_used_bytes` | Memory used by GPU process |
| `nvidia_gpu_process_sm_utilization` | SM utilization per process (%) |
| `nvidia_gpu_process_memory_utilization` | Memory utilization per process (%) |
| `nvidia_gpu_process_encoder_utilization` | Encoder utilization per process (%) |
| `nvidia_gpu_process_decoder_utilization` | Decoder utilization per process (%) |

Process metrics are labeled with `minor_number`, `pod_name`, `container`, `namespace`. When a process cannot be resolved (e.g., Pod deleted but GPU process remains), it is reported with `unknown` labels.

## Usage

### Docker

```bash
docker build -t gpu-exporter .
docker run --gpus all -p 9445:9445 gpu-exporter
```

### Binary

```bash
go build -o gpu-exporter .
./gpu-exporter --web.listen-address=:9445
```

Metrics endpoint: `http://localhost:9445/metrics`

## Test

```bash
go test -v ./...
go test -bench=. -benchmem ./...
```
