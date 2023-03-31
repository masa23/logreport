package logreport

import (
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
	DataTypeString      = "string"
)

// Config is confiure struct
type Config struct {
	Debug         bool            `yaml:"Debug"`
	PidFile       string          `yaml:"PidFile"`
	ErrorLogFile  string          `yaml:"ErrorLogFile"`
	LogFile       string          `yaml:"LogFile"`
	PosFile       string          `yaml:"PosFile"`
	LogBufferSize int             `yaml:"LogBufferSize"`
	LogFormat     string          `yaml:"LogFormat"`
	Graphite      configGraphite  `yaml:"Graphite"`
	Report        configReport    `yaml:"Report"`
	Metrics       []configMetrics `yaml:"Metrics"`
	TimeColumn    string          `yaml:"TimeColumn"`
	TimeParse     string          `yaml:"TimeParse"`
	LogColumns    []logColumn
}

// logColumn
type logColumn struct {
	Name     string
	DataType string
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
	DataType  string `yaml:"DataType"`
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
	if conf.LogFormat == "" {
		conf.LogFormat = "ltsv"
	}
	if !isValidLogFormat(conf.LogFormat) {
		return conf, fmt.Errorf("LogFormat type %s is unsupported", conf.LogFormat)
	}
	if conf.LogBufferSize == 0 {
		conf.LogBufferSize = 4096
	}
	for i, metric := range conf.Metrics {
		if !isValidMetricType(metric.Type) {
			return conf, fmt.Errorf("metric type %s is unsupported", metric.Type)
		}
		if metric.DataType == "" {
			conf.Metrics[i].DataType = DataTypeString
			metric.DataType = DataTypeString
		}
		if !isValidDataType(metric.DataType) {
			return conf, fmt.Errorf("data type %s is unsupported", metric.DataType)
		}
		if metric.Filter == nil {
			continue
		}
		for j, filter := range metric.Filter {
			if filter.DataType == "" {
				conf.Metrics[i].Filter[j].DataType = DataTypeString
				filter.DataType = DataTypeString
			}
			if !isValidDataType(filter.DataType) {
				return conf, fmt.Errorf("data type %s is unsupported", filter.DataType)
			}
		}
	}
	confLogColumns(conf)
	return conf, nil
}

func confLogColumns(conf *Config) {
	conf.LogColumns = append(conf.LogColumns, logColumn{
		Name:     conf.TimeColumn,
		DataType: "string",
	})

	for _, metric := range conf.Metrics {
		if !inArrayColmuns(conf.LogColumns, metric.LogColumn) {
			conf.LogColumns = append(conf.LogColumns, logColumn{
				Name:     metric.LogColumn,
				DataType: metric.DataType,
			})
		}
		if metric.Filter == nil {
			continue
		}
		for _, filter := range metric.Filter {
			if !inArrayColmuns(conf.LogColumns, filter.LogColumn) {
				conf.LogColumns = append(conf.LogColumns, logColumn{
					Name:     filter.LogColumn,
					DataType: filter.DataType,
				})
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
	if str == DataTypeInt || str == DataTypeFloat || str == DataTypeString {
		return true
	}
	return false
}

func inArrayColmuns(array []logColumn, name string) bool {
	for _, v := range array {
		if v.Name == name {
			return true
		}
	}
	return false
}

func isValidLogFormat(str string) bool {
	if str == "ltsv" || str == "json" {
		return true
	}
	return false
}
