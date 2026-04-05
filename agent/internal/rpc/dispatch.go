package rpc

import (
	"fmt"
	"log/slog"

	agentv1 "github.com/salicloud/gratis/gen/agent/v1"
	"github.com/salicloud/gratis/agent/internal/webserver"
)

// dispatch routes a command to the appropriate handler and returns the result.
func dispatch(cmd *agentv1.Command) *agentv1.CommandResult {
	var err error

	switch p := cmd.Payload.(type) {
	case *agentv1.Command_CreateVhost:
		err = handleCreateVhost(p.CreateVhost)
	case *agentv1.Command_DeleteVhost:
		err = handleDeleteVhost(p.DeleteVhost)
	default:
		err = fmt.Errorf("unhandled command type %T", cmd.Payload)
		slog.Warn("unhandled command", "type", fmt.Sprintf("%T", cmd.Payload))
	}

	if err != nil {
		return &agentv1.CommandResult{
			CommandId: cmd.CommandId,
			Success:   false,
			Error:     err.Error(),
		}
	}
	return &agentv1.CommandResult{
		CommandId: cmd.CommandId,
		Success:   true,
	}
}

func handleCreateVhost(cmd *agentv1.CreateVhostCmd) error {
	return webserver.CreateVhost(webserver.VhostConfig{
		Domain:     cmd.Domain,
		Aliases:    cmd.Aliases,
		Docroot:    cmd.Docroot,
		PHPVersion: cmd.PhpVersion,
	})
}

func handleDeleteVhost(cmd *agentv1.DeleteVhostCmd) error {
	return webserver.DeleteVhost(cmd.Domain)
}
