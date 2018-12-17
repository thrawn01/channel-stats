package channelstats

import (
	"bufio"
	"bytes"
	"github.com/pkg/errors"
	"github.com/robfig/cron"
	"github.com/sirupsen/logrus"
	"github.com/thrawn01/channel-stats/assets"
	"html/template"
	"io"
	"time"
)

type RenderFunc func(store Storer, w io.Writer, timeRange *TimeRange, counter, channelID string) error

// Any struct that can return a list of channels to create reports for
type ChanLister interface {
	Channels() []SlackChannelInfo
}

type Reporter interface {
	Start() error
	Stop()
}

type Report struct {
	log   *logrus.Entry
	cron  *cron.Cron
	list  ChanLister
	conf  Config
	mail  Mailer
	store Storer
}

func NewReporter(conf Config, list ChanLister, notify Mailer, store Storer) (Reporter, error) {
	r := Report{
		log:   GetLogger().WithField("prefix", "reporter"),
		cron:  cron.New(),
		mail:  notify,
		store: store,
		conf:  conf,
		list:  list,
	}
	return &r, r.Start()
}

func (r *Report) Start() error {
	err := r.cron.AddFunc(r.conf.Report.Schedule, func() {
		timeRange := toTimeRange(r.conf.Report.ReportDuration.Duration)

		for _, channel := range r.list.Channels() {
			// Skip channels the bot is not in
			if !channel.IsMember {
				continue
			}

			html, err := r.genHtml("templates/email.tmpl", channel.Name)
			if err != nil {
				r.log.Errorf("during email generate: %s", err)
			}

			data := ReportData{
				Images: make(map[string][]byte),
				Html:   html,
			}

			// Generate the images for the report
			data.Images["most-active"] = r.genImage(RenderSum, timeRange, channel.Id, "message")
			data.Images["top-links"] = r.genImage(RenderSum, timeRange, channel.Id, "link")
			data.Images["top-emoji"] = r.genImage(RenderSum, timeRange, channel.Id, "emoji")
			data.Images["most-negative"] = r.genImage(RenderPercentage, timeRange, channel.Id, "negative")
			data.Images["most-positive"] = r.genImage(RenderPercentage, timeRange, channel.Id, "positive")

			// Email the report
			/*if err := r.mail.Report(channel, &ReportData{}); err != nil {
				r.log.Errorf("while sending report: %s")
			}*/
		}
	})
	if err != nil {
		return err
	}

	r.cron.Start()
	return nil
}

func (r *Report) Stop() {
	r.cron.Stop()
}

func (r *Report) genImage(render RenderFunc, timeRange *TimeRange, channel, counter string) []byte {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)

	if err := render(r.store, w, timeRange, channel, counter); err != nil {
		r.log.Errorf("while rendering image for '%s' with range '%+v' on channel '%s'",
			counter, timeRange, channel)
	}
	return buf.Bytes()
}

func (r *Report) genHtml(file string, chanName string) ([]byte, error) {
	type Data struct {
		Name string
	}

	content, err := assets.Get(file)

	t, err := template.New("email").Parse(string(content))
	if err != nil {
		return nil, errors.Wrapf(err, "while parsing template '%s'", file)
	}

	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	if err = t.Execute(w, Data{Name: chanName}); err != nil {
		return nil, errors.Wrapf(err, "while executing template '%s'", file)
	}

	return buf.Bytes(), nil
}

func toTimeRange(duration time.Duration) *TimeRange {
	startHour := time.Now().UTC()
	endHour := startHour.Add(-duration)
	return &TimeRange{
		Start: startHour,
		End:   endHour,
	}
}
