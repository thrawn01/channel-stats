package channelstats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"

	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SlackChannelInfo struct {
	Id   string
	Name string
}

type SlackChannelList struct {
	Ok       bool
	Error    string
	Channels []SlackChannelInfo
}

type ChannelManager struct {
	channels map[string]string
	log      *logrus.Entry
	token    string
}

func NewChannelManager() (*ChannelManager, error) {
	token := os.Getenv("SLACK_LEGACY_TOKEN")
	if token == "" {
		return nil, errors.New("environment variable 'SLACK_LEGACY_TOKEN' empty or missing")
	}
	s := ChannelManager{
		log:   log.WithField("prefix", "channel-manager"),
		token: token,
	}
	// Populate our channel listing
	return &s, s.UpdateChannels()
}

func (s *ChannelManager) UpdateChannels() (err error) {
	s.channels, err = s.fetchListing()
	if err != nil {
		return err
	}
	return nil
}

func (s *ChannelManager) fetchListing() (map[string]string, error) {
	params := url.Values{}
	params.Add("token", s.token)

	s.log.Info("Fetching Channel Listing...")
	url := fmt.Sprintf("https://slack.com/api/channels.list?%s", params.Encode())
	resp, err := http.Get(url)
	if err != nil {
		return nil, errors.Wrapf(err, "GET '%s' failed with '%d'", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	// Parse the response
	var channelList SlackChannelList
	err = json.NewDecoder(resp.Body).Decode(&channelList)
	if err != nil {
		return nil, errors.Wrap(err, "GET '%s' failed during json decode")
	}

	// Handle slack error
	if !channelList.Ok {
		return nil, errors.Errorf("GET '%s' failed with slack error '%s'", channelList.Error)
	}

	// Extract channel name and id's
	results := make(map[string]string, len(channelList.Channels))
	for _, channel := range channelList.Channels {
		results[channel.Name] = channel.Id
	}

	return results, nil
}

func (s *ChannelManager) GetID(name string) (string, error) {
	if id, exists := s.channels[name]; exists {
		return id, nil
	}
	return "", errors.Errorf("channel '%s' not found")
}
