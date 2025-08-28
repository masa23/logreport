package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"github.com/hnakamur/errstack"
	"github.com/hnakamur/ltsvlog"

	"github.com/masa23/gotail"

	"github.com/masa23/logreport"
	"github.com/masa23/logreport/internal/exporter"
	"github.com/masa23/logreport/internal/exporter/graphite"
	"github.com/masa23/logreport/internal/exporter/otlpgrpc"
	_ "github.com/ophum/grpc-go-addrs-resolver"
)

var (
	conf             *logreport.Config
	confLock         = new(sync.Mutex)
	graphiteExporter *graphite.GraphiteExporter
	otlpGrpcExporter *otlpgrpc.OtlpGrpcExporter
)

type sumData struct {
	Int       map[string]int64
	Float     map[string]float64
	timestamp time.Time
	new       bool
}

type times []time.Time

// time sort
func (t times) Len() int {
	return len(t)
}
func (t times) Less(i, j int) bool {
	return t[i].Before(t[j])
}
func (t times) Swap(i, j int) {
	t[i], t[j] = t[j], t[i]
}

func main() {
	var configFile string
	var err error
	flag.StringVar(&configFile, "config", "./config.yaml", "config file path")
	flag.Parse()

	conf, err = logreport.ConfigLoad(configFile)
	if err != nil {
		panic(err)
	}

	// Error Log
	if conf.ErrorLogFile != "" {
		logFile, err := os.OpenFile(conf.ErrorLogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
		if err != nil {
			panic(err)
		}
		defer logFile.Close()
		ltsvlog.Logger = ltsvlog.NewLTSVLogger(logFile, conf.Debug)
	} else {
		ltsvlog.Logger = ltsvlog.NewLTSVLogger(os.Stdout, conf.Debug)
	}
	pid := os.Getpid()
	ltsvlog.Logger.Info().Fmt("msg", "start logreport pid=%d", pid).Log()

	if conf.Exporters.Graphite != nil || conf.Graphite != nil {
		var exporterConfig *graphite.GraphiteExporterConfig
		if conf.Exporters.Graphite != nil {
			exporterConfig = &graphite.GraphiteExporterConfig{
				Host:          conf.Exporters.Graphite.Host,
				Port:          conf.Exporters.Graphite.Port,
				Prefix:        conf.Exporters.Graphite.Prefix,
				SendBuffer:    conf.Exporters.Graphite.SendBuffer,
				MaxRetryCount: conf.Exporters.Graphite.MaxRetryCount,
				RetryWait:     time.Second,
			}
		}

		// 互換性を維持するため`Exporters.Graphite`が設定されていない場合は、古いフィールドの設定値を利用します。
		if conf.Exporters.Graphite == nil {
			ltsvlog.Logger.Info().String("msg", "Warning: the config field `Graphite` is deprecated. Please use `Exporters.Graphite` instead").Log()
			exporterConfig = &graphite.GraphiteExporterConfig{
				Host:          conf.Graphite.Host,
				Port:          conf.Graphite.Port,
				Prefix:        conf.Graphite.Prefix,
				SendBuffer:    conf.Graphite.SendBuffer,
				MaxRetryCount: conf.Graphite.MaxRetryCount,
				RetryWait:     time.Second,
			}
		}

		graphiteExporter, err = graphite.NewGraphiteExporter(exporterConfig)
		if err != nil {
			ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "graphite connection error", err)))
			os.Exit(1)
		}

		go graphiteExporter.Start(context.TODO())
	}
	if conf.Exporters.OtlpGrpc != nil {
		var caCertPool *x509.CertPool
		if conf.Exporters.OtlpGrpc.TLS.CACertificate != "" {
			caPem, err := os.ReadFile(conf.Exporters.OtlpGrpc.TLS.CACertificate)
			if err != nil {
				ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("failed to read otlpgrpc CA Certificate %s err=%+v", conf.Exporters.OtlpGrpc.TLS.CACertificate, err)))
				os.Exit(1)
			}
			caCertPool = x509.NewCertPool()
			if !caCertPool.AppendCertsFromPEM(caPem) {
				ltsvlog.Logger.Err(errors.New("failed to load ca certificate"))
				os.Exit(1)
			}

		}
		var clientCertificate *tls.Certificate
		if conf.Exporters.OtlpGrpc.TLS.ClientCertificate != "" && conf.Exporters.OtlpGrpc.TLS.ClientCertificateKey != "" {
			cert, err := tls.LoadX509KeyPair(conf.Exporters.OtlpGrpc.TLS.ClientCertificate, conf.Exporters.OtlpGrpc.TLS.ClientCertificateKey)
			if err != nil {
				ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("failed to LoadX509KeyPair cert=%s key=%s err=%+v",
					conf.Exporters.OtlpGrpc.TLS.ClientCertificate,
					conf.Exporters.OtlpGrpc.TLS.ClientCertificateKey,
					err,
				)))
				os.Exit(1)
			}
			clientCertificate = &cert
		}
		otlpGrpcExporter, err = otlpgrpc.NewOtlpGrpcExporter(context.TODO(), &otlpgrpc.OtlpGrpcExporterConfig{
			URL: conf.Exporters.OtlpGrpc.URL,
			TLS: &otlpgrpc.OtlpGrpcExporterConfigTLS{
				Insecure:          conf.Exporters.OtlpGrpc.TLS.Insecure,
				CACertPool:        caCertPool,
				ClientCertificate: clientCertificate,
			},
			SendBuffer:         conf.Exporters.OtlpGrpc.SendBuffer,
			MaxRetryCount:      conf.Exporters.OtlpGrpc.MaxRetryCount,
			RetryWait:          conf.Exporters.OtlpGrpc.RetryWait,
			ResourceAttributes: conf.Exporters.OtlpGrpc.ResourceAttributes,
		})
		if err != nil {
			ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "otlpgrpc connection error", err)))
			os.Exit(1)
		}
		go otlpGrpcExporter.Start(context.TODO())
	}
	go readLog()

	for {
		signalChan := make(chan os.Signal, 1)
		signal.Notify(signalChan, syscall.SIGHUP)

		switch <-signalChan {
		case syscall.SIGHUP:
			newConf, err := logreport.ConfigLoad(configFile)
			if err != nil {
				// エラー出してcontinue
				ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "reload error", err)))
				continue
			}

			// ログのリオープン
			newLogger := new(ltsvlog.LTSVLogger)
			if newConf.ErrorLogFile != "" {
				newLogFile, err := os.OpenFile(newConf.ErrorLogFile, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0600)
				if err != nil {
					ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "log file reopen faild", err)))
					continue
				}
				//defer newLogFile.Close()
				newLogger = ltsvlog.NewLTSVLogger(newLogFile, newConf.Debug)
			} else {
				newLogger = ltsvlog.NewLTSVLogger(os.Stdout, newConf.Debug)
			}
			// graphiteのリオープン
			newGraphite, err := graphite.NewGraphiteExporter(&graphite.GraphiteExporterConfig{
				Host:          conf.Graphite.Host,
				Port:          conf.Graphite.Port,
				Prefix:        conf.Graphite.Prefix,
				SendBuffer:    conf.Graphite.SendBuffer,
				MaxRetryCount: conf.Graphite.MaxRetryCount,
				RetryWait:     time.Second,
			})
			if err != nil {
				ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "graphite connection error", err)))
				continue
			}

			confLock.Lock()
			ltsvlog.Logger = newLogger
			oldGraphite := graphiteExporter
			graphiteExporter = newGraphite
			conf = newConf
			confLock.Unlock()
			if err := oldGraphite.Stop(context.TODO()); err != nil {
				ltsvlog.Logger.Err(err)
			}
			ltsvlog.Logger.Info().String("msg", "reload logreport").Log()
		}
	}
}

