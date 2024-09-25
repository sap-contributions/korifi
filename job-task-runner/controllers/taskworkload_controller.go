/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"fmt"
	"time"

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	workloadContainerName = "workload"
	ServiceAccountName    = "korifi-task"
)

//counterfeiter:generate -o fake -fake-name TaskStatusGetter . TaskStatusGetter

type TaskStatusGetter interface {
	GetStatusConditions(ctx context.Context, job *batchv1.Job) ([]metav1.Condition, error)
}

// TaskWorkloadReconciler reconciles a TaskWorkload object
type TaskWorkloadReconciler struct {
	remoteClient                               client.Client
	localClient                                client.Client
	localCache                                 cache.Cache
	localMapper                                meta.RESTMapper
	logger                                     logr.Logger
	scheme                                     *runtime.Scheme
	statusGetter                               TaskStatusGetter
	jobTTL                                     time.Duration
	jobTaskRunnerTemporarySetPodSeccompProfile bool
}

func NewTaskWorkloadReconciler(
	logger logr.Logger,
	remoteClient client.Client,
	localClient client.Client,
	localCache cache.Cache,
	localMapper meta.RESTMapper,
	scheme *runtime.Scheme,
	statusGetter TaskStatusGetter,
	jobTTL time.Duration,
	jobTaskRunnerTemporarySetPodSeccompProfile bool,
) *k8s.PatchingReconciler[korifiv1alpha1.TaskWorkload, *korifiv1alpha1.TaskWorkload] {
	taskReconciler := TaskWorkloadReconciler{
		remoteClient: remoteClient,
		localClient:  localClient,
		localCache:   localCache,
		localMapper:  localMapper,
		logger:       logger,
		scheme:       scheme,
		statusGetter: statusGetter,
		jobTTL:       jobTTL,
		jobTaskRunnerTemporarySetPodSeccompProfile: jobTaskRunnerTemporarySetPodSeccompProfile,
	}

	return k8s.NewPatchingReconciler[korifiv1alpha1.TaskWorkload, *korifiv1alpha1.TaskWorkload](logger, remoteClient, &taskReconciler)
}

func (r *TaskWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.TaskWorkload{}).
		WatchesRawSource(source.Kind[client.Object](r.localCache, &batchv1.Job{}, handler.EnqueueRequestForOwner(r.scheme, mgr.GetRESTMapper(), &korifiv1alpha1.TaskWorkload{})))
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads,verbs=get;list;watch;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/status,verbs=get;patch
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=taskworkloads/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;get;list;watch
//+kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *TaskWorkloadReconciler) ReconcileResource(ctx context.Context, taskWorkload *korifiv1alpha1.TaskWorkload) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	taskWorkload.Status.ObservedGeneration = taskWorkload.Generation
	log.V(1).Info("set observed generation", "generation", taskWorkload.Status.ObservedGeneration)

	if err := r.ensureNamespace(ctx, taskWorkload.GetNamespace()); err != nil {
		log.Info("error when creating namespace", "reason", err)
		return ctrl.Result{}, err
	}

	if err := r.ensureServiceAccount(ctx, taskWorkload.GetNamespace(), ServiceAccountName); err != nil {
		log.Info("error when creating service account", "reason", err)
		return ctrl.Result{}, err
	}

	if err := r.synchronizeSecrets(ctx, taskWorkload); err != nil {
		log.Info("failed to synchronize secrets", "reason", err)
		return ctrl.Result{}, err
	}

	job, err := r.getOrCreateJob(ctx, log, taskWorkload)
	if err != nil {
		return ctrl.Result{}, err
	}

	if job == nil {
		return ctrl.Result{}, nil
	}

	if err = r.updateTaskWorkloadStatus(ctx, taskWorkload, job); err != nil {
		log.Info("failed to update task workload status", "reason", err)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r TaskWorkloadReconciler) getOrCreateJob(ctx context.Context, logger logr.Logger, taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := &batchv1.Job{}

	err := r.remoteClient.Get(ctx, client.ObjectKeyFromObject(taskWorkload), job)
	if err == nil {
		return job, nil
	}

	if k8serrors.IsNotFound(err) {
		if meta.IsStatusConditionTrue(taskWorkload.Status.Conditions, korifiv1alpha1.TaskInitializedConditionType) {
			return nil, nil
		}

		return r.createJob(ctx, logger, taskWorkload)
	}

	logger.Info("getting job failed", "reason", err)
	return nil, err
}

func (r TaskWorkloadReconciler) createJob(ctx context.Context, logger logr.Logger, taskWorkload *korifiv1alpha1.TaskWorkload) (*batchv1.Job, error) {
	job := WorkloadToJob(taskWorkload, int32(r.jobTTL.Seconds()), r.jobTaskRunnerTemporarySetPodSeccompProfile)
	err := controllerutil.SetControllerReference(taskWorkload, job, r.scheme)
	if err != nil {
		return nil, err
	}

	err = r.localClient.Create(ctx, job)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logger.V(1).Info("job for TaskWorkload already exists")
		} else {
			logger.Info("failed to create job for task workload", "reason", err)
		}
		return nil, err
	}

	return job, nil
}

