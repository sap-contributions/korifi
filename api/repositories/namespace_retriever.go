package repositories

import (
	"context"
	"fmt"
	"strings"

	apierrors "code.cloudfoundry.org/korifi/api/errors"
	korifiv1alpha1 "code.cloudfoundry.org/korifi/controllers/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfapps;cfbuilds;cfpackages;cfprocesses;cfspaces;cftasks,verbs=list
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfdomains;cfroutes,verbs=list
//+kubebuilder:rbac:groups=korifi.cloudfoundry.org,resources=cfservicebindings;cfserviceinstances,verbs=list

var (
	CFAppsGVR             = korifiv1alpha1.Resource("cfapps").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFBuildsGVR           = korifiv1alpha1.Resource("cfbuilds").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFDomainsGVR          = korifiv1alpha1.Resource("cfdomains").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFDropletsGVR         = korifiv1alpha1.Resource("cfdroplets").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFPackagesGVR         = korifiv1alpha1.Resource("cfpackages").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFProcessesGVR        = korifiv1alpha1.Resource("cfprocesses").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFRoutesGVR           = korifiv1alpha1.Resource("cfroutes").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFServiceBindingsGVR  = korifiv1alpha1.Resource("cfservicebindings").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFServiceInstancesGVR = korifiv1alpha1.Resource("cfserviceinstances").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFSpacesGVR           = korifiv1alpha1.Resource("cfspaces").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)
	CFTasksGVR            = korifiv1alpha1.Resource("cftasks").WithVersion(korifiv1alpha1.SchemeGroupVersion.Version)

	ResourceMap = map[string]schema.GroupVersionResource{
		AppResourceType:             CFAppsGVR,
		BuildResourceType:           CFBuildsGVR,
		DropletResourceType:         CFDropletsGVR,
		DomainResourceType:          CFDomainsGVR,
		PackageResourceType:         CFPackagesGVR,
		ProcessResourceType:         CFProcessesGVR,
		RouteResourceType:           CFRoutesGVR,
		ServiceBindingResourceType:  CFServiceBindingsGVR,
		ServiceInstanceResourceType: CFServiceInstancesGVR,
		SpaceResourceType:           CFSpacesGVR,
		TaskResourceType:            CFTasksGVR,
	}
)

type NamespaceRetriever struct {
	client dynamic.Interface
}

func NewNamespaceRetriever(client dynamic.Interface) NamespaceRetriever {
	return NamespaceRetriever{
		client: client,
	}
}

func (nr NamespaceRetriever) NamespaceFor(ctx context.Context, resourceGUID, resourceType string) (string, error) {
	gvr, ok := ResourceMap[resourceType]
	if !ok {
		return "", fmt.Errorf("resource type %q unknown", resourceType)
	}

	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", resourceGUID),
	}

	list, err := nr.client.Resource(gvr).List(ctx, opts)
	if err != nil {
		return "", fmt.Errorf("failed to list %v: %w", resourceType, apierrors.FromK8sError(err, resourceType))
	}

	if len(list.Items) == 0 {
		return "", apierrors.NewNotFoundError(fmt.Errorf("resource %q not found", resourceGUID), resourceType)
	}

	if len(list.Items) > 1 {
		return "", fmt.Errorf("get-%s duplicate records exist", strings.ToLower(resourceType))
	}

	metadata := list.Items[0].Object["metadata"].(map[string]interface{})

	ns := metadata["namespace"].(string)

	if ns == "" {
		return "", fmt.Errorf("get-%s: resource is not namespace-scoped", strings.ToLower(resourceType))
	}

	return ns, nil
}

func (nr NamespaceRetriever) GetNamespacedObject(ctx context.Context, resourceGUID string, obj runtime.Object, resource schema.GroupVersionResource) error {
	opts := metav1.ListOptions{
		FieldSelector: fmt.Sprintf("metadata.name=%s", resourceGUID),
	}

	list, err := nr.client.Resource(resource).List(ctx, opts)
	if err != nil {
		return fmt.Errorf("failed to list %v: %w", resource.Resource, apierrors.FromK8sError(err, resource.Resource))
	}

	if len(list.Items) == 0 {
		return apierrors.NewNotFoundError(fmt.Errorf("resource %q not found", resourceGUID), resource.Resource)
	}

	if len(list.Items) > 1 {
		return fmt.Errorf("get-%s duplicate records exist", strings.ToLower(resource.Resource))
	}

	return runtime.DefaultUnstructuredConverter.FromUnstructured(list.Items[0].Object, obj)
}
