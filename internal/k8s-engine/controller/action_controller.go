package controller

import (
	"context"
	"fmt"

	"projectvoltron.dev/voltron/internal/ptr"
	"projectvoltron.dev/voltron/pkg/engine/k8s/api/v1alpha1"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
)

// ActionReconciler reconciles a Action object.
type ActionReconciler struct {
	k8sCli      client.Client
	log         logr.Logger
	svc         actionService
	recorder    record.EventRecorder
	rateLimiter workqueue.RateLimiter
	maxRetries  int
}

type (
	actionStarter interface {
		EnsureWorkflowSAExists(ctx context.Context, action *v1alpha1.Action) (*corev1.ServiceAccount, error)
		EnsureRunnerInputDataCreated(ctx context.Context, saName string, action *v1alpha1.Action) error
		EnsureRunnerExecuted(ctx context.Context, saName string, action *v1alpha1.Action) error
	}
	actionRenderer interface {
		RenderAction(ctx context.Context, action *v1alpha1.Action) (*v1alpha1.RenderingStatus, error)
	}
	actionStatusGetter interface {
		GetReportedRunnerStatus(ctx context.Context, action *v1alpha1.Action) (*GetReportedRunnerStatusOutput, error)
		GetRunnerJobStatus(ctx context.Context, action *v1alpha1.Action) (*GetRunnerJobStatusOutput, error)
	}
	actionService interface {
		actionRenderer
		actionStarter
		actionStatusGetter
	}
)

// NewActionReconciler returns the ActionReconciler instance.
func NewActionReconciler(log logr.Logger, svc actionService, maxRetriesForAction int) *ActionReconciler {
	return &ActionReconciler{
		log:        log.WithName("controllers").WithName("Action"),
		svc:        svc,
		maxRetries: maxRetriesForAction,
	}
}

// +kubebuilder:rbac:groups=core.projectvoltron.dev,resources=actions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.projectvoltron.dev,resources=actions/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch;create
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=events,verbs=get;list;watch;create;update;patch;delete

// Reconcile handles the reconcile logic for the Action CR.
// TODO: introduce and ignore permanent error in reconcile loop
func (r *ActionReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var (
		ctx = context.Background()
		log = r.log.WithValues("action", req.NamespacedName)
	)

	action := &v1alpha1.Action{}
	if err := r.k8sCli.Get(ctx, req.NamespacedName, action); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		log.Error(err, "while fetching Action CR")
		return ctrl.Result{}, err
	}

	reportOnError := func(err error, context string) (ctrl.Result, error) {
		r.recorder.Event(action, corev1.EventTypeWarning, context, err.Error())
		return ctrl.Result{}, err
	}

	if action.IsBeingDeleted() {
		// TODO: currently cannot reach this state.
		// Add finalizer and handle deletion properly (cancel running actions, remove ArgoWorkflows)
		return ctrl.Result{}, nil
	}

	if action.IsUninitialized() {
		log.Info("Initializing runner action")
		result, err := r.initAction(ctx, action)
		if err != nil {
			return reportOnError(err, "Init runner action")
		}
		return result, nil
	}

	if action.IsBeingRendered() {
		log.Info("Rendering runner action")
		result, err := r.renderAction(ctx, action)
		if err != nil {
			return reportOnError(err, "Render runner action")
		}
		return result, nil
	}

	if action.IsReadyToExecute() {
		log.Info("Execute runner")
		result, err := r.executeAction(ctx, action)
		if err != nil {
			return reportOnError(err, "Execute runner")
		}
		return result, nil
	}

	if action.IsExecuted() {
		log.Info("Check executed runner")
		result, err := r.handleRunningAction(ctx, action)
		if err != nil {
			return reportOnError(err, "Check executed runner")
		}
		return result, nil
	}

	return ctrl.Result{}, nil
}

func (r *ActionReconciler) initAction(ctx context.Context, action *v1alpha1.Action) (ctrl.Result, error) {
	action.Status = r.successStatus(action, v1alpha1.BeingRenderedActionPhase, "Rendering runner action")
	if err := r.k8sCli.Status().Update(ctx, action); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "while updating action object status")
	}
	// requeue to start the rendering process
	return ctrl.Result{Requeue: true}, nil
}

