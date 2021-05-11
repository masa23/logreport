package logreport

import (
	"bytes"
	"strconv"

	"github.com/valyala/fastjson"
)

type parsedLog struct {
	t string
	f float64
	i int64
	b []byte
}

type parsedLogs struct {
	list map[string]parsedLog
}

// Bytes
func (p *parsedLogs) Bytes(name string) []byte {
	return p.list[name].b
}

func (p *parsedLogs) Int(name string) int64 {
	return p.list[name].i
}

func (p *parsedLogs) Float(name string) float64 {
	return p.list[name].f
}

func (p *parsedLogs) IsColumn(name string) bool {
	_, ok := p.list[name]
	return ok
}

// ParseLog
// format json, ltsv
func ParseLog(buf []byte, columns []logColumn, format string) (parsedLogs, error) {
	var err error
	var parsed parsedLogs
	parsed.list = make(map[string]parsedLog)
	if format == "json" {
		var p fastjson.Parser

		v, err := p.ParseBytes(buf)
		if err != nil {
			return parsed, err
		}

		for _, column := range columns {
			switch column.DataType {
			case DataTypeString:
				b := v.GetStringBytes(column.Name)
				parsed.list[column.Name] = parsedLog{
					t: column.DataType,
					b: b,
				}
			case DataTypeInt:
				i := v.GetInt64(column.Name)
				parsed.list[column.Name] = parsedLog{
					t: column.DataType,
					i: i,
				}
			case DataTypeFloat:
				f := v.GetFloat64(column.Name)
				parsed.list[column.Name] = parsedLog{
					t: column.DataType,
					f: f,
				}
			}
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
					switch column.DataType {
					case DataTypeString:
						parsed.list[column.Name] = parsedLog{
							t: column.DataType,
							b: s[1],
						}
					case DataTypeInt:
						var i int64
						i, err = strconv.ParseInt(string(s[1]), 10, 64)
						parsed.list[column.Name] = parsedLog{
							t: column.DataType,
							i: i,
						}
					case DataTypeFloat:
						var f float64
						f, err = strconv.ParseFloat(string(s[1]), 64)
						parsed.list[column.Name] = parsedLog{
							t: column.DataType,
							f: f,
						}
					}
					break
				}
			}
		}
	}

	return parsed, err
}
