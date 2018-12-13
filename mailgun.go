package channelstats

import (
	"github.com/mailgun/mailgun-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	mgSubject = "[channel-stats] Notification"
)

type Notifier interface {
	Operator(string) error
}

type MailgunNotification struct {
	mg   mailgun.Mailgun
	log  *logrus.Entry
	conf Config
}

func NewNotifier(conf Config) (Notifier, error) {
	if !conf.Mailgun.Enabled {
		return &NullNotifier{}, nil
	}

	if conf.Debug {
		mailgun.Debug = true
	}

	return NewMailgunNotifier(conf)
}

func NewMailgunNotifier(conf Config) (Notifier, error) {

	mg := mailgun.NewMailgun(conf.Mailgun.Domain, conf.Mailgun.APIKey)

	return &MailgunNotification{
		log:  GetLogger().WithField("prefix", "notifier"),
		conf: conf,
		mg:   mg,
	}, nil
}

func (s *MailgunNotification) Operator(msg string) error {
	message := s.mg.NewMessage(s.conf.Mailgun.From, mgSubject, msg, s.conf.Mailgun.Operator)
	_, id, err := s.mg.Send(message)
	if err != nil {
		return errors.Wrap(err, "while sending email notification via Mailgun")
	}
	s.log.Infof("Sent notification via mailgun (%s)", id)
	return nil
}

type NullNotifier struct{}

func (n *NullNotifier) Operator(msg string) error {
	return nil
}
