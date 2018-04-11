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
	byName map[string]string
	byID   map[string]string
	log    *logrus.Entry
	token  string
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

func (s *ChannelManager) UpdateChannels() error {
	return s.fetchListing()
}

func (s *ChannelManager) fetchListing() error {
	params := url.Values{}
	params.Add("token", s.token)

	s.log.Info("Fetching Channel Listing...")
	url := fmt.Sprintf("https://slack.com/api/channels.list?%s", params.Encode())
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrapf(err, "GET '%s' failed with '%d'", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	// Parse the response
	var channelList SlackChannelList
	err = json.NewDecoder(resp.Body).Decode(&channelList)
	if err != nil {
		return errors.Wrap(err, "GET '%s' failed during json decode")
	}

	// Handle slack error
	if !channelList.Ok {
		return errors.Errorf("GET '%s' failed with slack error '%s'", channelList.Error)
	}

	// Extract channel name and id's
	s.byName = make(map[string]string, len(channelList.Channels))
	s.byID = make(map[string]string, len(channelList.Channels))
	for _, channel := range channelList.Channels {
		s.log.Debugf("Found Channel: %s - %s", channel.Name, channel.Id)
		s.byName[channel.Name] = channel.Id
		s.byID[channel.Id] = channel.Name
	}

	return nil
}

func (s *ChannelManager) GetID(name string) (string, error) {
	if id, exists := s.byName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel '%s' not found", name)
}

func (s *ChannelManager) GetName(id string) (string, error) {
	if id, exists := s.byID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel id '%s' not found", id)
}
