package logreport

import (
	"bytes"
	"strconv"

	"github.com/valyala/fastjson"
)

type parsedLogs struct {
	list map[string]string
}

// String
func (p *parsedLogs) String(name string) string {
	return p.list[name]
}

func (p *parsedLogs) IsColumn(name string) bool {
	_, ok := p.list[name]
	return ok
}

func (p *parsedLogs) Int(name string) int64 {
	i, _ := strconv.ParseInt(p.list[name], 10, 64)
	return i
}

func (p *parsedLogs) Float(name string) float64 {
	f, _ := strconv.ParseFloat(p.list[name], 64)
	return f
}

// ParseLog
// format json, ltsv
func ParseLog(buf []byte, columns []logColumn, format string) (parsedLogs, error) {
	var err error
	var parsed parsedLogs
	parsed.list = make(map[string]string)
	if format == "json" {
		var p fastjson.Parser

		v, err := p.ParseBytes(buf)
		if err != nil {
			return parsed, err
		}
		for _, column := range columns {
			s := string(v.GetStringBytes(column.Name))
			parsed.list[column.Name] = s
		}
	} else if format == "ltsv" {
		list := bytes.Split(buf, []byte("\t"))
		for _, column := range columns {
			for _, b := range list {
				s := bytes.SplitN(b, []byte(":"), 2)
				if len(s) < 2 {
					continue
				}
				if bytes.Equal(s[0], []byte(column.Name)) {
					parsed.list[column.Name] = string(s[1])
				}
			}
		}
	}

	return parsed, err
}
