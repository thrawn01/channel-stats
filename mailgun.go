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
	log       *logrus.Entry
	recipient string
	from      string
	disabled  bool
}

func NewNotifier() (Notifier, error) {
	mg, err := mailgun.NewMailgunFromEnv()
	if err != nil {
		return nil, err
	}
	l := log.WithField("prefix", "notifier")

	// TODO: Remove once we fix this upstream
	mg.SetAPIBase("https://api.mailgun.net/v3")
	var disabled bool

	recipient := os.Getenv("MG_RECIPIENT")
	if recipient == "" {
		l.Info("env variable 'MG_RECIPIENT' and 'MG_FROM' must be defined " +
			"to use Mailgun notifications, disabling notifications")
		disabled = true
	}

	from := os.Getenv("MG_FROM")
	if from == "" {
		l.Info("env variable 'MG_RECIPIENT' and 'MG_FROM' must be defined " +
			"to use Mailgun notifications, disabling notifications")
		disabled = true
	}

	return &MailgunNotification{
		recipient: recipient,
		disabled:  disabled,
		log:       l,
		from:      from,
		mg:        mg,
	}, nil
}

func (s *MailgunNotification) Send(msg string) error {

	if s.disabled {
		return nil
	}

	message := s.mg.NewMessage(s.from, mgSubject, msg, s.recipient)
	_, id, err := s.mg.Send(message)
	if err != nil {
		return errors.Wrap(err, "while sending email notification via Mailgun")
	}
	s.log.Infof("Sent notification via mailgun (%s)", id)
	return nil
}
