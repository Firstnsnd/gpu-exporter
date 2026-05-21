//go:build linux

package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vaniot-s/nvml"
)

var addr = flag.String("web.listen-address", ":9445", "Address to listen on for web interface and telemetry.")

func main() {
	flag.Parse()

	if err := nvml.Init(); err != nil {
		log.Fatalf("Couldn't initialize nvml: %v. Make sure NVML is in the shared library search path.", err)
	}
	defer nvml.Shutdown()

	prometheus.MustRegister(NewCollector())

	log.Printf("Starting GPU exporter on %s", *addr)
	log.Fatalf("ListenAndServe error: %v", http.ListenAndServe(*addr, promhttp.Handler()))
}
