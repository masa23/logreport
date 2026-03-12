package api

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"slices"
	"strings"
	"sync"

	"github.com/hnakamur/ltsvlog"
	"github.com/masa23/logreport/internal/exporter"
)

type API struct {
	lastMetrics map[string]*lastMetric
	mu          sync.RWMutex
	version     int
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
	}
}

func (a *API) SetLastMetrics(metrics []*exporter.Metric) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.version++
	for _, m := range metrics {
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
		Handler: mu,
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

	keys := []string{}
	for k := range a.lastMetrics {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&keys); err != nil {
		ltsvlog.Logger.Err(err)
	}
}

func (a *API) getLastMetrics(w http.ResponseWriter, r *http.Request) {
	defer a.recover()

	keysStr := r.URL.Query().Get("keys")
	if keysStr == "" {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("{}")); err != nil {
			ltsvlog.Logger.Err(err)
		}
		return
	}

	keys := strings.Split(keysStr, ",")

	res := map[string]any{}
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, k := range keys {
		var v any = 0
		m, ok := a.lastMetrics[k]
		if ok && m.Metric.Value != nil {
			v = m.Metric.Value
		}
		res[k] = v
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(&res); err != nil {
		ltsvlog.Logger.Err(err)
	}
}
