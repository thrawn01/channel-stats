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
	Name     string `json:"name"`
	Id       string `json:"id"`
	IsMember bool   `json:"is_member"`
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

type IDManager interface {
	UpdateUsers() error
	UpdateChannels() error
	GetChannelID(name string) (string, error)
	GetChannelName(id string) (string, error)
	GetUserID(name string) (string, error)
	GetUserName(id string) (string, error)
	Channels() []SlackChannelInfo
}

type IDManage struct {
	channelList   SlackChannelList
	channelByName map[string]string
	channelByID   map[string]string
	userByName    map[string]string
	userByID      map[string]string
	log           *logrus.Entry
	token         string
}

func NewIdManager(conf Config) (IDManager, error) {
	im := IDManage{
		log:   GetLogger().WithField("prefix", "id-manager"),
		token: conf.Slack.Token,
	}
	// Populate our channel listing
	if err := im.UpdateChannels(); err != nil {
		return nil, err
	}
	if err := im.UpdateUsers(); err != nil {
		return nil, err
	}
	return &im, nil
}

func (im *IDManage) Channels() []SlackChannelInfo {
	return im.channelList.Channels
}

func (im *IDManage) UpdateUsers() error {
	params := url.Values{}
	params.Add("token", im.token)

	im.log.Info("Fetching User Listing...")
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
		return errors.Wrapf(err, "GET '%s' failed during json decode", url)
	}

	// Handle slack error
	if !userList.Ok {
		return errors.Errorf("GET '%s' failed with slack error '%s'", url, userList.Error)
	}

	// Extract user name and id's
	im.userByName = make(map[string]string, len(userList.Members))
	im.userByID = make(map[string]string, len(userList.Members))
	for _, user := range userList.Members {
		im.log.Debugf("Found User: %s - %s", user.Name, user.Id)
		im.userByName[user.Name] = user.Id
		im.userByID[user.Id] = user.Name
	}

	return nil
}

func (im *IDManage) UpdateChannels() error {
	params := url.Values{}
	params.Add("token", im.token)

	im.log.Info("Fetching Channel Listing...")
	url := fmt.Sprintf("https://slack.com/api/channels.list?%s", params.Encode())
	resp, err := http.Get(url)
	if err != nil {
		return errors.Wrapf(err, "GET '%s' failed with '%d'", url, resp.StatusCode)
	}
	defer resp.Body.Close()

	// Parse the response
	err = json.NewDecoder(resp.Body).Decode(&im.channelList)
	if err != nil {
		return errors.Wrapf(err, "GET '%s' failed during json decode", url)
	}

	// Handle slack error
	if !im.channelList.Ok {
		return errors.Errorf("GET '%s' failed with slack error '%s'", url, im.channelList.Error)
	}

	// Extract channel name and id's
	im.channelByName = make(map[string]string, len(im.channelList.Channels))
	im.channelByID = make(map[string]string, len(im.channelList.Channels))
	for _, channel := range im.channelList.Channels {
		im.log.Debugf("Found Channel: %s - %s", channel.Name, channel.Id)
		im.channelByName[channel.Name] = channel.Id
		im.channelByID[channel.Id] = channel.Name
	}

	return nil
}

func (im *IDManage) GetChannelID(name string) (string, error) {
	if id, exists := im.channelByName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel '%s' not found", name)
}

func (im *IDManage) GetChannelName(id string) (string, error) {
	if id, exists := im.channelByID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("channel id '%s' not found", id)
}

func (im *IDManage) GetUserID(name string) (string, error) {
	if id, exists := im.userByName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user '%s' not found", name)
}

func (im *IDManage) GetUserName(id string) (string, error) {
	if id, exists := im.userByID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user id '%s' not found", id)
}

// Suitable for testing
type MockIDManage struct {
	UserByName map[string]string
	UserByID   map[string]string
}

func (n *MockIDManage) UpdateUsers() error { return nil }

func (n *MockIDManage) UpdateChannels() error { return nil }

func (n *MockIDManage) GetChannelID(name string) (string, error) { return "C02C073ND", nil }

func (n *MockIDManage) GetChannelName(id string) (string, error) { return "general", nil }

func (n *MockIDManage) GetUserID(name string) (string, error) {
	if id, exists := n.UserByName[name]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user '%s' not found", name)
}

func (n *MockIDManage) GetUserName(id string) (string, error) {
	if id, exists := n.UserByID[id]; exists {
		return id, nil
	}
	return "(unknown)", errors.Errorf("user id '%s' not found", id)
}
