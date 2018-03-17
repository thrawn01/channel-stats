package channelstats

import (
	"github.com/sirupsen/logrus"
	"github.com/x-cray/logrus-prefixed-formatter"
)

var log *logrus.Logger

func InitLogging(debug bool) {
	log = logrus.New()
	formatter := new(prefixed.TextFormatter)
	formatter.FullTimestamp = true
	log.Formatter = formatter
	if debug {
		log.SetLevel(logrus.DebugLevel)
	}
}
