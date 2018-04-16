package channelstats

import (
	"net/http"
	"os"
	"sync"

	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SlackBot struct {
	store  *Store
	rtm    *slack.RTM
	server *http.Server
	idMgr  *IDManager
	log    *logrus.Entry
	done   chan struct{}
}

func NewSlackBot(store *Store, idMgr *IDManager) *SlackBot {
	return &SlackBot{
		log:   log.WithField("prefix", "bot"),
		idMgr: idMgr,
		store: store,
	}
}

func (s *SlackBot) Start() error {
	s.done = make(chan struct{})
	var wg sync.WaitGroup

	// In a for loop because poorly written gorilla garbage panics occasionally
	for {
		token := os.Getenv("SLACK_TOKEN")
		if token == "" {
			return errors.New("environment variable 'SLACK_TOKEN' empty or missing")
		}

		s.log.Info("Opening RTM WebSocket...")

		api := slack.New(token)
		s.rtm = api.NewRTM()

		wg.Add(1)
		go func() {
			s.rtm.ManageConnection()
			wg.Done()
		}()

		// Return true if we wish to reconnect
		if s.handleEvents() {
			s.rtm.Disconnect()
			wg.Wait()
			continue
		}
		wg.Wait()
		return nil
	}
}

func (s *SlackBot) Stop() {
	s.rtm.Disconnect()
	close(s.done)
}

func (s *SlackBot) handleEvents() (shouldReconnect bool) {
	defer func() {
		// Gorilla Websockets can panic
		if r := recover(); r != nil {
			s.log.Error("Caught Gorilla WebSocket PANIC, reconnecting")
			shouldReconnect = true
		}
	}()

	for {
		select {
		case msg := <-s.rtm.IncomingEvents:
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				s.log.Debugf("Connection counter: %d", ev.ConnectionCount)
			case *slack.ConnectingEvent:
				s.log.Info("Connecting via RTM...")
			case *slack.HelloEvent:
				s.log.Info("Slack said hello... Connected!")
			case *slack.LatencyReport:
				s.log.Debugf("Latency Report '%s'", ev.Value)
			case *slack.MessageEvent:
				s.log.Debugf("Message: %s", ev.Text)
				err := s.store.HandleMessage(ev)
				if err != nil {
					s.log.Errorf("%s", err)
				}
			case *slack.ReactionAddedEvent:
				log.Debugf("Reaction Added By: %s", ev.ItemUser)
				err := s.store.HandleReactionAdded(ev)
				if err != nil {
					s.log.Errorf("%s", err)
				}

				/*info := s.rtm.GetInfo()
				prefix := fmt.Sprintf("<@%s> ", info.User.ID)

				if ev.User != info.User.ID && strings.HasPrefix(ev.Text, prefix) {
					s.rtm.SendMessage(s.rtm.NewOutgoingMessage("What's up buddy!?!?", ev.Channel))
				}*/
			case *slack.ChannelJoinedEvent, *slack.ChannelRenameEvent:
				s.log.Info("Channel Info Updated")
				err := s.idMgr.UpdateChannels()
				if err != nil {
					s.log.Errorf("Error updating channel metadata: %s", err)
				}
			case *slack.RTMError:
				s.log.Errorf("RTM: %s", ev.Error())
			case *slack.InvalidAuthEvent:
				s.log.Error("RTM reports invalid credentials; disconnecting...")
				return false
			case *slack.IncomingEventError:
				log.Errorf("Incoming Error '%+v'; disconnecting...", msg)
				return true
			default:
				s.log.Debugf("Event Received: %+v", msg)
			}
		case <-s.done:
			return false
		}
	}
}
