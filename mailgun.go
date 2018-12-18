package channelstats

import (
	"fmt"
	"github.com/mailgun/mailgun-go"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"net/http"
)

type ReportData struct {
	// The HTML that makes up the body of the report
	Html []byte
	// The images that are the rendered
	Images map[string][]byte
}

type Mailer interface {
	Operator(string) error
	Report(string, ReportData) error
}

type Mailgun struct {
	mg   mailgun.Mailgun
	log  *logrus.Entry
	conf Config
}

func NewMailer(conf Config) (Mailer, error) {
	if !conf.Mailgun.Enabled {
		return &NullMailer{}, nil
	}

	mailgun.Debug = conf.Debug
	return NewMailgunNotifier(conf)
}

func NewMailgunNotifier(conf Config) (Mailer, error) {

	mg := mailgun.NewMailgun(conf.Mailgun.Domain, conf.Mailgun.APIKey)

	// Set a reasonable timeout on network operations
	client := http.DefaultClient
	client.Timeout = conf.Mailgun.Timeout.Duration
	mg.SetClient(client)

	return &Mailgun{
		log:  GetLogger().WithField("prefix", "notifier"),
		conf: conf,
		mg:   mg,
	}, nil
}

// Send a report to the designated email address (could be mailing list)
func (m *Mailgun) Report(channelName string, data ReportData) error {
	if m.conf.Mailgun.ReportAddr == "" {
		m.log.Errorf("mailgun.enabled = true; however mailgun.report-address is empty; skipping..")
		return nil
	}

	// Create a subject for the report
	subject := fmt.Sprintf("[channel-stats] Report for %s", channelName)
	// Create a message with no text body
	message := m.mg.NewMessage(m.conf.Mailgun.From, subject, "", m.conf.Mailgun.ReportAddr)
	// Send the HTML to mailgun for MIME encoding
	message.SetHtml(string(data.Html))

	for file, contents := range data.Images {
		message.AddBufferAttachment(file, contents)
	}
	_, id, err := m.mg.Send(message)
	if err != nil {
		return err
	}
	m.log.Infof("Sent report via mailgun (%s)", id)
	return nil
}

// Send an email message to the designated operator the this chat bot
func (m *Mailgun) Operator(msg string) error {
	if m.conf.Mailgun.OperatorAddr == "" {
		m.log.Errorf("mailgun.enabled = true; however mailgun.operator-address is empty; skipping..")
		return nil
	}

	message := m.mg.NewMessage(m.conf.Mailgun.From, "[channel-stats] Operator Notification",
		msg, m.conf.Mailgun.OperatorAddr)
	_, id, err := m.mg.Send(message)
	if err != nil {
		return errors.Wrap(err, "while sending operator notification via Mailgun")
	}
	m.log.Infof("Notified operator via mailgun (%s)", id)
	return nil
}

type NullMailer struct{}

func (n *NullMailer) Operator(msg string) error {
	return nil
}

func (n *NullMailer) Report(channel string, data ReportData) error {
	return nil
}
