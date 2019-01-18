package logreport

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"gopkg.in/yaml.v2"
)

// Metric type
const (
	MetricTypeCount     = "count"
	MetricTypeSum       = "sum"
	MetricTypeMax       = "max"
	MetricTypeMin       = "min"
	MetricTypeItemCount = "itemCount"
	DataTypeInt         = "int"
	DataTypeFloat       = "float"
)

// Config is confiure struct
type Config struct {
	Debug        bool            `yaml:"Debug"`
	PidFile      string          `yaml:"PidFile"`
	ErrorLogFile string          `yaml:"ErrorLogFile"`
	LogFile      string          `yaml:"LogFile"`
	PosFile      string          `yaml:"PosFile"`
	Graphite     configGraphite  `yaml:"Graphite"`
	Report       configReport    `yaml:"Report"`
	Metrics      []configMetrics `yaml:"Metrics"`
	TimeColumn   string          `yaml:"TimeColumn"`
	TimeParse    string          `yaml:"TimeParse"`
	LogColumns   [][]byte
}

type configGraphite struct {
	Host       string `yaml:"Host"`
	Port       int    `yaml:"Port"`
	Prefix     string `yaml:"Prefix"`
	SendBuffer int    `yaml:"SendBuffer"`
}

type configReport struct {
	Interval time.Duration `yaml:"Interval"`
	Delay    time.Duration `yaml:"Delay"`
}

type configMetrics struct {
	Description string                `yaml:"Description"`
	ItemName    string                `yaml:"ItemName"`
	Type        string                `yaml:"Type"`
	DataType    string                `yaml:"DataType"`
	LogColumn   string                `yaml:"LogColumn"`
	Filter      []configMetricsFilter `yaml:"Filter"`
}

type configMetricsFilter struct {
	LogColumn string `yaml:"LogColumn"`
	Value     string `yaml:"Value"`
	Bool      bool   `yaml:"Bool"`
}

// ConfigLoad is loading yaml config
func ConfigLoad(file string) (*Config, error) {
	conf := new(Config)
	fd, err := os.Open(file)
	if err != nil {
		return conf, err
	}
	defer fd.Close()

	buf, err := ioutil.ReadAll(fd)
	if err != nil {
		return conf, err
	}
	err = yaml.Unmarshal(buf, &conf)
	if err != nil {
		return conf, err
	}
	for _, metric := range conf.Metrics {
		if !isValidMetricType(metric.Type) {
			return conf, fmt.Errorf("metric type %s is unsupported", metric.Type)
		}
		if metric.DataType == "" {
			metric.DataType = DataTypeInt
		}
		if !isValidDataType(metric.DataType) {
			return conf, fmt.Errorf("data type %s is unsupported", metric.DataType)
		}
	}
	confLogColumns(conf)
	return conf, nil
}

func confLogColumns(conf *Config) {
	conf.LogColumns = append(conf.LogColumns, []byte(conf.TimeColumn))
	for _, m := range conf.Metrics {
		if !inArrayBytes(conf.LogColumns, []byte(m.LogColumn)) {
			conf.LogColumns = append(conf.LogColumns, []byte(m.LogColumn))
		}
		if m.Filter == nil {
			continue
		}
		for _, f := range m.Filter {
			if !inArrayBytes(conf.LogColumns, []byte(f.LogColumn)) {
				conf.LogColumns = append(conf.LogColumns, []byte(f.LogColumn))
			}
		}
	}
}

func isValidMetricType(str string) bool {
	if str == MetricTypeCount || str == MetricTypeSum || str == MetricTypeMax ||
		str == MetricTypeMin || str == MetricTypeItemCount {
		return true
	}
	return false
}

func isValidDataType(str string) bool {
	if str == DataTypeInt || str == DataTypeFloat {
		return true
	}
	return false
}

func inArrayBytes(list [][]byte, b []byte) bool {
	for _, v := range list {
		if bytes.Equal(v, b) {
			return true
		}
	}
	return false
}
