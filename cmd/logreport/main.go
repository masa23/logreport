package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/hnakamur/errstack"
	"github.com/hnakamur/ltsvlog"

	"github.com/marpaia/graphite-golang"

	"github.com/masa23/gotail"

	"github.com/masa23/logreport"
)

var (
	conf     *logreport.Config
	confLock = new(sync.Mutex)
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

	sendMetrics := make(chan []graphite.Metric, conf.Graphite.SendBuffer)

	g, err := graphite.NewGraphite(conf.Graphite.Host, conf.Graphite.Port)
	if err != nil {
		ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "graphite connection error", err)))
		os.Exit(1)
	}

	go sendGraphite(sendMetrics, g)
	go readLog(sendMetrics)

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
				defer newLogFile.Close()
				newLogger = ltsvlog.NewLTSVLogger(newLogFile, newConf.Debug)
			} else {
				newLogger = ltsvlog.NewLTSVLogger(os.Stdout, newConf.Debug)
			}
			// graphiteのリオープン
			newGraphite, err := graphite.NewGraphite(conf.Graphite.Host, conf.Graphite.Port)
			if err != nil {
				ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "graphite connection error", err)))
				continue
			}

			confLock.Lock()
			ltsvlog.Logger = newLogger
			g = newGraphite
			conf = newConf
			confLock.Unlock()
			ltsvlog.Logger.Info().String("msg", "reload logreport").Log()
		}
	}
}

func sendGraphite(sendMetrics chan []graphite.Metric, g *graphite.Graphite) {
	ltsvlog.Logger.Debug().String("msg", "start sendGraphite go routine").Log()
	for {
		metrics := <-sendMetrics
		ltsvlog.Logger.Debug().Fmt("msg", "sendmetric len=%d", len(metrics)).Log()
		err := g.SendMetrics(metrics)
		if err != nil {
			g.Disconnect()
			for {
				time.Sleep(time.Second)
				g, err = graphite.NewGraphite(conf.Graphite.Host, conf.Graphite.Port)
				if err == nil {
					break
				}
			}
		}
	}
}

func readLog(sendMetrics chan []graphite.Metric) {
	ltsvlog.Logger.Debug().String("msg", "start readLog go routine").Log()
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
			t := <-timer.C

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
			var metrics []graphite.Metric
			for ts, m := range sum {
				if !now.Add(-conf.Report.Interval).Before(m.timestamp) {
					ltsvlog.Logger.Debug().Fmt("msg", "metric continue time=%s", m.timestamp.String()).Log()
					continue
				}
				for key, value := range m.Int {
					metrics = append(metrics, graphite.Metric{
						Name:      fmt.Sprintf("%s.%s", conf.Graphite.Prefix, key),
						Value:     strconv.FormatInt(value, 10),
						Timestamp: ts.Unix(),
					})
				}
				for key, value := range m.Float {
					metrics = append(metrics, graphite.Metric{
						Name:      fmt.Sprintf("%s.%s", conf.Graphite.Prefix, key),
						Value:     strconv.FormatFloat(value, 'f', 3, 64),
						Timestamp: ts.Unix(),
					})
				}
			}
			if len(metrics) > 0 {
				sendMetrics <- metrics
			}
			t = t.Truncate(conf.Report.Interval).Add(-conf.Report.Interval)
			lock.Unlock()
		}
	}()

	for tail.Scan() {
		buf := tail.Bytes()
		log, err := logreport.ParseLog(buf, conf.LogColumns, conf.LogFormat)
		if err != nil {
			ltsvlog.Logger.Err(errstack.WithLV(errstack.Errorf("%s err=%+v", "log parse error", err)))
			continue
		}

		if log.Bytes(conf.TimeColumn) == nil {
			continue
		}
		t, err := time.Parse(conf.TimeParse, string(log.Bytes(conf.TimeColumn)))
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
						if string(log.Bytes(filter.LogColumn)) != filter.Value {
							continue NEXT_METRIC
						}
					} else {
						if string(log.Bytes(filter.LogColumn)) == filter.Value {
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
				case logreport.DataTypeFloat:
					sum[t].Int[metric.ItemName] += log.Int(metric.LogColumn)
				case logreport.DataTypeInt:
					sum[t].Float[metric.ItemName] += log.Float(metric.LogColumn)
				}
			case logreport.MetricTypeMax:
				switch metric.DataType {
				case logreport.DataTypeFloat:
					num := log.Int(metric.LogColumn)
					if sum[t].Int[metric.ItemName] < num {
						sum[t].Int[metric.ItemName] = num
					}
				case logreport.DataTypeInt:
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
				sum[t].Int[metric.ItemName+"."+string(log.Bytes(metric.LogColumn))]++
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
