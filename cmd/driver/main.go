package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/klog/v2"

	"github.com/ursweiss/simple-local-path-provisioner/pkg/driver"
)

func main() {
	cfg := &driver.Config{}
	var showVersion bool

	flag.StringVar(&cfg.DriverName, "driver-name",
		getEnv("DRIVER_NAME", "simple-local-path.csi.whity.ch"),
		"CSI driver name")
	flag.StringVar(&cfg.Endpoint, "endpoint",
		getEnv("CSI_ENDPOINT", "unix:///csi/csi.sock"),
		"CSI gRPC endpoint")
	flag.StringVar(&cfg.NodeID, "node-id",
		getEnv("NODE_ID", ""),
		"Node identifier (required in node mode)")
	flag.StringVar(&cfg.BasePath, "base-path",
		getEnv("BASE_PATH", "/srv/k3d-persistent-volumes"),
		"Base directory for volume backing directories on the host")
	flag.StringVar(&cfg.Mode, "mode",
		getEnv("MODE", "controller"),
		"Driver mode: controller or node")
	flag.DurationVar(&cfg.StaleTimeout, "stale-timeout",
		mustParseDuration(getEnv("STALE_TIMEOUT", "1m")),
		"Duration after which a stale publication lock may be reclaimed (0 = disabled)")
	flag.BoolVar(&cfg.AllowForceTakeover, "allow-force-takeover",
		getEnvBool("ALLOW_FORCE_TAKEOVER", false),
		"Allow force takeover of any publication lock regardless of stale timeout")
	flag.IntVar(&cfg.LogLevel, "log-level", 2, "klog verbosity level")
	flag.BoolVar(&showVersion, "version", false, "Print version and exit")

	klog.InitFlags(nil)
	flag.Parse()

	if showVersion {
		fmt.Printf("simple-local-path-provisioner %s\n", driver.Version())
		os.Exit(0)
	}

	klog.V(1).Infof("Starting simple-local-path-provisioner: driver=%s mode=%s basePath=%s",
		cfg.DriverName, cfg.Mode, cfg.BasePath)

	d := driver.New(cfg)

	stopCh := make(chan os.Signal, 1)
	signal.Notify(stopCh,
		syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP, syscall.SIGQUIT)

	if err := d.Run(stopCh); err != nil {
		klog.Fatalf("Driver exited with error: %v", err)
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	switch os.Getenv(key) {
	case "true", "1":
		return true
	case "false", "0":
		return false
	default:
		return defaultValue
	}
}

func mustParseDuration(s string) time.Duration {
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Minute
	}
	return d
}
