package channelstats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

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

type SlackUserInfo struct {
	Id   string
	Name string
}

type SlackUserList struct {
	Ok      bool
	Error   string
	Members []SlackUserInfo
}

type IDManager struct {
	channelByName map[string]string
	channelByID   map[string]string
	userByName    map[string]string
	userByID      map[string]string
	log           *logrus.Entry
	token         string
}

func NewIdManager(conf Config) (*IDManager, error) {
	s := IDManager{
		log:   GetLogger().WithField("prefix", "id-manager"),
		token: conf.Slack.LegacyToken,
	}
	// Populate our channel listing
	if err := s.UpdateChannels(); err != nil {
		return nil, err
	}
	if err := s.UpdateUsers(); err != nil {
		return nil, err
	}
	return &s, nil
}

func (s *IDManager) UpdateUsers() error {
	params := url.Values{}
	params.Add("token", s.token)

	s.log.Info("Fetching User Listing...")
	url := fmt.Sprintf("https://slack.com/api/users.list?%s", params.Encode())
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrapf(err, "GET '%s' failed with '%d'", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	// Parse the response
	var userList SlackUserList
	err = json.NewDecoder(resp.Body).Decode(&userList)
	if err != nil {
		return errors.Wrap(err, "GET '%s' failed during json decode")
	}

	// Handle slack error
	if !userList.Ok {
		return errors.Errorf("GET '%s' failed with slack error '%s'", userList.Error)
	}

	// Extract user name and id's
	s.userByName = make(map[string]string, len(userList.Members))
	s.userByID = make(map[string]string, len(userList.Members))
	for _, user := range userList.Members {
		s.log.Debugf("Found User: %s - %s", user.Name, user.Id)
		s.userByName[user.Name] = user.Id
		s.userByID[user.Id] = user.Name
	}

	return nil
}

func (s *IDManager) UpdateChannels() error {
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
	s.channelByName = make(map[string]string, len(channelList.Channels))
	s.channelByID = make(map[string]string, len(channelList.Channels))
	for _, channel := range channelList.Channels {
		s.log.Debugf("Found Channel: %s - %s", channel.Name, channel.Id)
		s.channelByName[channel.Name] = channel.Id
		s.channelByID[channel.Id] = channel.Name
	}

	return nil
}

func (s *IDManager) GetChannelID(name string) (string, error) {
	if id, exists := s.channelByName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel '%s' not found", name)
}

func (s *IDManager) GetChannelName(id string) (string, error) {
	if id, exists := s.channelByID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel id '%s' not found", id)
}

func (s *IDManager) GetUserID(name string) (string, error) {
	if id, exists := s.userByName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user '%s' not found", name)
}

func (s *IDManager) GetUserName(id string) (string, error) {
	if id, exists := s.userByID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user id '%s' not found", id)
}
