package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"golang.org/x/time/rate"
)

type OutputFormat string

const (
	OutputFormatLTSV OutputFormat = "ltsv"
	OutputFormatJSON OutputFormat = "json"
)

func main() {
	path := flag.String("path", "access.log", "output file path")
	format := flag.String("format", "ltsv", "format ltsv or json")
	duration := flag.String("duration", "1m", "duration")
	logPerSec := flag.Int("log-per-sec", 100, "log count per second")
	append := flag.Bool("append", false, "append log")
	flag.Parse()

	var outputFormat OutputFormat
	switch *format {
	case string(OutputFormatLTSV):
		outputFormat = OutputFormatLTSV
	case string(OutputFormatJSON):
		outputFormat = OutputFormatJSON
	default:
		log.Fatal("invalid format", format)
	}

	d, err := time.ParseDuration(*duration)
	if err != nil {
		log.Fatal(err)
	}

	if err := run(*path, outputFormat, d, *logPerSec, *append); err != nil {
		log.Fatal(err)
	}
}

func run(path string, format OutputFormat, duration time.Duration, logPerSec int, append bool) error {
	var f *os.File
	var err error
	if append {
		f, err = os.OpenFile(path, os.O_WRONLY|os.O_APPEND, os.ModePerm)
	} else {
		f, err = os.Create(path)
	}
	if err != nil {
		return err
	}
	defer f.Close()

	limiter := rate.NewLimiter(rate.Limit(logPerSec), 1)
	ctx, cancel := context.WithTimeout(context.Background(), duration)
	defer cancel()

	for {
		if err := limiter.Wait(ctx); err != nil {
			if _, ok := ctx.Deadline(); ok {
				return nil
			}
			return err
		}
		l := newLog()
		switch format {
		case OutputFormatLTSV:
			if _, err := f.Write(l.LTSV()); err != nil {
				return err
			}
		case OutputFormatJSON:
			if _, err := f.Write(l.JSON()); err != nil {
				return err
			}
		}
	}
}

type Log struct {
	TimeLocal           string  `json:"time_local"`
	Status              int     `json:"status"`
	Scheme              string  `json:"scheme"`
	UpstreamCacheStatus string  `json:"upstream_cache_status"`
	BytesSent           int     `json:"bytes_sent"`
	UpstreamRequestTime float64 `json:"upstream_request_time"`
}

var statusCodes = []int{
	http.StatusContinue,
	http.StatusSwitchingProtocols,
	http.StatusProcessing,
	http.StatusEarlyHints,
	http.StatusOK,
	http.StatusCreated,
	http.StatusAccepted,
	http.StatusNonAuthoritativeInfo,
	http.StatusNoContent,
	http.StatusResetContent,
	http.StatusPartialContent,
	http.StatusMultiStatus,
	http.StatusAlreadyReported,
	http.StatusIMUsed,
	http.StatusMultipleChoices,
	http.StatusMovedPermanently,
	http.StatusFound,
	http.StatusSeeOther,
	http.StatusNotModified,
	http.StatusUseProxy,
	http.StatusTemporaryRedirect,
	http.StatusPermanentRedirect,
	http.StatusBadRequest,
	http.StatusUnauthorized,
	http.StatusPaymentRequired,
	http.StatusForbidden,
	http.StatusNotFound,
	http.StatusMethodNotAllowed,
	http.StatusNotAcceptable,
	http.StatusProxyAuthRequired,
	http.StatusRequestTimeout,
	http.StatusConflict,
	http.StatusGone,
	http.StatusLengthRequired,
	http.StatusPreconditionFailed,
	http.StatusRequestEntityTooLarge,
	http.StatusRequestURITooLong,
	http.StatusUnsupportedMediaType,
	http.StatusRequestedRangeNotSatisfiable,
	http.StatusExpectationFailed,
	http.StatusTeapot,
	http.StatusMisdirectedRequest,
	http.StatusUnprocessableEntity,
	http.StatusLocked,
	http.StatusFailedDependency,
	http.StatusTooEarly,
	http.StatusUpgradeRequired,
	http.StatusPreconditionRequired,
	http.StatusTooManyRequests,
	http.StatusRequestHeaderFieldsTooLarge,
	http.StatusUnavailableForLegalReasons,
	http.StatusInternalServerError,
	http.StatusNotImplemented,
	http.StatusBadGateway,
	http.StatusServiceUnavailable,
	http.StatusGatewayTimeout,
	http.StatusHTTPVersionNotSupported,
	http.StatusVariantAlsoNegotiates,
	http.StatusInsufficientStorage,
	http.StatusLoopDetected,
	http.StatusNotExtended,
	http.StatusNetworkAuthenticationRequired,
}

func newLog() *Log {
	status := statusCodes[rand.Intn(len(statusCodes))]
	scheme := []string{"http", "https"}[rand.Intn(2)]
	cacheStatus := []string{"HIT", "MISS"}[rand.Intn(2)]
	bytesSent := rand.Intn(10000)
	upstreamRequestTime := rand.Float64()

	return &Log{
		TimeLocal:           time.Now().Format("02/Jan/2006:15:04:05 -0700"),
		Status:              status,
		Scheme:              scheme,
		UpstreamCacheStatus: cacheStatus,
		BytesSent:           bytesSent,
		UpstreamRequestTime: upstreamRequestTime,
	}
}
func (l *Log) JSON() []byte {
	b, err := json.Marshal(l)
	if err != nil {
		panic(err)
	}
	return append(b, '\n')
}

func (l *Log) LTSV() []byte {
	return []byte(fmt.Sprintf("time_local:%s\tstatus:%d\tscheme:%s\tupstream_cache_status:%s\tbytes_sent:%d\tupstream_request_time:%f\n",
		l.TimeLocal,
		l.Status,
		l.Scheme,
		l.UpstreamCacheStatus,
		l.BytesSent,
		l.UpstreamRequestTime,
	))
}
