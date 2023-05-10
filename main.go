// Copyright 2021 CrowdStrike, Inc.
package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gorilla/handlers"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Build time information that is being set during compile time. See the
// Makefile for details.
var (
	Version = "none"
	GitHash = "none"
	Date    = "none"
)

// Logger is the global zap.SugaredLogger.
var Logger *zap.SugaredLogger

// Level is the log level used to configure the global Logger.
var Level = zap.NewAtomicLevel()

// init is used to configure and instanciate the Logger to ensure logging is
// available early.
func init() {
	logger := zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.Lock(os.Stdout),
		Level,
	))

	Logger = logger.Sugar()
	Logger.Infow("PromWatch starting",
		"version", Version,
		"githash", GitHash,
		"date", Date)
}

func main() {
	var configFile string
	flag.StringVar(&configFile, "config", "promwatch.yml", "Config file")
	flag.Parse()

	conf, err := loadConfig(configFile)
	dieOnError(err)

	Level.SetLevel(Levels.Get(conf.LogLevel))

	if len(conf.Collectors) == 0 {
		Logger.Warnf("No collectors defined, nothing to do.")
		os.Exit(0)
	}

	// Capacity never has to be larger than the number of collectors defined
	done := make(chan MetricCollector, len(conf.Collectors))
	collectors := map[CollectorID]*CollectorProc{}

	// Set up Prometheus metrics for PromWatch itself
	InitializeTelemetry()

	for _, c := range conf.Collectors {
		// We still want to go on starting other collectors in case any one is
		// invalid and can not be started.
		if !c.Valid() {
			Logger.Errorf("Invalid collector: %#v", c)
			continue
		}
		proc := c.Run()
		collectors[proc.ID] = proc
		// fan in messages from done channel
		go func() {
			d := <-proc.Done
			done <- d
			Logger.Warnf("collector %s was stopped, closing channels.", proc.ID)
			close(proc.Done)
		}()
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		Logger.Debug("metrics requested")
		// Print metrics collected from CloudWatch to the response
		for i, c := range collectors {
			Logger.Debugw("producing metrics for collector", "id", i)
			fmt.Fprint(w, c.Store.String())
		}

		// To avoid mixed uncompressed and compressed content compressions is
		// disabled here. The response will still be compressed as the whole
		// handler is being wrapped for compression.
		promhttp.HandlerFor(registry, promhttp.HandlerOpts{
			DisableCompression: true,
		}).ServeHTTP(w, r)
	})

	s := &http.Server{
		Addr:              conf.Listen,
		Handler:           handlers.CompressHandler(mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Second,
		WriteTimeout:      2 * time.Second,
		IdleTimeout:       30 * time.Second,
	}

	dieOnError(s.ListenAndServe())
}

func dieOnError(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
