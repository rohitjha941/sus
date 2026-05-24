package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/rohitjha941/sus"
	"github.com/NVIDIA/go-nvml/pkg/nvml"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	pinVoltage = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "astral_pin_voltage_volts",
			Help: "Per-pin voltage reading from the Astral 12V-2x6 connector (IT8915FN I2C).",
		},
		[]string{"pin_index"},
	)
	pinCurrent = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "astral_pin_current_amps",
			Help: "Per-pin current reading from the Astral 12V-2x6 connector (IT8915FN I2C).",
		},
		[]string{"pin_index"},
	)
	pinPower = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "astral_pin_power_watts",
			Help: "Per-pin power draw (voltage × current) from the Astral 12V-2x6 connector.",
		},
		[]string{"pin_index"},
	)
	pinPowerTotal = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "astral_pin_power_total_watts",
			Help: "Total power draw across all 6 pins (sum of per-pin voltage × current).",
		},
	)
	pinPowerMin = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "astral_pin_power_min_watts",
			Help: "Minimum per-pin power draw.",
		},
	)
	pinPowerMax = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "astral_pin_power_max_watts",
			Help: "Maximum per-pin power draw.",
		},
	)
	pinPowerRatio = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "astral_pin_power_ratio",
			Help: "Ratio of min to max per-pin power draw (1.0 = perfectly balanced).",
		},
	)
	totalLoad = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "astral_total_load_watts",
			Help: "Total GPU power draw as reported by NVML (nvidia-smi equivalent).",
		},
	)
	exporterScrapeErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "astral_pin_exporter_scrape_errors_total",
			Help: "Total number of failed scrape attempts.",
		},
	)
)

func init() {
	prometheus.MustRegister(
		pinVoltage, pinCurrent, pinPower,
		pinPowerTotal, pinPowerMin, pinPowerMax, pinPowerRatio,
		totalLoad, exporterScrapeErrors,
	)
}

func main() {
	port := "9401"
	if v := os.Getenv("PORT"); v != "" {
		port = v
	}

	ret := nvml.Init()
	if ret != nvml.SUCCESS {
		log.Fatalf("nvmlInit failed: %v", ret)
	}
	defer nvml.Shutdown()

	devices, err := sus.FindAstralDevices()
	if err != nil {
		log.Fatalf("FindAstralDevices failed: %v", err)
	}
	if len(devices) == 0 {
		log.Fatal("No Astral RTX 5090 devices found. Exiting.")
	}

	log.Printf("Found %d Astral device(s): %s", len(devices), devices[0].Identifier())

	go func() {
		for {
			collect(devices[0])
			time.Sleep(5 * time.Second)
		}
	}()

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	log.Printf("Astral pin exporter listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func collect(device sus.AstralDevice) {
	pins, err := sus.ReadAstralDevicePins(device)
	if err != nil {
		log.Printf("ReadAstralDevicePins failed: %v", err)
		exporterScrapeErrors.Inc()
		return
	}

	load, err := sus.ReadAstralDeviceLoad(device)
	if err != nil {
		log.Printf("ReadAstralDeviceLoad failed: %v", err)
		exporterScrapeErrors.Inc()
		return
	}

	totalDraw := 0.0
	upperDraw := 0.0
	lowerDraw := 1e6

	for i, pin := range pins {
		v := pin.Voltage()
		c := pin.Current()
		d := pin.Drawing()

		idx := fmt.Sprintf("%d", i)
		pinVoltage.WithLabelValues(idx).Set(v)
		pinCurrent.WithLabelValues(idx).Set(c)
		pinPower.WithLabelValues(idx).Set(d)

		if d > upperDraw {
			upperDraw = d
		}
		if d < lowerDraw {
			lowerDraw = d
		}
		totalDraw += d
	}

	ratio := lowerDraw / upperDraw

	pinPowerTotal.Set(totalDraw)
	pinPowerMin.Set(lowerDraw)
	pinPowerMax.Set(upperDraw)
	pinPowerRatio.Set(ratio)
	totalLoad.Set(load)
}
