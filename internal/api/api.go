package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hnakamur/ltsvlog"
	"github.com/masa23/logreport/internal/exporter"
	"golang.org/x/sync/singleflight"
)

type API struct {
	lastMetrics   map[string]*lastMetric
	lastTimestamp time.Time
	mu            sync.RWMutex
	version       int
	sf            singleflight.Group
}

type lastMetric struct {
	Version int
	Metric  *exporter.Metric
}

func NewAPI() *API {
	return &API{
		lastMetrics: map[string]*lastMetric{},
		mu:          sync.RWMutex{},
		version:     0,
		sf:          singleflight.Group{},
	}
}

func (a *API) SetLastMetrics(metrics []*exporter.Metric) {
	_, _, _ = a.sf.Do("setLastMetrics", func() (any, error) {
		a.setLastMetrics(metrics)
		return nil, nil
	})
}

func (a *API) setLastMetrics(metrics []*exporter.Metric) {
	lastTimestamp := time.Time{}
	for _, m := range metrics {
		if m.Timestamp.Compare(lastTimestamp) > 0 {
			lastTimestamp = m.Timestamp
		}
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.lastTimestamp = lastTimestamp
	a.version++
	for _, m := range metrics {
		// NOTE: metricsには複数のtimestampのメトリクスが入る可能性があるため、
		// その中で最新の時刻のメトリクスを採用する
		if !m.Timestamp.Equal(lastTimestamp) {
			continue
		}
		key := m.ItemName
		if m.ItemValue != "" {
			key += "." + m.ItemValue
		}
		lm, ok := a.lastMetrics[key]
		if !ok {
			lm = &lastMetric{}
		}

		lm.Version = a.version
		lm.Metric = m
		a.lastMetrics[key] = lm
	}
	for k, v := range a.lastMetrics {
		if v.Version != a.version {
			delete(a.lastMetrics, k)
		}
	}
}

func (a *API) Serve(l net.Listener) error {
	mu := http.NewServeMux()
	mu.HandleFunc("/keys", a.getKeys)
	mu.HandleFunc("/metrics", a.getLastMetrics)
	sv := http.Server{
		Handler:           mu,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return sv.Serve(l)
}

func (a *API) recover() {
	if v := recover(); v != nil {
		log.Printf("PANIC: %v", v)
		return
	}
}

func (a *API) getKeys(w http.ResponseWriter, r *http.Request) {
	defer a.recover()

	keys := make([]string, 0, len(a.lastMetrics))
	a.mu.RLock()
	for k := range a.lastMetrics {
		keys = append(keys, k)
	}
	a.mu.RUnlock()

	slices.Sort(keys)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&keys); err != nil {
		ltsvlog.Logger.Err(err)
	}
}

func (a *API) getLastMetrics(w http.ResponseWriter, r *http.Request) {
	defer a.recover()

	keysStr := r.URL.Query().Get("keys")
	var keys []string
	if keysStr == "" {
		keys = []string{}
	} else {
		keys = strings.Split(keysStr, ",")
	}

	res := map[string]any{}
	a.mu.RLock()
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}

		var v any = 0
		m, ok := a.lastMetrics[k]
		if ok && m.Metric.Value != nil {
			v = m.Metric.Value
		}
		res[k] = v
	}
	if !a.lastTimestamp.IsZero() {
		res["time"] = a.lastTimestamp
	}
	a.mu.RUnlock()

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		ltsvlog.Logger.Err(err)
	}
}
