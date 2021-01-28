package main

import (
	"fmt"
	"log"
	"os"

	"github.com/newrelic/go-agent"
	"github.com/usmbest/ocean.one/example/config"
)

func setupNewRelic(service string) newrelic.Application {
	hostname, _ := os.Hostname()
	appName := fmt.Sprintf("%s - %s - %s", config.Name, config.Environment, service)
	newRelic := newrelic.NewConfig(appName, config.NewRelicAPIKey)
	newRelic.Enabled = config.NewRelicEnabled
	newRelic.HostDisplayName = hostname
	app, err := newrelic.NewApplication(newRelic)
	if err != nil {
		log.Panicln(err)
	}
	return app
}
