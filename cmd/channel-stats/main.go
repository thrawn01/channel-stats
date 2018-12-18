package main

import (
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/thrawn01/channel-stats"
)

var Version = "dev-build"

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

	channelstats.GetLogger().Infof("Starting Version: %s", Version)

	// Can mailer an operator of events
	mail, err := channelstats.NewMailer(conf)
	checkErr(err)

	checkErr(mail.Operator(fmt.Sprintf("[channel-stats] Started @ %s", time.Now())))

	// The ID manager keeps track of channel and user ids
	idMgr, err := channelstats.NewIdManager(conf)
	checkErr(err)

	/*idMgr := &channelstats.MockIDManage{
		UserByID: map[string]string{
			"U02C11FN4": "Joe",
			"U02C6CMDP": "Scott",
			"U02C7322G": "Thrawn",
			"U02C73W94": "Admiral",
			"U02CG0QLN": "Cat",
			"U02DJSNKB": "Dog",
			"U8A9VQ22W": "Kitty",
			"U8MQTQUAX": "Person",
			"U8M58VCSE": "Doug",
			"U02C7788J": "Doug Jr",
			"U02C073N7": "Me",
		},
	}*/

	// Initialize the badger data store
	store, err := channelstats.NewStore(conf, idMgr)
	checkErr(err)
	defer store.Close()

	// Generates reports for channels and emails them to users
	reporter, err := channelstats.NewReporter(conf, idMgr, mail, store)
	checkErr(err)

	// Start the slack bot
	bot := channelstats.NewSlackBot(conf, store, idMgr, mail)

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
				// Stop the reporter
				reporter.Stop()
				//os.Exit(1)
			}
		}
	}()

	checkErr(bot.Start())
}
