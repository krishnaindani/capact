package argo

import (
	"context"
	"fmt"

	"capact.io/capact/pkg/runner"

	wfv1 "github.com/argoproj/argo/v2/pkg/apis/workflow/v1alpha1"
	wfclientset "github.com/argoproj/argo/v2/pkg/client/clientset/versioned"
	"github.com/argoproj/argo/v2/workflow/util"
	"github.com/pkg/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
	watchtools "k8s.io/client-go/tools/watch"
	"sigs.k8s.io/yaml"
)

const (
	wfManagedByLabelKey = "runner.capact.io/created-by"
	runnerName          = "argo-runner"
)

type (
	// Status provides info to easily identify started Argo Workflow.
	Status struct {
		ArgoWorkflowRef WorkflowRef `json:"argoWorkflowRef"`
	}
	// WorkflowRef represents a Argo Workflow CR.
	WorkflowRef struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	}
)

var _ runner.Runner = &Runner{}

// Runner provides functionality to run and wait for Argo Workflow.
type Runner struct {
	wfClientset wfclientset.Interface
}

// NewRunner returns new instance of Argo Runner.
func NewRunner(wfClientset wfclientset.Interface) *Runner {
	return &Runner{wfClientset: wfClientset}
}

// Start the Argo Workflow from the given manifest.
func (r *Runner) Start(ctx context.Context, in runner.StartInput) (*runner.StartOutput, error) {
	var renderedWorkflow = struct {
		Spec wfv1.WorkflowSpec `json:"workflow"`
	}{}

	if err := yaml.Unmarshal(in.Args, &renderedWorkflow); err != nil {
		return nil, errors.Wrap(err, "while unmarshaling workflow spec")
	}

	wf := wfv1.Workflow{
		ObjectMeta: metav1.ObjectMeta{
			Name:      in.RunnerCtx.Name,
			Namespace: in.RunnerCtx.Platform.Namespace,
			Labels: map[string]string{
				wfManagedByLabelKey: runnerName,
			},
			OwnerReferences: []metav1.OwnerReference{
				in.RunnerCtx.Platform.OwnerRef,
			},
		},
		Spec: renderedWorkflow.Spec,
	}

	// We have agreement that we should return error also if workflow already exits.
	if err := r.submitWorkflow(&wf, in.RunnerCtx); err != nil {
		return nil, errors.Wrap(err, "while creating Argo Workflow")
	}

	return &runner.StartOutput{
		Status: Status{
			ArgoWorkflowRef: WorkflowRef{
				Name:      wf.Name,
				Namespace: wf.Namespace,
			},
		},
	}, nil
}

// WaitForCompletion waits until Argo Workflow is finished.
func (r *Runner) WaitForCompletion(ctx context.Context, in runner.WaitForCompletionInput) (*runner.WaitForCompletionOutput, error) {
	if in.RunnerCtx.DryRun {
		return &runner.WaitForCompletionOutput{
			Succeeded: true,
			Message:   "In DryRun mode Argo Workflow is not created.",
		}, nil
	}

	fieldSelector := fields.OneTermEqualSelector("metadata.name", in.RunnerCtx.Name).String()
	lw := &cache.ListWatch{
		ListFunc: func(opts metav1.ListOptions) (runtime.Object, error) {
			opts.FieldSelector = fieldSelector
			return r.wfClientset.ArgoprojV1alpha1().Workflows(in.RunnerCtx.Platform.Namespace).List(opts)
		},
		WatchFunc: func(opts metav1.ListOptions) (watch.Interface, error) {
			opts.FieldSelector = fieldSelector
			return r.wfClientset.ArgoprojV1alpha1().Workflows(in.RunnerCtx.Platform.Namespace).Watch(opts)
		},
	}

	workflowCompleted := func(event watch.Event) (bool, error) {
		switch event.Type {
		case watch.Modified, watch.Added:
		case watch.Deleted:
			// We need to abort to avoid cases of recreation and not to silently watch the wrong (new) object
			return false, fmt.Errorf("workflow was deleted")
		default:
			return false, nil
		}

		status, _ := statusFromEvent(&event)
		if !status.FinishedAt.IsZero() {
			return true, nil
		}

		return false, nil
	}

	lastEvent, err := watchtools.UntilWithSync(ctx, lw, &wfv1.Workflow{}, nil, workflowCompleted)
	if err != nil {
		return nil, err
	}

	status, err := statusFromEvent(lastEvent)
	if err != nil {
		return nil, err
	}

	return &runner.WaitForCompletionOutput{
		Succeeded: status.Phase == wfv1.NodeSucceeded,
		Message:   status.Message,
	}, nil
}

func statusFromEvent(event *watch.Event) (wfv1.WorkflowStatus, error) {
	if event == nil {
		return wfv1.WorkflowStatus{}, errors.New("got nil event")
	}
	switch obj := event.Object.(type) {
	case *wfv1.Workflow:
		return obj.Status, nil
	default:
		return wfv1.WorkflowStatus{}, errors.Errorf("Wrong event object, want *wfv1.Workflow got %T", obj)
	}
}

func (r *Runner) submitWorkflow(wf *wfv1.Workflow, runnerCtx runner.Context) error {
	wfNSCli := r.wfClientset.ArgoprojV1alpha1().Workflows(runnerCtx.Platform.Namespace)
	_, err := util.SubmitWorkflow(wfNSCli, r.wfClientset, runnerCtx.Platform.Namespace, wf, &wfv1.SubmitOpts{
		ServiceAccount: runnerCtx.Platform.ServiceAccountName,
		ServerDryRun:   runnerCtx.DryRun,
	})
	return err
}

// Name returns runner name.
func (r *Runner) Name() string {
	return "argo.workflow"
}
