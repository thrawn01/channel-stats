package channelstats

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"

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

	// Notification configuration
	Mailgun MailgunConfig `json:"mailgun"`
}

type SlackConfig struct {
	LegacyToken string `json:"legacy-token" env:"STATS_SLACK_LEGACY_TOKEN"`
	Token       string `json:"token" env:"STATS_SLACK_TOKEN"`
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
	Operator string `json:"operator" env:"STATS_MG_OPERATOR"`

	// The from email address given when sending operator emails
	From string `json:"from" env:"STATS_MG_FROM"`
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
		if err := RequiredFields(conf.Mailgun, []string{"APIKey", "Domain", "Operator", "From"}); err != nil {
			return conf, fmt.Errorf("config mailgun.%s if mailgun.enabled = true", err)
		}
	}

	if err := RequiredFields(conf.Slack, []string{"LegacyToken", "Token"}); err != nil {
		return conf, fmt.Errorf("config slack.%s", err)
	}

	holster.SetDefault(&conf.Store.DataDir, "./badger-db")

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
			SrcFromEnv(field)
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
