package channelstats

import (
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

type SlackBot struct {
	store  *Store
	rtm    *slack.RTM
	server *http.Server
	done   chan struct{}
}

func NewSlackBot(store *Store) *SlackBot {
	return &SlackBot{store: store}
}

func (s *SlackBot) Start() error {
	s.done = make(chan struct{})
	var wg sync.WaitGroup

	/*mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/all", s.getAll)

	s.server = &http.Server{Addr: "0.0.0.0:1313", Handler: mux}

	wg.Add(1)
	go func() {
		fmt.Println("-- Listening on http://0.0.0.0:1313")

		if err := s.server.ListenAndServe(); err != nil {
			if err != http.ErrServerClosed {
				fmt.Fprintf(os.Stderr, "during ListAndServe(): %s\n", err)
				s.Stop()
			}
		}
		wg.Done()
	}()*/

	// In a for loop because poorly written gorilla garbage panics occasionally
	for {
		token := os.Getenv("SLACK_TOKEN")
		if token == "" {
			return errors.New("environment variable 'SLACK_TOKEN' empty or missing")
		}

		fmt.Printf("Connecting to slack...\n")

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
			fmt.Printf("Caught PANIC, reconnecting\n")
			shouldReconnect = true
		}
	}()

	for {
		select {
		case msg := <-s.rtm.IncomingEvents:
			fmt.Printf("Event Received: %s\n", msg)
			switch ev := msg.Data.(type) {
			case *slack.ConnectedEvent:
				fmt.Printf("Connection counter: %d\n", ev.ConnectionCount)

			case *slack.MessageEvent:
				fmt.Printf("Message: %v\n", ev)
				err := s.store.HandleMessage(ev)
				if err != nil {
					fmt.Fprint(os.Stderr, "-- %s\n", err)
				}

				info := s.rtm.GetInfo()
				prefix := fmt.Sprintf("<@%s> ", info.User.ID)

				if ev.User != info.User.ID && strings.HasPrefix(ev.Text, prefix) {
					s.rtm.SendMessage(s.rtm.NewOutgoingMessage("What's up buddy!?!?", ev.Channel))
				}

			case *slack.RTMError:
				fmt.Printf("Error: %s\n", ev.Error())

			case *slack.InvalidAuthEvent:
				fmt.Println("Invalid credentials")
				return false
			default:
				//Take no action
			}
		case <-s.done:
			return false
		}
	}
}
