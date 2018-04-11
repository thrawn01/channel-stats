package channelstats_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/thrawn01/channel-stats"
)

func TestStore(t *testing.T) {
	suite.Run(t, new(StoreSuite))
}

type StoreSuite struct {
	suite.Suite
}

func (s *TimeSuite) TestHasLink() {
	s.Equal(true, channelstats.HasLink(
		"This is a cool link http://google.com about monkeys"))
	s.Equal(true, channelstats.HasLink(
		"This is a cool link https://google.com about monkeys"))
	s.Equal(false, channelstats.HasLink(
		"This is a cool link on google.com about monkeys"))
}
