package channelstats

import (
	"os"

	"github.com/mailgun/mailgun-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

const (
	mgSubject = "[channel-stats] Notification"
)

type Notifier interface {
	Send(string) error
}

type MailgunNotification struct {
	mg        mailgun.Mailgun
	recipient string
	from      string
	log       *logrus.Entry
}

func NewNotifier() (Notifier, error) {
	mg, err := mailgun.NewMailgunFromEnv()
	if err != nil {
		return nil, err
	}

	recipient := os.Getenv("MG_RECIPIENT")
	if recipient == "" {
		return nil, errors.New("env variable 'MG_RECIPIENT' must be defined " +
			"when using Mailgun notifications")
	}

	from := os.Getenv("MG_FROM")
	if recipient == "" {
		return nil, errors.New("env variable 'MG_FROM' must be defined " +
			"when using Mailgun notifications")
	}

	return &MailgunNotification{
		log:       log.WithField("prefix", "notifier"),
		recipient: recipient,
		from:      from,
		mg:        mg,
	}, nil
}

func (s *MailgunNotification) Send(msg string) error {
	message := s.mg.NewMessage(s.from, mgSubject, msg, s.recipient)
	_, id, err := s.mg.Send(message)
	if err != nil {
		return errors.Wrap(err, "while sending email notification via Mailgun")
	}
	s.log.Infof("Sent notification via mailgun (%s)", id)
	return nil
}
