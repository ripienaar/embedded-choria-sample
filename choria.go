package main

import (
	"context"
	"sync"

	"github.com/choria-io/go-choria/choria"
	"github.com/choria-io/go-choria/server"
	"github.com/ripienaar/embedded-choria-sample/facts"
	"github.com/sirupsen/logrus"
)

// configures and sets up a Choria server instance
func setupChoria() (*server.Instance, error) {
	var err error

	cfg, err = choria.NewConfig("/dev/null")
	if err != nil {
		return nil, err
	}

	cfg.Choria.MiddlewareHosts = []string{"demo.nats.io:4222"}

	// how often to publish data
	cfg.RegisterInterval = 60

	// basic setup thats needed because there is no config at all
	cfg.LogLevel = "debug"
	cfg.MainCollective = "acme"
	cfg.Collectives = []string{"acme"}
	cfg.RegistrationCollective = "acme"

	// disable TLS so this works on the plain text demo.nats.io
	cfg.DisableTLS = true

	fw, err = choria.NewWithConfig(cfg)
	if err != nil {
		return nil, err
	}

	return server.NewInstance(fw)
}

func runChoria(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// start publishing facts to a temp file
	wg.Add(1)
	go facts.Expose(ctx, wg, cfg)

	// starts the server, initial connects will also be interrupted by the context etc
	wg.Add(1)
	cserver.Run(ctx, wg)

	// register agent this has to happen only after the server instance is up and running
	// and had its initial connect done
	err := registerAgent(ctx)
	if err != nil {
		logrus.Fatalf("Could not register choria agent: %s", err)
	}

	// register our registration provider, has to be done after Run()
	err = cserver.RegisterRegistrationProvider(ctx, wg, &registration{})
	if err != nil {
		logrus.Fatalf("Could not register registration provider: %s", err)
	}
}
