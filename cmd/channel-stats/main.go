package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/thrawn01/channel-stats"
)

func checkErr(err error) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "-- %s\n", err)
		os.Exit(1)
	}
}

func main() {

	// Load config
	conf, err := channelstats.LoadConfig()
	checkErr(err)

	// Initialize our logging config
	channelstats.InitLogging(conf)

	// Can notify an operator of events
	notify, err := channelstats.NewNotifier(conf)
	checkErr(err)

	checkErr(notify.Operator(fmt.Sprintf("[channel-stats] Started @ %s", time.Now())))

	// The ID manager keeps track of channel and user ids
	idMgr, err := channelstats.NewIdManager(conf)
	checkErr(err)

	// Initialize the badger data store
	store, err := channelstats.NewStore(conf, idMgr)
	checkErr(err)
	defer store.Close()

	// Start the slack bot
	bot := channelstats.NewSlackBot(conf, store, idMgr, notify)

	// Start the http server
	server := channelstats.NewServer(store, idMgr)

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			if sig == os.Interrupt {
				fmt.Printf("caught interupt; user requested premature exit\n")
				// Stop http handlers
				server.Stop()
				// Stop the bot
				bot.Stop()
			}
		}
	}()

	if err := bot.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "-- %s\n", err)
		os.Exit(1)
	}
}
