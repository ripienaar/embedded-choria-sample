package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/choria-io/go-choria/choria"
	"github.com/choria-io/go-choria/server"
	"github.com/sirupsen/logrus"
)

var cserver *server.Instance
var cfg *choria.Config
var fw *choria.Framework

// flag the agent manages and other parts should check
var allowWork = true

// place holder for a loop you wish to do your work in, this has a basic
// circuit breaker that we will later manage at run time using a Choria agent
func doWork(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	for {
		select {
		case <-time.Tick(time.Duration(5) * time.Second):
			if allowWork {
				logrus.Infof("doing work....")
			} else {
				logrus.Infof("work cannot be done due to circuit breaker")
			}
		case <-ctx.Done():
			return
		}
	}
}

func main() {
	var err error

	ctx, cancel := context.WithCancel(context.Background())
	wg := &sync.WaitGroup{}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	cserver, err = setupChoria()
	if err != nil {
		logrus.Errorf("Choria could not be setup: %s", err)
		os.Exit(1)
	}

	wg.Add(1)
	go runChoria(ctx, wg)

	wg.Add(1)
	go doWork(ctx, wg)

	for {
		select {
		case sig := <-sigs:
			fmt.Printf("Shutting down on %s", sig)
			cancel()
		case <-ctx.Done():
			// waits for choria to cleanly shut down
			wg.Wait()
			cancel()
			return
		}
	}
}
