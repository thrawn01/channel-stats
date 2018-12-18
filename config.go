package channelstats

import (
	"flag"
	"fmt"
	"github.com/mailgun/holster/clock"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/fatih/structs"
	"github.com/ghodss/yaml"
	"github.com/mailgun/holster"
)

const (
	confFileUsage = "path to a valid YAML config"
)

type Config struct {
	// Enable debug output via logrus
	Debug bool `json:"debug" env:"STATS_DEBUG"`

	// Slack application config
	Slack SlackConfig `json:"slack"`

	// Store config
	Store StoreConfig `json:"store"`

	// Mailgun configuration
	Mailgun MailgunConfig `json:"mailgun"`

	Report ReportConfig `json:"report"`
}

type SlackConfig struct {
	Token string `json:"token" env:"STATS_SLACK_TOKEN"`
}

type StoreConfig struct {
	DataDir string `json:"data-dir" env:"STATS_STORE_DATA_DIR"`
}
type MailgunConfig struct {
	// Enable sending notifications via mailgun
	Enabled bool `json:"enabled" env:"STATS_MG_ENABLED"`

	// API key used to authenticate your account with the mailgun API
	APIKey string `json:"api-key" env:"STATS_MG_API_KEY"`

	// The name of the domain your sending email from
	Domain string `json:"domain" env:"STATS_MG_DOMAIN"`

	// The email address of the operator of the bot
	OperatorAddr string `json:"operator-address" env:"STATS_MG_OPERATOR_ADDR"`

	// The email address reports are sent to (Could be an mailing list address)
	ReportAddr string `json:"report-address" env:"STATS_MG_REPORT_ADDR"`

	// The from email address given when sending operator emails
	From string `json:"from" env:"STATS_MG_FROM"`

	// Timeout for network operations when talking to mailgun
	// (See http://golang.org/pkg/time/#ParseDuration for string format)
	// Defaults to "20s" (20 Seconds)
	Timeout clock.DurationJSON `json:"timeout" env:"STATS_MG_TIMEOUT"`
}

type ReportConfig struct {
	// The cron like string the dictates when reports are sent to users
	// (See https://godoc.org/github.com/robfig/cron#hdr-CRON_Expression_Format)
	// Default is "0 0 0 * * SUN" - Run once a week, midnight on Sunday
	Schedule string `json:"schedule" env:"STATS_REPORT_SCHEDULE"`

	// The duration used to decide the start and end hour of the report
	// (See http://golang.org/pkg/time/#ParseDuration for string format)
	// Defaults to "168h" aka 7 days
	ReportDuration clock.DurationJSON `json:"report-duration" env:"STATS_REPORT_DURATION"`
}

func LoadConfig() (Config, error) {
	var conf Config
	var confFile string

	flag.StringVar(&confFile, "config", "", confFileUsage)
	flag.StringVar(&confFile, "c", "", confFileUsage+" (shorthand)")
	flag.Parse()

	SrcFromEnv(&conf)

	if confFile != "" {
		fd, err := os.Open(confFile)
		if err != nil {
			return conf, fmt.Errorf("while opening config file: %s", err)
		}

		content, err := ioutil.ReadAll(fd)
		if err != nil {
			return conf, fmt.Errorf("while reading config file '%s': %s", confFile, err)
		}

		if err := yaml.Unmarshal(content, &conf); err != nil {
			return conf, fmt.Errorf("while marshalling config file '%s': %s", confFile, err)
		}
	}

	if conf.Mailgun.Enabled {
		// Ensure mailgun required fields are provided
		if err := RequiredFields(conf.Mailgun, []string{"APIKey", "Domain", "From"}); err != nil {
			return conf, fmt.Errorf("config mailgun.%s if mailgun.enabled = true", err)
		}
	}

	if err := RequiredFields(conf.Slack, []string{"Token"}); err != nil {
		return conf, fmt.Errorf("config slack.%s", err)
	}

	holster.SetDefault(&conf.Store.DataDir, "./badger-db")
	holster.SetDefault(&conf.Report.Schedule, "0 0 0 * * SUN")
	holster.SetDefault(&conf.Report.ReportDuration, time.Hour*168)
	holster.SetDefault(&conf.Mailgun.Timeout, time.Second*20)

	return conf, nil
}

func RequiredFields(obj interface{}, fields []string) error {
	s := structs.New(obj)

	for _, name := range fields {
		f := s.Field(name)
		if f.IsZero() {
			return fmt.Errorf("%s is required", f.Tag("json"))
		}
	}
	return nil
}

type FieldsInterface interface {
	Fields() []*structs.Field
}

func SrcFromEnv(obj interface{}) {
	var s FieldsInterface
	var ok bool

	if s, ok = obj.(*structs.Field); !ok {
		s = structs.New(obj)
	}

	for _, field := range s.Fields() {
		if field.Kind() == reflect.Struct {
			// Handle Durations
			if _, ok := field.Value().(clock.DurationJSON); ok {
				if tag := field.Tag("env"); tag != "" {
					if value := os.Getenv(tag); value != "" {
						field.Set(clock.NewDurationJSONOrPanic(tag))
					}
				}
			} else {
				SrcFromEnv(field)
			}
			continue
		}

		var val interface{}
		if field.Kind() == reflect.Bool {
			strVal := strings.ToLower(os.Getenv(field.Tag("env")))
			if strVal == "true" || strVal == "yes" {
				val = true
			} else {
				val = false
			}
		} else {
			val = os.Getenv(field.Tag("env"))
		}

		if err := field.Set(val); err != nil {
			panic(fmt.Sprintf("While setting '%s' - %s", field.Name(), err))
		}
	}
}