func (r *TaskWorkloadReconciler) ensureServiceAccount(ctx context.Context, namespace, name string) error {
	res, err := controllerutil.CreateOrPatch(ctx, r.localClient, &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Annotations: map[string]string{
				"cloudfoundry.org/propagate-service-account": "true",
				"cloudfoundry.org/propagate-deletion":        "false",
			},
		},
	}, nil)
	if err != nil {
		return err
	}
	r.logger.Info("cf-space namespace reconciled", "name", name, "result", res)

	return nil
}

func (r *TaskWorkloadReconciler) ensureNamespace(ctx context.Context, name string) error {
	res, err := controllerutil.CreateOrPatch(ctx, r.localClient, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, nil)
	if err != nil {
		return err
	}
	r.logger.Info("cf-space namespace reconciled", "name", name, "result", res)

	return nil
}

func (r *TaskWorkloadReconciler) synchronizeSecrets(ctx context.Context, appWorkload *korifiv1alpha1.TaskWorkload) error {
	for _, s := range appWorkload.Spec.ImagePullSecrets {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Namespace: appWorkload.GetNamespace(),
				Name:      s.Name,
			},
			Type: corev1.SecretTypeDockerConfigJson,
		}

		res, err := controllerutil.CreateOrPatch(ctx, r.localClient, secret, func() error {
			originalSecret := new(corev1.Secret)
			err := r.remoteClient.Get(ctx, types.NamespacedName{Namespace: appWorkload.GetNamespace(), Name: s.Name}, originalSecret)
			if err != nil {
				return err
			}

			secret.Data = originalSecret.Data

			return nil
		})
		if err != nil {
			return err
		}

		r.logger.Info("secret reconciled", "namespace", appWorkload.GetNamespace(), "name", s.Name, "result", res)
	}

	return r.ensureEnvSecrets(ctx, appWorkload.GetNamespace(), appWorkload.Spec.Env)
}

func (r *TaskWorkloadReconciler) ensureEnvSecrets(ctx context.Context, namespace string, env []corev1.EnvVar) error {
	for _, e := range env {
		if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
			secretRef := e.ValueFrom.SecretKeyRef
			s := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretRef.Name,
					Namespace: namespace,
				},
			}
			res, err := controllerutil.CreateOrPatch(ctx, r.localClient, s, func() error {
				originalSecret := new(corev1.Secret)
				if err := r.remoteClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: secretRef.Name}, originalSecret, nil); err != nil {
					return err
				}

				s.Data = originalSecret.Data
				s.Type = originalSecret.Type
				s.Immutable = originalSecret.Immutable

				return nil
			})
			if err != nil {
				return err
			}

			r.logger.Info("ensured secret", "namespace", namespace, "name", s.GetName(), "result", res)
		}
	}
	return nil
}

func WorkloadToJob(
	taskWorkload *korifiv1alpha1.TaskWorkload,
	jobTTL int32,
	jobTaskRunnerTemporarySetPodSeccompProfile bool,
) *batchv1.Job {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      taskWorkload.Name,
			Namespace: taskWorkload.Namespace,
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:            tools.PtrTo(int32(0)),
			Parallelism:             tools.PtrTo(int32(1)),
			Completions:             tools.PtrTo(int32(1)),
			TTLSecondsAfterFinished: tools.PtrTo(jobTTL),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyNever,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: tools.PtrTo(true),
					},
					AutomountServiceAccountToken: tools.PtrTo(false),
					ImagePullSecrets:             taskWorkload.Spec.ImagePullSecrets,
					Containers: []corev1.Container{{
						Name:      workloadContainerName,
						Image:     taskWorkload.Spec.Image,
						Command:   taskWorkload.Spec.Command,
						Resources: taskWorkload.Spec.Resources,
						Env:       taskWorkload.Spec.Env,
						SecurityContext: &corev1.SecurityContext{
							Capabilities: &corev1.Capabilities{
								Drop: []corev1.Capability{"ALL"},
							},
							AllowPrivilegeEscalation: tools.PtrTo(false),
							SeccompProfile: &corev1.SeccompProfile{
								Type: corev1.SeccompProfileTypeRuntimeDefault,
							},
						},
					}},
					ServiceAccountName: ServiceAccountName,
				},
			},
		},
	}

	if jobTaskRunnerTemporarySetPodSeccompProfile {
		job.Spec.Template.Spec.SecurityContext.SeccompProfile = &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		}
	}
	return job
}

func (r *TaskWorkloadReconciler) updateTaskWorkloadStatus(ctx context.Context, taskWorkload *korifiv1alpha1.TaskWorkload, job *batchv1.Job) error {
	conditions, err := r.statusGetter.GetStatusConditions(ctx, job)
	if err != nil {
		return fmt.Errorf("failed to get status conditions for job %s:%s: %w", job.Namespace, job.Name, err)
	}

	for _, condition := range conditions {
		condition.ObservedGeneration = taskWorkload.Generation
		meta.SetStatusCondition(&taskWorkload.Status.Conditions, condition)
	}

	return nil
}
