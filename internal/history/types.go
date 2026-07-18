package history

import (
	"errors"
	"time"
)

var ErrClosed = errors.New("history service is closed")

type Sample struct {
	ServerKey string
	At        time.Time
	Online    bool
	CPU       *float64
	Memory    *float64
	Disk      *float64
	NetRX     *float64
	NetTX     *float64
	Load1     *float64
	Issues    []string
}

type Point struct {
	At     time.Time
	Online bool
	CPU    *float64
	Memory *float64
	Disk   *float64
	NetRX  *float64
	NetTX  *float64
	Load1  *float64
	Issues []string
}

type Range struct {
	Duration time.Duration
	Rollup   bool
}

var (
	Range1H  = Range{Duration: time.Hour}
	Range6H  = Range{Duration: 6 * time.Hour}
	Range24H = Range{Duration: 24 * time.Hour}
	Range7D  = Range{Duration: 7 * 24 * time.Hour, Rollup: true}
	Range30D = Range{Duration: 30 * 24 * time.Hour, Rollup: true}
)

type Options struct {
	QueueSize          int
	RawRetention       time.Duration
	AggregateRetention time.Duration
	Now                func() time.Time
}

func (o Options) withDefaults() Options {
	if o.QueueSize <= 0 {
		o.QueueSize = 128
	}
	if o.RawRetention <= 0 {
		o.RawRetention = 24 * time.Hour
	}
	if o.AggregateRetention <= 0 {
		o.AggregateRetention = 720 * time.Hour
	}
	if o.Now == nil {
		o.Now = time.Now
	}
	return o
}
