package main

import (
	"fmt"
	"os"
	"os/signal"

	"github.com/thrawn01/channel-stats"
)

func main() {

	store, err := channelstats.NewStore()
	if err != nil {
		fmt.Fprintf(os.Stderr, "-- %s\n", err)
		os.Exit(1)
	}
	// Close badger so we don't have LOCK errors
	defer store.Close()

	// Start the slackbot
	bot := channelstats.NewSlackBot(store)

	// Start the http server
	server := channelstats.NewServer(store)

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
	fmt.Println("-- done")
}
