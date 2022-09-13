package signer

import (
	"context"
	"errors"
	"fmt"

	kmmv1beta1 "github.com/kubernetes-sigs/kernel-module-management/api/v1beta1"
	"github.com/kubernetes-sigs/kernel-module-management/internal/constants"
	"github.com/kubernetes-sigs/kernel-module-management/internal/registry"
	"github.com/kubernetes-sigs/kernel-module-management/internal/build"
	batchv1 "k8s.io/api/batch/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

var errNoMatchingJob = errors.New("no matching job")

type signingManager struct {
	client   client.Client
	registry registry.Registry
	signer   Signer
	helper   build.Helper
}

func NewSigningManager(client client.Client, registry registry.Registry, signer Signer, helper build.Helper) *signingManager {
	return &signingManager{
		client:   client,
		registry: registry,
		signer:    signer,
		helper:   helper,
	}
}

func labels(mod kmmv1beta1.Module, targetKernel string, jobname string) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel:      mod.Name,
		constants.TargetKernelTarget:   targetKernel,
		constants.BuildStage: 		jobname,
	}
}

func (sm *signingManager) GetName() string {
	return sm.signer.GetName()
}

func (sm *signingManager) ShouldRun(mod *kmmv1beta1.Module, km *kmmv1beta1.KernelMapping) bool {
	if mod.Spec.ModuleLoader.Container.Sign == nil && km.Sign == nil {
		return false
	}
	return true
}


func (sm *signingManager) getJob(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*batchv1.Job, error) {
	jobList := batchv1.JobList{}

	opts := []client.ListOption{
		client.MatchingLabels(labels(mod, targetKernel, sm.GetName())),
		client.InNamespace(mod.Namespace),
	}
	if err := sm.client.List(ctx, &jobList, opts...); err != nil {
		return nil, fmt.Errorf("could not list jobs: %v", err)
	}

	if n := len(jobList.Items); n == 0 {
		return nil, errNoMatchingJob
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 job, got %d", n)
	}

	return &jobList.Items[0], nil
}


func (sm *signingManager) Sync(ctx context.Context, mod kmmv1beta1.Module, km kmmv1beta1.KernelMapping, targetKernel string, pushImage bool) (build.Result, error) {
	logger := log.FromContext(ctx)

	signConfig := sm.helper.GetRelevantSign(mod, km)

	// find a job with the matching labels, if its there we're running already
	// if not we need to run
	job, err := sm.getJob(ctx, mod, targetKernel)
	if err != nil {
		if !errors.Is(err, errNoMatchingJob) {
			return build.Result{}, fmt.Errorf("error getting the job: %v", err)
		}

		logger.Info("Creating job")

		job, err = sm.signer.MakeJob(mod, signConfig, targetKernel, pushImage)
		if err != nil {
			return build.Result{}, fmt.Errorf("could not make Job: %v", err)
		}

		if err = sm.client.Create(ctx, job); err != nil {
			return build.Result{}, fmt.Errorf("could not create Job: %v", err)
		}

		return build.Result{Status: build.StatusCreated, Requeue: true}, nil
	}

	logger.Info("Returning job status", "name", job.Name, "namespace", job.Namespace)

	switch {
	case job.Status.Succeeded == 1:
		return build.Result{Status: build.StatusCompleted}, nil
	case job.Status.Active == 1:
		return build.Result{Status: build.StatusInProgress, Requeue: true}, nil
	case job.Status.Failed == 1:
		return build.Result{}, fmt.Errorf("job failed: %v", err)
	default:
		return build.Result{}, fmt.Errorf("unknown status: %v", job.Status)
	}
}
