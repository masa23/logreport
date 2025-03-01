package logreport

import (
	"fmt"
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
	Graphite      *configGraphite `yaml:"Graphite"`
	Report        configReport    `yaml:"Report"`
	Metrics       []configMetrics `yaml:"Metrics"`
	TimeColumn    string          `yaml:"TimeColumn"`
	TimeParse     string          `yaml:"TimeParse"`
	LogColumns    []logColumn
	Exporters     configExporters `yaml:"Exporters"`
}

type configExporters struct {
	Graphite *configGraphite `yaml:"Graphite"`
	OtlpGrpc *configOtlpGrpc `yaml:"OtlpGrpc"`
}

type configOtlpGrpcTLS struct {
	Insecure             bool   `yaml:"Insecure"`
	CACertificate        string `yaml:"CACertificate"`
	ClientCertificate    string `yaml:"ClientCertificate"`
	ClientCertificateKey string `yaml:"ClientCertificateKey"`
}

type configOtlpGrpc struct {
	URL           string             `yaml:"URL"`
	TLS           *configOtlpGrpcTLS `yaml:"TLS"`
	SendBuffer    int                `yaml:"SendBuffer"`
	MaxRetryCount int                `yaml:"MaxRetryCount"`
	RetryWait     time.Duration      `yaml:"RetryWait"`
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
	LogColumn string   `yaml:"LogColumn"`
	Value     string   `yaml:"Value"`
	Values    []string `yaml:"Values"`
	DataType  string   `yaml:"DataType"`
	Bool      bool     `yaml:"Bool"`
}

// ConfigLoad is loading yaml config
func ConfigLoad(file string) (*Config, error) {
	conf := new(Config)
	buf, err := os.ReadFile(file)
	if err != nil {
		return conf, err
	}
	err = yaml.Unmarshal(buf, &conf)
	if err != nil {
		return conf, err
	}

	defaultConfig(conf)

	if !isValidLogFormat(conf.LogFormat) {
		return conf, fmt.Errorf("LogFormat type %s is unsupported", conf.LogFormat)
	}

	for i, metric := range conf.Metrics {
		if err := validateMetricAndSetDefault(&conf.Metrics[i]); err != nil {
			return conf, err
		}
		if metric.Filter == nil {
			continue
		}
		for j := range metric.Filter {
			if err := validateFilterAndSetDefault(&conf.Metrics[i].Filter[j]); err != nil {
				return conf, err
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

func defaultConfig(conf *Config) {
	if conf.LogFormat == "" {
		conf.LogFormat = "ltsv"
	}
	if conf.LogBufferSize == 0 {
		conf.LogBufferSize = 4096
	}
}

func validateMetricAndSetDefault(metric *configMetrics) error {
	if !isValidMetricType(metric.Type) {
		return fmt.Errorf("metric type %s is unsupported", metric.Type)
	}
	if metric.DataType == "" {
		metric.DataType = DataTypeString
	}
	if !isValidDataType(metric.DataType) {
		return fmt.Errorf("data type %s is unsupported", metric.DataType)
	}
	return nil
}

func validateFilterAndSetDefault(filter *configMetricsFilter) error {
	if filter.Value != "" && filter.Values != nil {
		return fmt.Errorf("filter value and values are both set")
	} else if filter.Value != "" {
		filter.Values = []string{filter.Value}
	}
	if filter.DataType == "" {
		filter.DataType = DataTypeString
	}
	if !isValidDataType(filter.DataType) {
		return fmt.Errorf("data type %s is unsupported", filter.DataType)
	}
	return nil
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
