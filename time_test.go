package channelstats_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/thrawn01/channel-stats"
)

func TestTime(t *testing.T) {
	suite.Run(t, new(TimeSuite))
}

type TimeSuite struct {
	suite.Suite
}

func (s *TimeSuite) TestByHour() {
	timeRange, err := channelstats.NewTimeRange(
		"2006-01-02T15", "2006-01-02T20")
	s.Require().NoError(err)
	s.Equal([]string{
		"2006-01-02T15",
		"2006-01-02T16",
		"2006-01-02T17",
		"2006-01-02T18",
		"2006-01-02T19",
		"2006-01-02T20",
	}, timeRange.ByHour())
}
