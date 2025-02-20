package action

import (
	"context"
	"io"

	"capact.io/capact/internal/cli/client"
	"capact.io/capact/internal/cli/config"
	"capact.io/capact/internal/k8s-engine/graphql/namespace"
	"capact.io/capact/internal/ptr"
	gqlengine "capact.io/capact/pkg/engine/api/graphql"

	"github.com/fatih/color"
)

// CreateOutput defines output for Create function.
type CreateOutput struct {
	Action    *gqlengine.Action
	Namespace string
}

// Create creates a given Action.
func Create(ctx context.Context, opts CreateOptions, w io.Writer) (*CreateOutput, error) {
	if err := opts.resolve(); err != nil {
		return nil, err
	}

	server := config.GetDefaultContext()

	actionCli, err := client.NewCluster(server)
	if err != nil {
		return nil, err
	}

	ctxWithNs := namespace.NewContext(ctx, opts.Namespace)
	act, err := actionCli.CreateAction(ctxWithNs, &gqlengine.ActionDetailsInput{
		Name:  opts.ActionName,
		Input: opts.ActionInput(),
		ActionRef: &gqlengine.ManifestReferenceInput{
			Path: opts.InterfacePath,
		},
		DryRun: ptr.Bool(opts.DryRun),
	})
	if err != nil {
		return nil, err
	}

	okCheck := color.New(color.FgGreen).FprintfFunc()
	okCheck(w, "Action %s/%s created successfully\n", opts.Namespace, act.Name)

	return &CreateOutput{
		Action:    act,
		Namespace: opts.Namespace,
	}, nil
}
