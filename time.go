package channelstats

import (
	"fmt"
	"time"

	"github.com/pkg/errors"
)

type TimeRange struct {
	Start time.Time
	End   time.Time
}

func NewTimeRange(start, end string) (*TimeRange, error) {

	// Cheap, but works for now
	if len(start) == 0 || len(end) == 0 {
		return &TimeRange{time.Now().UTC().AddDate(0, -1, 0), time.Now()}, nil
	}

	startTime, err := time.Parse(time.RFC1123, start)
	if err != nil {
		return nil, errors.Wrap(err, "'start' is not valid 'RFC1123 time string")
	}
	endTime, err := time.Parse(time.RFC1123, end)
	if err != nil {
		return nil, errors.Wrap(err, "'end' is not valid 'RFC1123 time string")
	}

	if endTime.Before(startTime) {
		return nil, fmt.Errorf("'end' is before 'start' time")
	}

	return &TimeRange{
		Start: startTime,
		End:   endTime,
	}, nil
}

func (s *TimeRange) ByHour() []string {
	var result []string
	it := s.Start
	for {
		result = append(result, it.Format(hourLayout))
		it.Add(time.Hour)
		if it.Before(s.End) {
			continue
		}
	}
	return result
}
