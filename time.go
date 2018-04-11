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
	var startTime, endTime time.Time
	var err error

	if len(start) != 0 {
		startTime, err = time.Parse(time.RFC1123, start)
		if err != nil {
			return nil, errors.Wrap(err, "'start' is not valid 'RFC1123 time string")
		}
	}

	if len(end) != 0 {
		endTime, err = time.Parse(time.RFC1123, end)
		if err != nil {
			return nil, errors.Wrap(err, "'end' is not valid 'RFC1123 time string")
		}
	}

	now := time.Now()
	// If no start time specified, choose 1 week (aka 7 days)
	if startTime.IsZero() {
		startTime = now.AddDate(0, 0, -7)
	}

	// If no end time specified, choose now
	if endTime.IsZero() {
		endTime = now
	}

	if endTime.Before(startTime) {
		return nil, fmt.Errorf("'end' is before 'start' time")
	}

	return &TimeRange{
		Start: startTime.UTC(),
		End:   endTime.UTC(),
	}, nil
}

func (s *TimeRange) ByHour() []string {
	var result []string
	result = append(result, s.Start.Format(hourLayout))
	it := s.Start
	for {
		it = it.Add(time.Hour)
		if it.After(s.End) {
			break
		}
		result = append(result, it.Format(hourLayout))
	}
	return result
}
