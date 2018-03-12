package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	hc "cirello.io/HumorChecker"
	"github.com/dgraph-io/badger"
	"github.com/nlopes/slack"
	"github.com/pkg/errors"
)

type Response struct {
	Items []Pair `json:"items"`
}

type Pair struct {
	Key   string
	Value string
}

type SlackBot struct {
	db     *badger.DB
	rtm    *slack.RTM
	server *http.Server
	done   chan struct{}
}

func (s *SlackBot) Start() error {
	s.done = make(chan struct{})
	var wg sync.WaitGroup
	var err error

	s.db, err = initBadger()
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.index)

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
	}()

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
	s.server.Shutdown(context.Background())
	s.rtm.Disconnect()
	s.db.Close()
	close(s.done)
}

func (s *SlackBot) index(w http.ResponseWriter, r *http.Request) {
	fmt.Println("GET /")
	var response Response

	err := s.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10
		it := txn.NewIterator(opts)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			k := item.Key()
			v, err := item.Value()
			if err != nil {
				return err
			}

			response.Items = append(response.Items, Pair{Key: string(k), Value: string(v)})
		}
		return nil
	})
	if err != nil {
		fmt.Fprint(os.Stderr, "during DB View: %s\n", err)
		w.WriteHeader(500)
	}
	fmt.Printf("Response: %+v\n", response)
	resp, err := json.Marshal(response)
	if err != nil {
		fmt.Fprint(os.Stderr, "during JSON marshall: %s\n", err)
		w.WriteHeader(500)
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
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
				s.countMessage(ev)

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

func toHour(epoc string) string {
	float, err := strconv.ParseFloat(epoc, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "timestamp conversion for '%s' - %s", epoc, err)
	}
	timestamp := time.Unix(0, int64(float*1000000)*int64(time.Microsecond/time.Nanosecond)).UTC()
	return timestamp.Format("2006-01-02T15")
}

func dataPoint(items ...string) []byte {
	return []byte(strings.Join(items, "/"))
}

func (s *SlackBot) countMessage(ev *slack.MessageEvent) {
	// get the hour from message timestamp
	hour := toHour(ev.Timestamp)

	// Start a badger transaction
	s.db.Update(func(txn *badger.Txn) error {
		err := increment(txn, dataPoint(hour, ev.User, "messages", ev.Channel))
		if err != nil {
			fmt.Fprint(os.Stderr, "during 'messages' increment: %s\n", err)
		}

		result := hc.Analyze(ev.Text)
		fmt.Printf("Result: %+v\n", result)
		if result.Score > 0 {
			err = increment(txn, dataPoint(hour, ev.User, "positive", ev.Channel))
			if err != nil {
				fmt.Fprint(os.Stderr, "during 'messages' increment: %s\n", err)
			}
		}
		if result.Score < 0 {
			increment(txn, dataPoint(hour, ev.User, "negative", ev.Channel))
			if err != nil {
				fmt.Fprint(os.Stderr, "during 'messages' increment: %s\n", err)
			}
		}
		return nil
	})
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
		if err != nil {
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
