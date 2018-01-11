# Embedding a Choria Server into another Go application

One of the, if not the very first, desires with MCollective has been to make an embeddable Ruby server into other Ruby apps where one could perform admin tasks and adjust their run time behaviour.  This never materialised in Ruby mainly due to shifting goals but also to a large extend the nature of the Ruby runtime.

With the Go based Choria Server embedding is viable, this article explores this feature.

TODO:

  * Show provisioning

## Why?

### Server process live-administration
A good example of the problem embedding solves is in the [stream-replicator](https://github.com/choria-io/stream-replicator) project, when deployed in a large network one will end up with very many of these things running.

Lets assume they mostly replicate upwards to a central point, maintenance, debugging or just basic survival of the central might demand run time changes to the replicators such as adjusting intervals or pausing/resuming message replication.

Lets assume we have 50 of these Stream Replicators spread across 30 data centres, just knowing where they are, their management ports for interaction via REST etc can be a real bourdon.

But these are exactly problems that the discovery model solve - all the replicators in `DC1`, all the replicators for topic `cmdb` and so forth - and the Choria model have strong security, auditing and authorization as a first class feature.

So for the Stream Replicator I plan to embed a Choria server.  When enabled it will connect to a specific Choria Broker cluster and live inside a dedicated sub-collective.  There it will expose discovery data to let people identify them and it will have at least actions to pause and resume its processing of messages.

## Custom life-cycles and embedded needs like IoT

Another area where this can be very useful is in the IoT world or truly custom automation systems.  Imagine I have many RaspberryPI with sensors spread over a large area in many locations and I want to gather those metrics as a stream and on an ad-hoc basis.

One might want a data source that publishes metrics regularly into the middleware and a interactive/API based method for on-demand querying of a specific device.  One can also have general device life cycle management steps like joining a AWS IoT network or provisioning API keys and SSL certs.

This is all functions the agent system in Choria would be well suited for.  The initial provisioning is also catered for by the provisioning feature that can be enabled in custom builds of Choria.  Provisioning allows your device to join a special collective exposing just a basic agent capable of managing the configuration of the service and then reload it into it's final setup.

Together these features create the ability to construct a flexible, scalable, distributed IoT network that's secure and fit for ones specific needs.

A small project that embeds Choria in this manner is my example [choriapi](https://github.com/ripienaar/choriapi) project, it does not yet use the provisioning features though.

# Example

I will walk through creating a small custom binary that embeds a Choria server while performing some work in a loop.  The idea is that the work loop - here just printing to STDOUt - is the main function this app is supposed to perform.

In order to manage a Circuit Breaker in it and extract data from it's internals it utilize Choria Server as a library to run custom management agent, settings, data exfil etc all secured by the Choria Protocol.

The example will publish data about it's internals regularly to the network secured using the Choria protocol so it's tamper proof and you can verify the publisher really did publish it.

You will be able to receive this data using a Choria Broker and using it's Protocol Adapters republish that data into a NATS Streaming Server topic where you can process the data using Stream Processing tools.

You will be able to interact with it - stop and resume processing - via a circuit breaker using `mco rpc circuit_breaker switch` CLI.

It'll be discoverable and you can have custom facts about the specific instance to help you find them in the group of similar running instances of this.

It will run in a custom sub-collective where you can put all these same processes if you wish.

## Embedding a server

Let's look at doing the basic server embedding, starting, stopping etc.  Right now there will be no custom agents, discovery, facts etc. those will follow later

You'd perhaps have your own configuration file or CLI options, but lets assume there's no Choria compatible configuration, we have to make a config and instance of the server.

This program we are embedding into will do some processing of some form, here's a little placeholder function to show it doing regular work, it checks the `suspendProcessing` variable as a circuit breaker:

```go
package main

import (
	// and others
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
```

Here's the function that creates a Choria configuration and prepare an instance of the Choria Server

```go
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
```

Next we'll want to create a method to run the server, this will start it up, do initial connection, start the core agents and get ready to process requests.

The server uses contexts and waitgroups to handle a orderly shutdown:

```go
func runChoria(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// starts the server, initial connects will also be interrupted by the context etc
	wg.Add(1)
	cserver.Run(ctx, wg)
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
```

## Exposing facts

You have the basic server up and running, lets look at adding some facts so discovery can work.  We'll do this in a package called `facts`.  Since Choria only allows reading facts from a file we'll setup a temp file and write into that regularly.

I am leaving out the detail of what goes in the facts, the structure just needs to be something that can be used by `json.Marshal`

```go
package facts

import (
	// not shown
)

var f *os.File
var conf *choria.Config
var err error

type Factdata struct {
	Identity string `json:"identity"`
	Time     int64  `json:"time"`
}

func Expose(ctx context.Context, wg *sync.WaitGroup, cfg *choria.Config) {
	defer wg.Done()

	conf = cfg

	f, err = ioutil.TempFile("", "acmefacts")
	if err != nil {
		logrus.Fatalf("Could not create fact temp file: %s", err)
	}
	defer os.Remove(f.Name())

	// we will do atomic write to this file via renames, so not directly to it
	f.Close()

	// discovery will find this file now and report on its contents as json format
	cfg.Choria.FactSourceFile = f.Name()

	writer := func() {
		err := write()
		if err != nil {
			logrus.Warnf("Could not write fact data: %s", err)
		}
	}

	logrus.Infof("Writing fact data to %s", f.Name())

	writer()

	for {
		select {
		case <-time.Tick(time.Duration(60) * time.Second):
			writer()
		case <-ctx.Done():
			return
		}
	}
}

// Data returns current set of Factdata
func Data() Factdata {
	return Factdata{conf.Identity, time.Now().Unix()}
}

func write() error {
	tf, err := ioutil.TempFile("", "")
	if err != nil {
		return err
	}
	defer os.Remove(tf.Name())

	tf.Close()

	j, err := json.Marshal(Data())
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(tf.Name(), j, 0644)
	if err != nil {
		return err
	}

	err = os.Rename(tf.Name(), f.Name())
	if err != nil {
		return err
	}

	return nil
}
```

With this package we need to make a small change to our earlier `runChoria()` function:

```go
func runChoria(ctx context.Context, wg *sync.WaitGroup) {
	defer wg.Done()

	// start publishing facts to a temp file
	wg.Add(1)
	go facts.Expose(ctx, wg, cfg)

	// starts the server, initial connects will also be interrupted by the context etc
	wg.Add(1)
	cserver.Run(ctx, wg)
}
```

## Create an agent

We can create a custom agent so users can interact with us from the outside, here I'll just have one that flips a flag for some kind of circuit breaker for example.

```go
// reply structure for the RPC agent
type switchReply struct {
	Message string `json:"message"`
	State   bool   `json:"state"`
}

func registerAgent(ctx context.Context) error {
	// mcollective like metadata
	m := &agents.Metadata{
		Name:        "circuit_breaker",
		Description: "Circuit Breaker",
		Author:      "R.I.Pienaar <rip@devco.net>",
		Version:     "0.0.1",
		License:     "Apache-2.0",
		Timeout:     1,
		URL:         "http://choria.io",
	}

	agent := mcorpc.New("circuit_breaker", m, fw, logrus.WithFields(logrus.Fields{"agent": "circuit_breaker"}))

	err := agent.RegisterAction("switch", switchAction)
	if err != nil {
		return err
	}

	// adds the agent to the running instance of the server
	// this has to happen after its initial connect
	return cserver.RegisterAgent(ctx, "circuit_breaker", agent)
}

func switchAction(req *mcorpc.Request, reply *mcorpc.Reply, agent *mcorpc.Agent, conn choria.ConnectorInfo) {
	allowWork = !allowWork

	agent.Log.Infof("Switches the circuit breaker, allowWork = %t", allowWork)

	reply.Data = &switchReply{fmt.Sprintf("Switched the circuit breaker, allowWork = %t", allowWork), allowWork}
}
```

This is the most super basic agent, lets modify `runChoria()` again to register this:

```go
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
}
```

## Publish internal data regularly

Finally we can utilise registration to publish internals of our process regularly, metrics or facts, whatever I'll leave the detail of what is being published out.

The published messages will be wrapped in Choria protocol bits - signed, tamper proof, authentication assured by certname and so forth.

It goes to `acme.data` on the middleware and you can use a Choria Adapter to publish that into NATS Stream for processing using Stream Processing semantics.

```go
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
```

Again we modify the `runChoria()` to register this:

```go
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
```

## Running
When I run this final product it looks like this:

```
INFO[0000] Initial servers: []choria.Server{choria.Server{Host:"demo.nats.io", Port:4222, Scheme:"nats"}}  component=server identity=dust.local
INFO[0000] Writing fact data to /var/folders/_c/t9z4bgvj7yz77d94rkcxkz5h0000gn/T/acmefacts800973235
INFO[0000] Connected to nats://demo.nats.io:4222         component=server identity=dust.local
INFO[0000] Registering new agent discovery of type discovery  component=server identity=dust.local subsystem=agents
INFO[0000] Subscribing agent discovery to acme.broadcast.agent.discovery  component=server identity=dust.local subsystem=agents
DEBU[0000] Susbscribing to acme.broadcast.agent.discovery in group '' on server nats://demo.nats.io:4222  component=server identity=dust.local
INFO[0000] Registering new agent choria_util of type choria_util  component=server identity=dust.local subsystem=agents
INFO[0000] Subscribing agent choria_util to acme.broadcast.agent.choria_util  component=server identity=dust.local subsystem=agents
DEBU[0000] Susbscribing to acme.broadcast.agent.choria_util in group '' on server nats://demo.nats.io:4222  component=server identity=dust.local
INFO[0000] Subscribing node dust.local to acme.node.dust.local  component=server identity=dust.local
DEBU[0000] Susbscribing to acme.node.dust.local in group '' on server nats://demo.nats.io:4222  component=server identity=dust.local
INFO[0000] Registering new agent circuit_breaker of type circuit_breaker  component=server identity=dust.local subsystem=agents
INFO[0000] Subscribing agent circuit_breaker to acme.broadcast.agent.circuit_breaker  component=server identity=dust.local subsystem=agents
DEBU[0000] Susbscribing to acme.broadcast.agent.circuit_breaker in group '' on server nats://demo.nats.io:4222  component=server identity=dust.local
DEBU[0000] Sending a broadcast message to NATS target 'acme.data' for message 4b44d5119171498781203b91f2a4f0d0 type request
DEBU[0000] Publishing 4399 bytes to acme.data
INFO[0005] doing work....
INFO[0010] doing work....
INFO[0015] doing work....
INFO[0020] doing work....
INFO[0025] doing work....
INFO[0030] doing work....
INFO[0035] doing work....
INFO[0040] doing work....
INFO[0045] doing work....
INFO[0050] doing work....
INFO[0055] doing work....
INFO[0060] doing work....
DEBU[0060] Sending a broadcast message to NATS target 'acme.data' for message 3940faf3c9a0411b957ad51a409ad873 type request
DEBU[0060] Publishing 4399 bytes to acme.data
INFO[0065] doing work....
```

You can see all the agents registering, facts publishing starting, registration starting and finally our work loop running, lets switch it:

```
% ./mco rpc circuit_breaker switch --dm=mc --target acme
Discovering hosts using the mc method for 2 second(s) .... 1

 * [ ============================================================> ] 1 / 1


dust.local
   message: Switched the circuit breaker, allowWork = false
     state: false



Finished processing 1 / 1 hosts in 460.93 ms
```

And the resulting log:

```
DEBU[0098] Secure Request signature verified using /Users/rip/.puppetlabs/etc/puppet/ssl/choria_secuirty/public_certs/rip.mcollective.pem
DEBU[0098] Matching request 82f44d6fd5205da4bba07f8b7f1b5f11 with agent filters '[]string{"circuit_breaker"}'  component=server identity=dust.local subsystem=discovery
DEBU[0098] Handling message 82f44d6fd5205da4bba07f8b7f1b5f11 with timeout 2000000000  component=server identity=dust.local subsystem=agents
DEBU[0098] Sending a broadcast message to NATS target 'acme.reply.dev1.devco.net.25473.1' for message 82f44d6fd5205da4bba07f8b7f1b5f11 type reply
DEBU[0098] Publishing 557 bytes to acme.reply.dev1.devco.net.25473.1
INFO[0100] doing work....
DEBU[0100] Secure Request signature verified using /Users/rip/.puppetlabs/etc/puppet/ssl/choria_secuirty/public_certs/rip.mcollective.pem
DEBU[0100] Matching request 4e3b281b048058bd9ef39a5e4dfae613 with agent filters '[]string{"circuit_breaker"}'  component=server identity=dust.local subsystem=discovery
DEBU[0100] Handling message 4e3b281b048058bd9ef39a5e4dfae613 with timeout 1000000000  component=server identity=dust.local subsystem=agents
INFO[0100] Handling message 4e3b281b048058bd9ef39a5e4dfae613 for circuit_breaker#switch from choria=rip.mcollective  agent=circuit_breaker
INFO[0100] Switches the circuit breaker, allowWork = false  agent=circuit_breaker
DEBU[0100] Sending a broadcast message to NATS target 'acme.reply.dev1.devco.net.25473.2' for message 4e3b281b048058bd9ef39a5e4dfae613 type reply
DEBU[0100] Publishing 769 bytes to acme.reply.dev1.devco.net.25473.2

INFO[0105] work cannot be done due to circuit breaker
```

Switching it again will result in work being done :)

# Conclusion
The outcome is a custom binary that performs the work you intend to do - and this is optional you could just use this mechanism to create an entirely custom Choria Server.

The server has a custom agent compiled into it, custom registration and facts and can be fully interacted with from the outside using the traditional ruby Choria.

You'll have to make a DDL file for the ruby side to speak to it, I did not show that, but with that in place you could do `mco rpc circuit_breaker switch --target acme` and it will suspend or resume processing of your worker loop.
