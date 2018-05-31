package channelstats

import (
	"fmt"
	"net/http"
	"os"
	"runtime/debug"
	"sync"

	"sync/atomic"
	"time"

	"github.com/nlopes/slack"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

type SlackBot struct {
	log      *logrus.Entry
	done     chan struct{}
	server   *http.Server
	rtm      *slack.RTM
	idMgr    *IDManager
	notifier Notifier
	store    *Store
}

func NewSlackBot(store *Store, idMgr *IDManager, notifier Notifier) *SlackBot {
	return &SlackBot{
		log:      log.WithField("prefix", "bot"),
		notifier: notifier,
		idMgr:    idMgr,
		store:    store,
	}
}

func (s *SlackBot) Start() error {
	s.done = make(chan struct{})
	var wg sync.WaitGroup
	var connected int32

	go func() {
		ticker := time.Tick(time.Second * 30)
		var disconnected bool
		var notified bool

		// TODO: Shut this down gracefully
		for {
			select {
			case <-ticker:
				if disconnected {
					// If we are still disconnected
					if atomic.LoadInt32(&connected) == 0 {
						// Only notify once
						if notified {
							continue
						}

						err := s.notifier.Send("channel-stats has been disconnected from " +
							"slack for more than 30 seconds")
						if err != nil {
							s.log.Errorf("while sending notification - %s", err)
							continue
						}
						notified = true
					}
				}

				// If we are disconnected, Wait another 30 seconds
				if atomic.LoadInt32(&connected) == 0 {
					disconnected = true
				} else {
					disconnected = false
					notified = false
				}
			}
		}
	}()

	// In a for loop because poorly written gorilla garbage panics occasionally
	for {
		token := os.Getenv("SLACK_TOKEN")
		if token == "" {
			return errors.New("environment variable 'SLACK_TOKEN' empty or missing")
		}

		s.log.Info("Opening RTM WebSocket...")
		atomic.StoreInt32(&connected, 1)

		api := slack.New(token)
		s.rtm = api.NewRTM()

		wg.Add(1)
		go func() {
			s.rtm.ManageConnection()
			wg.Done()
			log.Debug("ManageConnection() done")
		}()

		// Return true if we wish to reconnect
		if s.handleEvents() {
			atomic.StoreInt32(&connected, 0)
			log.Debug("Reconnecting...")
			s.rtm.Disconnect()
			wg.Wait()
			continue
		}
		atomic.StoreInt32(&connected, 0)
		log.Debug("Disconnecting...")
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
			fmt.Printf("Caught PANIC in handleEvents(), reconnecting\n")
			debug.PrintStack()
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
				userName, err := s.idMgr.GetUserName(ev.User)
				if err != nil {
					s.log.Debug("Unknown user message: %+v", ev)
					continue
				}

				s.log.Debugf("Message: [%s] %s", userName, ev.Text)
				err = s.store.HandleMessage(ev)
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
				return
			case *slack.IncomingEventError:
				log.Errorf("Incoming Error '%+v'", msg)
				//shouldReconnect = true
				//return true
			case *slack.DisconnectedEvent:
				log.Errorf("Disconnected...", msg)
				//shouldReconnect = true
				//return true
			default:
				s.log.Debugf("Event Received: %+v", msg)
			}
		case <-s.done:
			return
		}
	}
}
