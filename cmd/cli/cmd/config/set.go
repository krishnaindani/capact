package config

import (
	"fmt"

	"capact.io/capact/internal/cli"
	"capact.io/capact/internal/cli/config"
	"capact.io/capact/internal/cli/credstore"
	"capact.io/capact/internal/cli/heredoc"

	"github.com/AlecAivazis/survey/v2"
	"github.com/spf13/cobra"
)

type setContextOptions struct {
	serverAddress string
}

// NewSet returns a cobra.Command for setting an active CLI context.
func NewSet() *cobra.Command {
	var opts setContextOptions

	return &cobra.Command{
		Use:   "set-context",
		Short: "Updates the active hub configuration context",
		Example: heredoc.WithCLIName(`
			# Selects which Hub/Gateway server to use of via a prompt
			<cli> config set-context
			
			# Sets the specified Hub/Gateway server
			<cli> config set-context localhost:8080
		`, cli.Name),
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.serverAddress = args[0]
			}
			return setRun(opts)
		},
	}
}

func setRun(opts setContextOptions) error {
	if opts.serverAddress == "" {
		answer, err := askWhatServerToSet()
		if err != nil {
			return err
		}
		opts.serverAddress = answer
	}

	return config.SetAsDefaultContext(opts.serverAddress, true)
}

func askWhatServerToSet() (string, error) {
	candidates, err := credstore.ListHubServer()
	if err != nil {
		return "", err
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("Not logged in to any server")
	}

	var serverAddress string
	err = survey.AskOne(&survey.Select{
		Message: "Which Hub server do you want to set as the default? ",
		Options: candidates,
	}, &serverAddress)
	if err != nil {
		return "", err
	}

	return serverAddress, nil
}
