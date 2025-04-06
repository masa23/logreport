package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/hnakamur/errstack"
	"github.com/hnakamur/ltsvlog"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func initOtelMetrics(ctx context.Context) (shutdown func(ctx context.Context) error, err error) {
	var creds credentials.TransportCredentials
	if conf.OpenTelemetry.TLS.Insecure {
		creds = insecure.NewCredentials()
	} else {
		var caCertPool *x509.CertPool
		if conf.OpenTelemetry.TLS.CACertificate != "" {
			caPem, err := os.ReadFile(conf.Exporters.OtlpGrpc.TLS.CACertificate)
			if err != nil {
				return nil, errstack.WithLV(errstack.Errorf("failed to read otlpgrpc CA Certificate %s err=%+v", conf.Exporters.OtlpGrpc.TLS.CACertificate, err))
			}
			caCertPool = x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caPem) {
				return nil, errors.New("failed to load ca certificate")
			}

		}
		var clientCertificate *tls.Certificate
		if conf.OpenTelemetry.TLS.ClientCertificate != "" && conf.OpenTelemetry.TLS.ClientCertificateKey != "" {
			cert, err := tls.LoadX509KeyPair(conf.OpenTelemetry.TLS.ClientCertificate, conf.OpenTelemetry.TLS.ClientCertificateKey)
			if err != nil {
				return nil, errstack.WithLV(errstack.Errorf("failed to LoadX509KeyPair cert=%s key=%s err=%+v",
					conf.OpenTelemetry.TLS.ClientCertificate,
					conf.OpenTelemetry.TLS.ClientCertificateKey,
					err,
				))
			}
			clientCertificate = &cert
		}
		certificates := []tls.Certificate{}
		if clientCertificate != nil {
			certificates = append(certificates, *clientCertificate)
		}
		creds = credentials.NewTLS(&tls.Config{
			Certificates: certificates,
			RootCAs:      caCertPool,
		})
	}

	instanceID, err := uuid.NewRandom()
	if err != nil {
		return nil, err
	}
	res, err := resource.New(ctx,
		resource.WithOS(),
		resource.WithProcess(),
		resource.WithContainer(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName("logreport-internal-exporter"),
			semconv.ServiceInstanceID(instanceID.String()),
		),
	)
	if err != nil {
		return nil, err
	}

	conn, err := grpc.NewClient(conf.OpenTelemetry.URL,
		grpc.WithDefaultServiceConfig(`{"loadBalancingConfig": [{"round_robin":{}}]}`),
		grpc.WithTransportCredentials(creds),
	)
	if err != nil {
		return nil, err
	}

	exporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(conn))
	if err != nil {
		conn.Close()
		return nil, err
	}
	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(exporter)),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		if err := exporter.Shutdown(ctx); err != nil {
			ltsvlog.Logger.Err(err)
		}
		return nil, err
	}
	return exporter.Shutdown, nil
}
