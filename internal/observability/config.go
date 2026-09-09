package observability

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	LogLevel    string
	Metrics     bool
	MetricsPort int
}

func ConfigFromEnv() (Config, error) {
	level := strings.TrimSpace(os.Getenv("LANDLOCK_GENPROF_LOG_LEVEL"))
	if level == "" {
		level = DefaultLogLevel
	}
	if _, err := ParseLevel(level); err != nil {
		return Config{}, err
	}
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("LANDLOCK_GENPROF_METRICS_ENABLED")), "true")
	port := 0
	if raw := strings.TrimSpace(os.Getenv("LANDLOCK_GENPROF_METRICS_PORT")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("invalid metrics port")
		}
		port = parsed
	}
	if enabled && port == 0 {
		return Config{}, fmt.Errorf("metrics port is required when metrics are enabled")
	}
	return Config{LogLevel: level, Metrics: enabled, MetricsPort: port}, nil
}

func NewFromEnv(out io.Writer) (*Logger, *Metrics, Config, error) {
	config, err := ConfigFromEnv()
	if err != nil {
		return nil, nil, Config{}, err
	}
	logger, err := NewLogger(out, config.LogLevel)
	if err != nil {
		return nil, nil, Config{}, err
	}
	return logger, NewMetrics(), config, nil
}
