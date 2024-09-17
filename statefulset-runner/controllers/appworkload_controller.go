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

	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	"code.cloudfoundry.org/korifi/tools/k8s"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

const (
	// Environment Variable Names
	EnvPodName              = "POD_NAME"
	EnvCFInstanceIP         = "CF_INSTANCE_IP"
	EnvCFInstanceGUID       = "CF_INSTANCE_GUID"
	EnvCFInstanceInternalIP = "CF_INSTANCE_INTERNAL_IP"
	EnvCFInstanceIndex      = "CF_INSTANCE_INDEX"

	// StatefulSet Keys
	AnnotationVersion     = "korifi.cloudfoundry.org/version"
	AnnotationAppID       = "korifi.cloudfoundry.org/application-id"
	AnnotationProcessGUID = "korifi.cloudfoundry.org/process-guid"

	LabelGUID            = "korifi.cloudfoundry.org/guid"
	LabelVersion         = "korifi.cloudfoundry.org/version"
	LabelAppGUID         = "korifi.cloudfoundry.org/app-guid"
	LabelAppWorkloadGUID = "korifi.cloudfoundry.org/appworkload-guid"
	LabelProcessType     = "korifi.cloudfoundry.org/process-type"

	ApplicationContainerName  = "application"
	AppWorkloadReconcilerName = "statefulset-runner"
	ServiceAccountName        = "korifi-app"

	LivenessFailureThreshold  = 4
	ReadinessFailureThreshold = 1

	PodAffinityTermWeight = 100
)

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate
//counterfeiter:generate -o ../fake -fake-name PDB . PDB
type PDB interface {
	Update(ctx context.Context, statefulSet *appsv1.StatefulSet) error
}

//counterfeiter:generate -o ../fake -fake-name WorkloadToStatefulsetConverter . WorkloadToStatefulsetConverter
type WorkloadToStatefulsetConverter interface {
	Convert(appWorkload *korifiv1alpha1.AppWorkload) (*appsv1.StatefulSet, error)
}

// AppWorkloadReconciler reconciles a AppWorkload object
type AppWorkloadReconciler struct {
	remoteClient     client.Client
	localClient      client.Client
	localCache       cache.Cache
	localMapper      meta.RESTMapper
	scheme           *runtime.Scheme
	workloadsToStSet WorkloadToStatefulsetConverter
	pdb              PDB
	log              logr.Logger
}

func NewAppWorkloadReconciler(
	remoteClient client.Client,
	localClient client.Client,
	localCache cache.Cache,
	localMapper meta.RESTMapper,
	scheme *runtime.Scheme,
	workloadsToStSet WorkloadToStatefulsetConverter,
	pdb PDB,
	log logr.Logger,
) *k8s.PatchingReconciler[korifiv1alpha1.AppWorkload, *korifiv1alpha1.AppWorkload] {
	appWorkloadReconciler := AppWorkloadReconciler{
		remoteClient:     remoteClient,
		localClient:      localClient,
		localCache:       localCache,
		localMapper:      localMapper,
		scheme:           scheme,
		workloadsToStSet: workloadsToStSet,
		pdb:              pdb,
		log:              log,
	}
	return k8s.NewPatchingReconciler[korifiv1alpha1.AppWorkload, *korifiv1alpha1.AppWorkload](log, remoteClient, &appWorkloadReconciler)
}

func (r *AppWorkloadReconciler) SetupWithManager(mgr ctrl.Manager) *builder.Builder {
	return ctrl.NewControllerManagedBy(mgr).
		For(&korifiv1alpha1.AppWorkload{}).
		WatchesRawSource(source.Kind[client.Object](r.localCache, &appsv1.StatefulSet{}, handler.EnqueueRequestForOwner(r.scheme, mgr.GetRESTMapper(), &korifiv1alpha1.AppWorkload{}))).
		WithEventFilter(predicate.NewPredicateFuncs(filterAppWorkloads))
}

func filterAppWorkloads(object client.Object) bool {
	appWorkload, ok := object.(*korifiv1alpha1.AppWorkload)
	if !ok {
		return true
	}

	return appWorkload.Spec.RunnerName == AppWorkloadReconcilerName
}

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads,verbs=get;list;watch;create;patch;delete
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=appworkloads/status,verbs=get;patch

//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=create;patch;get;list;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets/finalizers,verbs=update

//+kubebuilder:rbac:groups=policy,resources=poddisruptionbudgets,verbs=create;patch;deletecollection

func (r *AppWorkloadReconciler) ReconcileResource(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)

	appWorkload.Status.ObservedGeneration = appWorkload.Generation
	log.V(1).Info("set observed generation", "generation", appWorkload.Status.ObservedGeneration)

	statefulSet, err := r.workloadsToStSet.Convert(appWorkload)
	// Not clear what errors this would produce, but we may use it later
	if err != nil {
		log.Info("error when converting AppWorkload", "reason", err)
		return ctrl.Result{}, err
	}

	if err := r.ensureNamespace(ctx, appWorkload.GetNamespace()); err != nil {
		log.Info("error when creating namespace", "reason", err)
		return ctrl.Result{}, err
	}

	if err := r.ensureServiceAccount(ctx, appWorkload.GetNamespace(), ServiceAccountName); err != nil {
		log.Info("error when creating service account", "reason", err)
		return ctrl.Result{}, err
	}

	if err := r.synchronizeSecrets(ctx, appWorkload); err != nil {
		log.Info("failed to synchronize secrets", "reason", err)
		return ctrl.Result{}, err
	}

	createdStSet := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSet.Name,
			Namespace: statefulSet.Namespace,
		},
	}
	_, err = controllerutil.CreateOrPatch(ctx, r.localClient, createdStSet, func() error {
		createdStSet.Labels = statefulSet.Labels
		createdStSet.Annotations = statefulSet.Annotations
		createdStSet.OwnerReferences = statefulSet.OwnerReferences
		createdStSet.Spec = statefulSet.Spec

		return nil
	})
	if err != nil {
		log.Info("error when creating or updating StatefulSet", "reason", err)
		return ctrl.Result{}, err
	}

	err = r.pdb.Update(ctx, createdStSet)
	if err != nil {
		log.Info("error when creating or patching pod disruption budget", "reason", err)
		return ctrl.Result{}, err
	}

	appWorkload.Status.ActualInstances = createdStSet.Status.Replicas

	return ctrl.Result{}, nil
}

func (r *AppWorkloadReconciler) ensureServiceAccount(ctx context.Context, namespace, name string) error {
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
	r.log.Info("cf-space namespace reconciled", "name", name, "result", res)

	return nil
}

func (r *AppWorkloadReconciler) ensureNamespace(ctx context.Context, name string) error {
	res, err := controllerutil.CreateOrPatch(ctx, r.localClient, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}, nil)
	if err != nil {
		return err
	}
	r.log.Info("cf-space namespace reconciled", "name", name, "result", res)

	return nil
}

func (r *AppWorkloadReconciler) synchronizeSecrets(ctx context.Context, appWorkload *korifiv1alpha1.AppWorkload) error {
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

		r.log.Info("secret reconciled", "namespace", appWorkload.GetNamespace(), "name", s.Name, "result", res)
	}

	return r.ensureEnvSecrets(ctx, appWorkload.GetNamespace(), appWorkload.Spec.Env)
}

func (r *AppWorkloadReconciler) ensureEnvSecrets(ctx context.Context, namespace string, env []corev1.EnvVar) error {
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

			r.log.Info("ensured secret", "namespace", namespace, "name", s.GetName(), "result", res)
		}
	}
	return nil
}
