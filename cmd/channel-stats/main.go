package main

import (
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"sync"

	"github.com/cdipaolo/sentiment"
	"github.com/dgraph-io/badger"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

type SlackBot struct {
	db     *badger.DB
	rtm   *slack.RTM
	model sentiment.Models
	done  chan struct{}
}

func (s *SlackBot) Start() error {
	s.done = make(chan struct{})
	var err error

	// init the sentiment model
	s.model, err = sentiment.Restore()
	if err != nil {
		return err
	}

	s.db, err = initBadger()
	if err != nil {
		return err
	}

	// In a for loop because poorly written gorilla garbage panics occasionally
	for {
		var wg sync.WaitGroup


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
				countMessage(ev)

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

func (s *SlackBot) Stop() {
	s.rtm.Disconnect()
	db.Close()
	close(s.done)
}

func toHour(epoc string) string {
	float, err := strconv.ParseFloat(epoc, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "timestamp conversion for '%s' - %s", epoc, err)
	}
	timestamp := time.Unix(0, int64(float*1000000)*int64(time.Microsecond/time.Nanosecond)).UTC()
	return timestamp.Format("2006-01-02T15")
}

func datapoint(items ...string) []byte {
	return []byte(strings.Join(items, "/"))
}

func increment(txn *badger.Txn, key []byte) error {
	// Fetch this data point from the store
	item, err := txn.Get(key)
	if err != nil {
		if err != badger.ErrKeyNotFound {
			return errors.Wrapf(err, "while fetching key '%s'", key)
		}
	}

	var counter int64
	// If value exists in the store, retrieve the current counter
	if item != nil {
		value, err := item.Value()
		if value != nil {
			return errors.Wrapf(err, "while fetching counter value '%s'", key)
		}
		counter, err = strconv.ParseInt(string(value), 10, 64)
	}
	// Increment our counter
	counter += 1
	err = txn.Set(key, []byte(fmt.Sprintf("%d", counter)))
	if err != nil {
		return errors.Wrapf(err, "while setting counter for key '%s'", key)
	}
	return err
}

func countMessage(ev *slack.MessageEvent) {
	// get the hour from message timestamp
	hour := toHour(ev.Timestamp)

	// Start a badger transaction
	err := db.Update(func(txn *badger.Txn) error {
		return increment(txn, datapoint(hour, ev.User, "messages", ev.Channel))
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "while recording message counts: %s\n", err)
	}

	// Increment the messages counter for this hour
	/*increment(hour, ev.Username, "messages", ev.Channel)

	// Increment the sentiment counters for this message
	if sModel.SentimentAnalysis(ev.Text, sentiment.English).Score == 1 {
		increment(hour, ev.Username, "positive", ev.Channel)
	} else {
		increment(hour, ev.Username, "negative", ev.Channel)
	}*/
}

func initBadger() (*badger.DB, error) {
	opts := badger.DefaultOptions
	opts.Dir = "./badger"
	opts.ValueDir = "./badger"
	opts.SyncWrites = true

	db, err := badger.Open(opts)
	if err != nil {
		return nil, errors.Wrap(err, "while opening badger database")
	}
	return db, nil
}


// Globals, because why not
var (
)

func main() {
	bot := SlackBot{}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			if sig == os.Interrupt {
				// Close badger so we don't have LOCK errors
				fmt.Printf("caught interupt; user requested premature exit\n")
				bot.Stop()
			}
		}
	}()

	if err := bot.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "-- %s\n", err)
		os.Exit(1)
	}
	fmt.Println("-- done")
}
