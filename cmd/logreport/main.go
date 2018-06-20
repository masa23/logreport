package main

import (
	"flag"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/marpaia/graphite-golang"

	"github.com/masa23/gotail"

	"github.com/masa23/logreport"
)

var conf logreport.Config

type sumData struct {
	sum       map[string]int64
	timestamp time.Time
}

type times []time.Time

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
	sendMetrics := make(chan []graphite.Metric, conf.Graphite.SendBuffer)
	g, err := graphite.NewGraphite(conf.Graphite.Host, conf.Graphite.Port)
	if err != nil {
		panic(err)
	}
	go func() {
		for {
			metrics := <-sendMetrics
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
	}()
	readLog(sendMetrics)
}

func readLog(sendMetrics chan []graphite.Metric) {
	sum := make(map[time.Time]*sumData)
	tail, err := gotail.Open(conf.LogFile, conf.PosFile)
	if err != nil {
		panic(err)
	}
	tail.InitialReadPositionEnd = true

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
					}
				}
			}

			var metrics []graphite.Metric
			for ts, m := range sum {
				if !now.Add(-conf.Report.Interval).Before(m.timestamp) {
					continue
				}
				for key, value := range m.sum {
					metrics = append(metrics, graphite.Metric{
						Name:      fmt.Sprintf("%s.%s", conf.Graphite.Prefix, key),
						Value:     strconv.FormatInt(value, 10),
						Timestamp: ts.Unix(),
					})
				}
			}
			sendMetrics <- metrics
			t = t.Truncate(conf.Report.Interval).Add(-conf.Report.Interval)
			lock.Unlock()
		}
	}()

	tail.Scan()
	for {
		buf := tail.TailBytes()
		log := logreport.ParseLTSV(buf, conf.LogColumns)

		if log[conf.TimeColumn] == nil {
			continue
		}
		t, err := time.Parse(conf.TimeParse, string(log[conf.TimeColumn]))
		if err != nil {
			continue
		}

		lock.Lock()
		if _, ok := sum[t]; !ok {
			sum[t] = &sumData{
				sum:       make(map[string]int64),
				timestamp: time.Now(),
			}
		}
		sum[t].timestamp = time.Now()
	NEXT_METRIC:
		for _, metric := range conf.Metrics {
			if log[metric.LogColumn] == nil {
				continue
			}
			if metric.Filter != nil {
				for _, filter := range metric.Filter {
					if log[filter.LogColumn] == nil {
						continue NEXT_METRIC
					}
					if filter.Bool {
						if string(log[filter.LogColumn]) != filter.Value {
							continue NEXT_METRIC
						}
					} else {
						if string(log[filter.LogColumn]) == filter.Value {
							continue NEXT_METRIC
						}
					}
				}
			}

			switch metric.Type {
			case logreport.MetricTypeCount:
				sum[t].sum[metric.ItemName]++
			case logreport.MetricTypeSum:
				num, _ := strconv.ParseInt(string(log[metric.LogColumn]), 10, 64)
				sum[t].sum[metric.ItemName] += num
			case logreport.MetricTypeItemCount:
				sum[t].sum[metric.ItemName+"."+string(log[metric.LogColumn])]++
			}
		}
		lock.Unlock()
	}
}
