package client

import (
	govclient "github.com/cosmos/cosmos-sdk/x/gov/client"
	"github.com/petri-labs/warmage/x/oracle/client/cli"
	"github.com/petri-labs/warmage/x/oracle/client/rest"
)

var (
	RegisterTargetProposalHandler = govclient.NewProposalHandler(cli.NewRegisterTargetProposalCmd, rest.RegisterTargetProposalRESTHandler)
)
