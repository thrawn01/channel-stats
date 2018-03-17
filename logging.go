package channelstats

import "github.com/sirupsen/logrus"

var log *logrus.Logger

func InitLogging(debug bool) {
	log = logrus.New()
	if debug {
		log.SetLevel(logrus.DebugLevel)
	}
}