// renderAction renders a given action. If finally rendered, sets status to v1alpha1.ReadyToRunActionPhase phase.
//
// TODO: add support for v1alpha1.AdvancedModeRenderingIterationActionPhase phase
func (r *ActionReconciler) renderAction(ctx context.Context, action *v1alpha1.Action) (ctrl.Result, error) {
	renderingStatus, err := r.svc.RenderAction(ctx, action)
	if err != nil {
		msg := fmt.Sprintf("Cannot render given action: %s", err)
		return r.handleRetry(ctx, action, v1alpha1.BeingRenderedActionPhase, msg)
	}

	action.Status.Rendering = renderingStatus

	action.Status = r.successStatus(action, v1alpha1.ReadyToRunActionPhase, "Runner action is rendered and ready to be executed")
	if err := r.k8sCli.Status().Update(ctx, action); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "while updating action object status")
	}

	// requeue to check if runner should be executed
	return ctrl.Result{Requeue: true}, nil
}

// executeAction executes action (run, dryRun, cancel etc) and set v1alpha1.RunningActionPhase.
//
// TODO: add support v1alpha1.BeingCanceledActionPhase phase.
func (r *ActionReconciler) executeAction(ctx context.Context, action *v1alpha1.Action) (ctrl.Result, error) {
	sa, err := r.svc.EnsureWorkflowSAExists(ctx, action)
	if err != nil {
		msg := fmt.Sprintf("Cannot create runner ServiceAccount: %s", err)
		return r.handleRetry(ctx, action, v1alpha1.RunningActionPhase, msg)
	}

	if err := r.svc.EnsureRunnerInputDataCreated(ctx, sa.Name, action); err != nil {
		msg := fmt.Sprintf("Cannot create runner input: %s", err)
		return r.handleRetry(ctx, action, v1alpha1.RunningActionPhase, msg)
	}

	if err := r.svc.EnsureRunnerExecuted(ctx, sa.Name, action); err != nil {
		msg := fmt.Sprintf("Cannot execute runner: %s", err)
		return r.handleRetry(ctx, action, v1alpha1.RunningActionPhase, msg)
	}

	action.Status = r.successStatus(action, v1alpha1.RunningActionPhase, "Kubernetes runner executed. Waiting for finish phase.")
	if err := r.k8sCli.Status().Update(ctx, action); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "while updating status of executed action")
	}

	// requeue to check execution status
	return ctrl.Result{Requeue: true}, nil
}

