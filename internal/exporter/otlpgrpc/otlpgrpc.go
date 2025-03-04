package otlpgrpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/hnakamur/ltsvlog"
	"github.com/masa23/logreport/internal/exporter"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/sdk/resource"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

type OtlpGrpcExporter struct {
	metricsCh    chan []*exporter.Metric
	stopCh       chan struct{}
	config       *OtlpGrpcExporterConfig
	otlpExporter *otlpmetricgrpc.Exporter
	res          *resource.Resource
	isRunning    atomic.Bool
}

var _ exporter.Exporter = (*OtlpGrpcExporter)(nil)

type OtlpGrpcExporterConfig struct {
	URL                string
	TLS                *OtlpGrpcExporterConfigTLS
	SendBuffer         int
	MaxRetryCount      int
	RetryWait          time.Duration
	ResourceAttributes map[string]string
}

type OtlpGrpcExporterConfigTLS struct {
	Insecure          bool
	CACertPool        *x509.CertPool
	ClientCertificate *tls.Certificate
}

func NewOtlpGrpcExporter(ctx context.Context, config *OtlpGrpcExporterConfig) (*OtlpGrpcExporter, error) {
	var creds credentials.TransportCredentials
	if config.TLS.Insecure {
		creds = insecure.NewCredentials()
	} else {
		certificates := []tls.Certificate{}
		if config.TLS.ClientCertificate != nil {
			certificates = append(certificates, *config.TLS.ClientCertificate)
		}
		creds = credentials.NewTLS(&tls.Config{
			Certificates: certificates,
			RootCAs:      config.TLS.CACertPool,
		})
	}

	conn, err := grpc.NewClient(config.URL,
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, err
	}
	otlpExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		return nil, err
	}

	attributes := []attribute.KeyValue{}
	for k, v := range config.ResourceAttributes {
		attributes = append(attributes, attribute.String(k, v))
	}
	res, err := resource.New(ctx, resource.WithAttributes(attributes...))
	if err != nil {
		return nil, err
	}

	e := &OtlpGrpcExporter{
		metricsCh:    make(chan []*exporter.Metric, config.SendBuffer),
		stopCh:       make(chan struct{}),
		config:       config,
		otlpExporter: otlpExporter,
		res:          res,
		isRunning:    atomic.Bool{},
	}
	e.isRunning.Store(false)
	return e, nil

}

func (e *OtlpGrpcExporter) Export(ctx context.Context, metrics []*exporter.Metric) error {
	e.metricsCh <- metrics
	return nil
}

func (e *OtlpGrpcExporter) Stop(ctx context.Context) error {
	e.stopCh <- struct{}{}
	return nil
}

func (e *OtlpGrpcExporter) IsRunning() bool {
	return e.isRunning.Load()
}

func (e *OtlpGrpcExporter) Start(ctx context.Context) {
	ltsvlog.Logger.Debug().String("msg", "Starting OtlpGrpcExporter goroutine").Log()
	e.isRunning.Store(true)
	for {
		select {
		case metrics := <-e.metricsCh:
			if err := e.send(ctx, metrics); err != nil {
				ltsvlog.Logger.Err(err)
			}
		case <-e.stopCh:
			// ctxはCancelされているため新しくcontext.Contextを作成する
			stopCtx := context.Background()

			ltsvlog.Logger.Info().String("msg", "otlpgrpc exporter receive stop signal")
			// これ以上channelに書き込まれないようにcloseする
			close(e.metricsCh)
			if len(e.metricsCh) > 0 {
				ltsvlog.Logger.Info().String("msg", "otlpgrpc exporter send remaining metrics")
				metrics := <-e.metricsCh
				if err := e.send(stopCtx, metrics); err != nil {
					ltsvlog.Logger.Err(err)
				}
			}

			if err := e.otlpExporter.ForceFlush(stopCtx); err != nil {
				ltsvlog.Logger.Err(err)
			}
			if err := e.otlpExporter.Shutdown(stopCtx); err != nil {
				ltsvlog.Logger.Err(err)
			}
			e.isRunning.Store(false)
			return
		}
	}
}

func (e *OtlpGrpcExporter) send(ctx context.Context, metrics []*exporter.Metric) error {
	ltsvlog.Logger.Debug().Fmt("msg", "Sending %d metrics to otlpgrpc", len(metrics)).Log()
	otlpMetrics := e.convertOtlpMetrics(metrics)
	retryCount := 0
	for ; retryCount < e.config.MaxRetryCount; retryCount++ {
		if err := e.otlpExporter.Export(ctx, &metricdata.ResourceMetrics{
			Resource: e.res,
			ScopeMetrics: []metricdata.ScopeMetrics{{
				Metrics: otlpMetrics,
			}},
		}); err == nil {
			return nil
		} else {
			ltsvlog.Logger.Info().Fmt("msg", "failed to otlpExporter.Export err=%s", err.Error()).
				Int("retryCount", retryCount).Log()
		}
		time.Sleep(e.config.RetryWait)
	}
	return fmt.Errorf("failed to send otlpgrpc, retry %d", retryCount)
}

func (e *OtlpGrpcExporter) convertOtlpMetrics(metrics []*exporter.Metric) []metricdata.Metrics {
	ometrics := make([]metricdata.Metrics, 0, len(metrics))
	for _, m := range metrics {
		var data metricdata.Aggregation
		switch v := m.Value.(type) {
		case int64:
			data = metricdata.Gauge[int64]{
				DataPoints: []metricdata.DataPoint[int64]{
					{
						Time:  m.Timestamp,
						Value: v,
					},
				},
			}
		case float64:
			data = metricdata.Gauge[float64]{
				DataPoints: []metricdata.DataPoint[float64]{
					{
						Time:  m.Timestamp,
						Value: v,
					},
				},
			}
		default:
			continue
		}

		ometrics = append(ometrics, metricdata.Metrics{
			Name: m.Key,
			Data: data,
		})
	}
	return ometrics
}
