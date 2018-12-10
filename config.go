package channelstats

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/fatih/structs"
	"github.com/mailgun/holster"
	"github.com/ghodss/yaml"
)

const (
	confFileDefault = "./channel-stats.yaml"
	confFileUsage   = "path to a valid YAML config"
)

type Config struct {

	// Enable debug output via logrus
	Debug bool `json:"debug"`

	Slack SlackConfig `json:"slack"`

	Store StoreConfig `json:"store"`

	// Notification configuration
	Mailgun MailgunConfig `json:"mailgun"`
}

type SlackConfig struct {
	LegacyToken string `json:"legacy_token"`
	Token       string `json:"token"`
}

type StoreConfig struct {
	DataDir string `json:"data-dir"`
}
type MailgunConfig struct {
	// Enable sending notifications via mailgun
	Enabled bool `json:"enabled"`

	// API key used to authenticate your account with the mailgun API
	APIKey string `json:"api_key"`

	// The name of the domain your sending email from
	Domain string `json:"domain"`

	// The address of the recipient or mailing list that will receive periodic stats
	Recipient string `json:"recipient"`

	// The from address given when sending stats
	From string `json:"from"`
}

func LoadConfig() (Config, error) {
	var conf Config
	var confFile string

	flag.StringVar(&confFile, "config", confFileDefault, confFileUsage)
	flag.StringVar(&confFile, "c", confFileDefault, confFileUsage+" (shorthand)")
	flag.Parse()

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

	if conf.Mailgun.Enabled {
		// Ensure mailgun required fields are provided
		if err := RequiredFields(conf.Mailgun, []string{"APIKey", "Domain", "Recipient", "From"}); err != nil {
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
