package signjob

import (
	"context"
	"errors"
	"fmt"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/kubernetes-sigs/kernel-module-management/internal/sign"
	"github.com/kubernetes-sigs/kernel-module-management/internal/utils"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const jobHashAnnotation = "kmm.node.kubernetes.io/last-hash"

var errNoMatchingBuild = errors.New("no matching signing job")

type signJobManager struct {
	client client.Client
	signer Signer
	helper sign.Helper
}

func NewSignJobManager(client client.Client, signer Signer, helper sign.Helper) *signJobManager {
	return &signJobManager{
		client: client,
		signer: signer,
		helper: helper,
	}
}

func labels(mod kmmv1beta1.Module, targetKernel string) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel:    mod.Name,
		constants.TargetKernelTarget: targetKernel,
		constants.JobType:            "sign",
	}
}

func (jbm *signJobManager) getJob(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*batchv1.Job, error) {
	jobList := batchv1.JobList{}

	opts := []client.ListOption{
		client.MatchingLabels(labels(mod, targetKernel)),
		client.InNamespace(mod.Namespace),
	}

	if err := jbm.client.List(ctx, &jobList, opts...); err != nil {
		return nil, fmt.Errorf("could not list jobs: %v", err)
	}

	if n := len(jobList.Items); n == 0 {
		return nil, errNoMatchingBuild
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 job, got %d", n)
	}

	return &jobList.Items[0], nil
}

func (jbm *signJobManager) Sync(ctx context.Context, mod kmmv1beta1.Module, m kmmv1beta1.KernelMapping, targetKernel string, imageToSign string, targetImage string, pushImage bool) (sign.Result, error) {
	logger := log.FromContext(ctx)

	logger.Info("Signing in-cluster")

	signConfig := jbm.helper.GetRelevantSign(mod, m)

	jobTemplate, err := jbm.signer.MakeJobTemplate(mod, signConfig, targetKernel, imageToSign, targetImage, pushImage)
	if err != nil {
		return sign.Result{}, fmt.Errorf("could not make Job template: %v", err)
	}

	job, err := jbm.getJob(ctx, mod, targetKernel)
	if err != nil {
		if !errors.Is(err, errNoMatchingBuild) {
			return sign.Result{}, fmt.Errorf("error getting the signing job: %v", err)
		}

		logger.Info("Creating job")

		if err = jbm.client.Create(ctx, jobTemplate); err != nil {
			return sign.Result{}, fmt.Errorf("could not create Job: %v", err)
		}

		return sign.Result{Status: sign.StatusCreated, Requeue: true}, nil
	}

	changed, err := jbm.isJobChanged(job, jobTemplate)
	if err != nil {
		return sign.Result{}, fmt.Errorf("could not determine if job has changed: %v", err)
	}

	if changed {
		logger.Info("The module's sign spec has been changed, deleting the current job so a new one can be created", "name", job.Name)
		opts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		err = jbm.client.Delete(ctx, job, opts...)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete signing job %s: %v", job.Name, err)))
		}
		return sign.Result{Status: sign.StatusInProgress, Requeue: true}, nil
	}

	logger.Info("Returning job status", "name", job.Name, "namespace", job.Namespace)

	switch {
	case job.Status.Succeeded == 1:
		return sign.Result{Status: sign.StatusCompleted}, nil
	case job.Status.Active == 1:
		return sign.Result{Status: sign.StatusInProgress, Requeue: true}, nil
	case job.Status.Failed == 1:
		return sign.Result{}, fmt.Errorf("job failed: %v", err)
	default:
		return sign.Result{}, fmt.Errorf("unknown status: %v", job.Status)
	}
}

func (jbm *signJobManager) isJobChanged(existingJob *batchv1.Job, newJob *batchv1.Job) (bool, error) {
	existingAnnotations := existingJob.GetAnnotations()
	newAnnotations := newJob.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing job %s", existingJob.Name)
	}
	if existingAnnotations[jobHashAnnotation] == newAnnotations[jobHashAnnotation] {
		return false, nil
	}
	return true, nil
}