// handleRunningAction checks execution status. If completed, sets final state v1alpha1.SucceededActionPhase,
// v1alpha1.CanceledActionPhase, or v1alpha1.FailedActionPhase depends on currently scheduled activity.
//
// TODO: add support for cancel phase.
func (r *ActionReconciler) handleRunningAction(ctx context.Context, action *v1alpha1.Action) (ctrl.Result, error) {
	type newStatusCreator func(ctx context.Context, action *v1alpha1.Action) (*v1alpha1.ActionStatus, error)
	steps := []newStatusCreator{
		r.reportedRunnerStatus,
		r.runnerJobStatus,
	}

	for _, step := range steps {
		newStatus, err := step(ctx, action)
		if err != nil {
			msg := fmt.Sprintf("Unable to check runner status: %s", err)
			return r.handleRetry(ctx, action, v1alpha1.RunningActionPhase, msg)
		}
		if newStatus == nil {
			continue
		}

		action.Status = *newStatus
		if err := r.k8sCli.Status().Update(ctx, action); err != nil {
			return ctrl.Result{}, errors.Wrap(err, "while updating status of executed action")
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// status didn't change, no need to requeue
	return ctrl.Result{}, nil
}

func (r *ActionReconciler) reportedRunnerStatus(ctx context.Context, action *v1alpha1.Action) (*v1alpha1.ActionStatus, error) {
	reportedStatus, err := r.svc.GetReportedRunnerStatus(ctx, action)
	if err != nil {
		return nil, errors.Wrap(err, "while getting scheduled runner status")
	}

	if !reportedStatus.Changed {
		return nil, nil
	}

	statusCpy := action.Status.DeepCopy()
	if statusCpy.Runner == nil {
		statusCpy.Runner = &v1alpha1.RunnerStatus{}
	}
	statusCpy.Runner.Status = &runtime.RawExtension{
		Raw: reportedStatus.Status,
	}

	return statusCpy, nil
}

func (r *ActionReconciler) runnerJobStatus(ctx context.Context, action *v1alpha1.Action) (*v1alpha1.ActionStatus, error) {
	out, err := r.svc.GetRunnerJobStatus(ctx, action)
	if err != nil {
		return nil, errors.Wrap(err, "while getting runner job status")
	}

	if !out.Finished {
		return nil, nil
	}

	var outStatus v1alpha1.ActionStatus
	switch out.JobStatus {
	case batchv1.JobComplete:
		outStatus = r.successStatus(action, v1alpha1.SucceededActionPhase, "Runner finished successfully")
	case batchv1.JobFailed:
		outStatus = r.failStatus(action, v1alpha1.FailedActionPhase, "Runner finished unsuccessfully")
	default:
		outStatus = r.failStatus(action, v1alpha1.FailedActionPhase, "Unknown runner job status")
	}

	return &outStatus, nil
}

func (r *ActionReconciler) handleRetry(ctx context.Context, action *v1alpha1.Action, currentPhase v1alpha1.ActionPhase, errMsg string) (ctrl.Result, error) {
	key := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      action.Name,
			Namespace: action.Namespace,
		},
	}
	retry := r.rateLimiter.NumRequeues(key)

	var result ctrl.Result
	switch {
	case retry < r.maxRetries:
		errMsg = fmt.Sprintf("%s (will retry - %d/%d)", errMsg, retry, r.maxRetries)
		action.Status = r.failStatus(action, currentPhase, errMsg)
		result = ctrl.Result{Requeue: true} // AddRateLimited
	default:
		errMsg = fmt.Sprintf("%s (giving up - exceeded %d retries)", errMsg, r.maxRetries)
		action.Status = r.failStatus(action, v1alpha1.FailedActionPhase, errMsg)
		result = ctrl.Result{} // no retry
	}

	if err := r.k8sCli.Status().Update(ctx, action); err != nil {
		return ctrl.Result{}, errors.Wrap(err, "while updating action object status")
	}

	return result, nil
}

// failStatus sets generic status fields to indicated action failed state. Emits proper K8s Event.
func (r *ActionReconciler) failStatus(action *v1alpha1.Action, phase v1alpha1.ActionPhase, msg string) v1alpha1.ActionStatus {
	return r.newStatusForAction(action, corev1.EventTypeWarning, phase, msg)
}

// successStatus sets generic status fields to indicated action success state. Emits proper K8s Event.
func (r *ActionReconciler) successStatus(action *v1alpha1.Action, phase v1alpha1.ActionPhase, msg string) v1alpha1.ActionStatus {
	return r.newStatusForAction(action, corev1.EventTypeNormal, phase, msg)
}

func (r *ActionReconciler) newStatusForAction(action *v1alpha1.Action, eventType string, phase v1alpha1.ActionPhase, msg string) v1alpha1.ActionStatus {
	r.recorder.Event(action, eventType, string(phase), msg)

	statusCpy := action.Status.DeepCopy()
	statusCpy.Phase = phase
	statusCpy.Message = ptr.String(msg)
	statusCpy.LastTransitionTime = metav1.Now()
	statusCpy.ObservedGeneration = action.Generation

	if statusCpy.Phase == action.Status.Phase {
		statusCpy.LastTransitionTime = action.Status.LastTransitionTime
	}

	return *statusCpy
}

func (r *ActionReconciler) SetupWithManager(mgr ctrl.Manager, maxConcurrentReconciles int) error {
	r.k8sCli = mgr.GetClient()
	r.recorder = mgr.GetEventRecorderFor("action-controller")
	r.rateLimiter = workqueue.DefaultControllerRateLimiter()

	return ctrl.NewControllerManagedBy(mgr).
		For(&v1alpha1.Action{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
			RateLimiter:             r.rateLimiter,
		}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
