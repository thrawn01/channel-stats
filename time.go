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

const RFC3339Short = "2006-01-02T15"

func NewTimeRange(start, end string) (*TimeRange, error) {
	var startTime, endTime time.Time
	var err error

	if len(start) != 0 {
		startTime, err = time.Parse(RFC3339Short, start)
		if err != nil {
			return nil, errors.Wrapf(err,
				"'start' is not in the format '%s'", RFC3339Short)
		}
	}

	if len(end) != 0 {
		endTime, err = time.Parse(RFC3339Short, end)
		if err != nil {
			return nil, errors.Wrapf(err,
				"'end' is not in the format '%s'", RFC3339Short)
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
	result = append(result, s.Start.Format(RFC3339Short))
	it := s.Start
	for {
		it = it.Add(time.Hour)
		if it.After(s.End) {
			break
		}
		result = append(result, it.Format(RFC3339Short))
	}
	return result
}
