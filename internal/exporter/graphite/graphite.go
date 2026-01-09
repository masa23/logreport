package graphite

import (
	"context"
	"fmt"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/hnakamur/ltsvlog"
	"github.com/marpaia/graphite-golang"
	"github.com/masa23/logreport/internal/exporter"
)

type GraphiteExporter struct {
	metricsCh chan []*exporter.Metric
	stopCh    chan struct{}
	config    *GraphiteExporterConfig
	g         *graphite.Graphite
	isRunning atomic.Bool
}

var _ exporter.Exporter = (*GraphiteExporter)(nil)

type GraphiteExporterConfig struct {
	Prefix        string
	Host          string
	Port          int
	SendBuffer    int
	MaxRetryCount int
	RetryWait     time.Duration
}

func NewGraphiteExporter(config *GraphiteExporterConfig) (*GraphiteExporter, error) {
	g, err := graphite.NewGraphite(config.Host, config.Port)
	if err != nil {
		return nil, err
	}
	e := &GraphiteExporter{
		metricsCh: make(chan []*exporter.Metric, config.SendBuffer),
		stopCh:    make(chan struct{}),
		config:    config,
		g:         g,
		isRunning: atomic.Bool{},
	}
	e.isRunning.Store(false)
	return e, nil
}

func (e *GraphiteExporter) Export(ctx context.Context, metrics []*exporter.Metric) error {
	e.metricsCh <- metrics
	return nil
}

func (e *GraphiteExporter) Stop(ctx context.Context) error {
	// metricsChに残っているメトリクスを送信してから終了させる必要があるため
	// 実際の終了処理はStart()関数のstopCh受け取り部分で行います
	e.stopCh <- struct{}{}
	return nil
}

func (e *GraphiteExporter) IsRunning() bool {
	return e.isRunning.Load()
}

func (e *GraphiteExporter) Start(ctx context.Context) {
	ltsvlog.Logger.Debug().String("msg", "Starting GraphiteExporter goroutine").Log()
	e.isRunning.Store(true)
	for {
		select {
		case metrics := <-e.metricsCh:
			if err := e.send(metrics); err != nil {
				ltsvlog.Logger.Err(err)
			}
		case <-e.stopCh:
			ltsvlog.Logger.Info().String("msg", "graphite exporter receive stop signal")
			// これ以上channelに書き込まれないようにcloseする
			close(e.metricsCh)
			if len(e.metricsCh) > 0 {
				ltsvlog.Logger.Info().String("msg", "graphite exporter send remaining metrics")
				metrics := <-e.metricsCh
				if err := e.send(metrics); err != nil {
					ltsvlog.Logger.Err(err)
				}
			}

			_ = e.g.Disconnect()
			e.isRunning.Store(false)
			ltsvlog.Logger.Info().String("msg", "graphite exporter stopped")
			return
		}
	}
}

func (e *GraphiteExporter) send(metrics []*exporter.Metric) error {
	ltsvlog.Logger.Debug().Fmt("msg", "Sending %d metrics to Graphite", len(metrics)).Log()
	graphiteMetrics := e.convertGraphiteMetrics(metrics)
	retryCount := 0
	for ; retryCount < e.config.MaxRetryCount; retryCount++ {
		// 2回目以降は接続からやり直しする
		if retryCount >= 1 {
			if err := e.g.Connect(); err != nil {
				ltsvlog.Logger.Info().Fmt("msg", "failed to connect graphite err=%s", err.Error()).
					Int("retryCount", retryCount).Log()
				time.Sleep(e.config.RetryWait)
				continue
			}
		}
		if err := e.g.SendMetrics(graphiteMetrics); err == nil {
			return nil
		} else {
			ltsvlog.Logger.Info().Fmt("msg", "failed to graphite.SendMetrics err=%s", err.Error()).
				Int("retryCount", retryCount).Log()
		}
		time.Sleep(e.config.RetryWait)

	}
	return fmt.Errorf("failed to send graphite, retry %d", retryCount)
}

func (e *GraphiteExporter) convertGraphiteMetrics(metrics []*exporter.Metric) []graphite.Metric {
	gmetrics := make([]graphite.Metric, 0, len(metrics))
	for _, m := range metrics {
		var value string
		switch v := m.Value.(type) {
		case int64:
			value = strconv.FormatInt(v, 10)
		case float64:
			value = strconv.FormatFloat(v, 'f', 3, 64)
		default:
			//TODO: log
			continue
		}

		name := fmt.Sprintf("%s.%s", e.config.Prefix, m.ItemName)
		if m.ItemValue != "" {
			name = fmt.Sprintf("%s.%s", name, m.ItemValue)
		}
		gmetrics = append(gmetrics, graphite.Metric{
			Name:      name,
			Value:     value,
			Timestamp: m.Timestamp.Unix(),
		})
	}
	return gmetrics
}
