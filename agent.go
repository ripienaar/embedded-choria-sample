package main

import (
	"context"
	"fmt"

	"github.com/choria-io/go-choria/choria"
	"github.com/choria-io/go-choria/mcorpc"
	"github.com/choria-io/go-choria/server/agents"
	"github.com/sirupsen/logrus"
)

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