func containsString(arr []string, str string) bool {
	for _, s := range arr {
		if s == str {
			return true
		}
	}
	return false
}

func readLog() {
	ltsvlog.Logger.Debug().String("msg", "start readLog go routine").Log()
	gotail.DefaultBufSize = conf.LogBufferSize
	sum := make(map[time.Time]*sumData)
	tail, err := gotail.Open(conf.LogFile, conf.PosFile)
	if err != nil {
		ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s logFile=%s posFile=%s err=%+v", "tail logfile faild", conf.LogFile, conf.PosFile, err)))
		os.Exit(1)
	}
	tail.InitialReadPositionEnd = false

	lock := new(sync.Mutex)
	timer := time.NewTimer(0)
	defer timer.Stop()

	go func() {
		for {
			now := time.Now()
			target := now.Truncate(conf.Report.Interval)
			d := target.Add(conf.Report.Interval).Sub(now)
			timer.Reset(d)
			<-timer.C

			lock.Lock()

			// 最新のものからDelay以上古いものは削除
			tl := make(times, len(sum))
			for ts := range sum {
				tl = append(tl, ts)
			}
			sort.Sort(sort.Reverse(times(tl)))
			if len(tl) > 0 {
				for ts := range sum {
					if !ts.After(tl[0].Add(-conf.Report.Delay)) {
						delete(sum, ts)
						ltsvlog.Logger.Debug().Fmt("msg", "delete metric time=%s", ts.String()).Log()
					}
				}
			}
			var metrics []*exporter.Metric
			for ts, m := range sum {
				if !now.Add(-conf.Report.Interval).Before(m.timestamp) {
					ltsvlog.Logger.Debug().Fmt("msg", "metric continue time=%s", m.timestamp.String()).Log()
					continue
				}
				for key, value := range m.Int {
					metrics = append(metrics, &exporter.Metric{
						Key:       key,
						Value:     value,
						Timestamp: ts,
					})
				}
				for key, value := range m.Float {
					metrics = append(metrics, &exporter.Metric{
						Key:       key,
						Value:     value,
						Timestamp: ts,
					})
				}
			}
			if len(metrics) > 0 {
				if graphiteExporter != nil {
					if err := graphiteExporter.Export(context.TODO(), metrics); err != nil {
						// graphiteExporter.Exportの実装上エラーは戻らない
						panic("unreachable code reached")
					}
				}
				if otlpGrpcExporter != nil {
					if err := otlpGrpcExporter.Export(context.TODO(), metrics); err != nil {
						// otlpGrpcExporter.Exportの実装上エラーは戻らない
						panic("unreachable code reached")
					}
				}
			}
			lock.Unlock()
		}
	}()

	for tail.Scan() {
		buf := tail.Bytes()
		ltsvlog.Logger.Debug().Fmt("readlog", "%s", string(buf)).Log()
		log, err := logreport.ParseLog(buf, conf.LogColumns, conf.LogFormat)
		if err != nil {
			ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "log parse error", err)))
			continue
		}

		if !log.IsColumn(conf.TimeColumn) {
			continue
		}

		t, err := time.Parse(conf.TimeParse, log.String(conf.TimeColumn))
		t = t.Truncate(conf.Report.Interval)
		if err != nil {
			continue
		}
		t = t.Truncate(conf.Report.Interval)

		lock.Lock()
		if _, ok := sum[t]; !ok {
			sum[t] = &sumData{
				Int:       make(map[string]int64),
				Float:     make(map[string]float64),
				timestamp: time.Now(),
				new:       true,
			}
		}
		sum[t].timestamp = time.Now()
	NEXT_METRIC:
		for _, metric := range conf.Metrics {
			if !log.IsColumn(metric.LogColumn) {
				continue
			}
			if metric.Filter != nil {
				for _, filter := range metric.Filter {
					if !log.IsColumn(filter.LogColumn) {
						continue NEXT_METRIC
					}
					if filter.Bool {
						if !containsString(filter.Values, string(log.String(filter.LogColumn))) {
							continue NEXT_METRIC
						}
					} else {
						if containsString(filter.Values, string(log.String(filter.LogColumn))) {
							continue NEXT_METRIC
						}
					}
				}
			}

			switch metric.Type {
			case logreport.MetricTypeCount:
				sum[t].Int[metric.ItemName]++
			case logreport.MetricTypeSum:
				switch metric.DataType {
				case logreport.DataTypeInt:
					sum[t].Int[metric.ItemName] += log.Int(metric.LogColumn)
				case logreport.DataTypeFloat:
					sum[t].Float[metric.ItemName] += log.Float(metric.LogColumn)
				}
			case logreport.MetricTypeMax:
				switch metric.DataType {
				case logreport.DataTypeInt:
					num := log.Int(metric.LogColumn)
					if sum[t].Int[metric.ItemName] < num {
						sum[t].Int[metric.ItemName] = num
					}
				case logreport.DataTypeFloat:
					num := log.Float(metric.LogColumn)
					if sum[t].Float[metric.ItemName] < num {
						sum[t].Float[metric.ItemName] = num
					}
				}
			case logreport.MetricTypeMin:
				if metric.DataType == logreport.DataTypeInt {
					num := log.Int(metric.LogColumn)
					if sum[t].new {
						sum[t].Int[metric.ItemName] = num
					} else {
						if sum[t].Int[metric.ItemName] > num {
							sum[t].Int[metric.ItemName] = num
						}
					}
				} else if metric.DataType == logreport.DataTypeFloat {
					num := log.Float(metric.LogColumn)
					if sum[t].new {
						sum[t].Float[metric.ItemName] = num
					} else {
						if sum[t].Float[metric.ItemName] > num {
							sum[t].Float[metric.ItemName] = num
						}
					}
				}
			case logreport.MetricTypeItemCount:
				sum[t].Int[metric.ItemName+"."+log.String(metric.LogColumn)]++
			}

		}
		sum[t].new = false
		lock.Unlock()
	}

	if err = tail.Err(); err != nil {
		ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "tail log err", err)))
		os.Exit(1)
	}
}
