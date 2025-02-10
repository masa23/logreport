package exporter

import (
	"context"
	"time"
)

type Metric struct {
	Timestamp time.Time
	Key       string
	Value     any // int64 or float64
}

type Exporter interface {
	Export(ctx context.Context, metrics []*Metric) error
}
