package main

import (
	"context"
	"sync"
	"time"

	"github.com/choria-io/go-choria/server/data"
)

type registration struct{}

func (r *registration) StartRegistration(ctx context.Context, wg *sync.WaitGroup, interval int, output chan *data.RegistrationItem) {
	defer wg.Done()

	r.publish(output)

	for {
		select {
		case <-time.Tick(time.Duration(interval) * time.Second):
			r.publish(output)
		case <-ctx.Done():
			return
		}
	}
}

func (r *registration) publish(output chan *data.RegistrationItem) {
	// jdat here is a variable with JSON data that you wish to publish, not shown how to make that
	// its up to your code and needs, just make a struct and marshal it
	jdat := []byte(`{"hello":"world"}`)

	item := &data.RegistrationItem{
		Data:        &jdat,
		Destination: "acme.data",
	}

	output <- item
}
