package channelstats

import (
	"github.com/sirupsen/logrus"
	"github.com/x-cray/logrus-prefixed-formatter"
)

var logger *logrus.Logger

func GetLogger() *logrus.Logger {
	return logger
}

func InitLogging(conf Config) {
	logger = logrus.New()
	formatter := new(prefixed.TextFormatter)
	formatter.FullTimestamp = true
	logger.Formatter = formatter
	if conf.Debug {
		logger.SetLevel(logrus.DebugLevel)
	}
}
