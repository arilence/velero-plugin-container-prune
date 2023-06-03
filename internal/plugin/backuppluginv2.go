/*
Copyright the Velero contributors.

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

package plugin

import (
	"strings"

	"github.com/sirupsen/logrus"

	corev1api "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/pkg/errors"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	biav2 "github.com/vmware-tanzu/velero/pkg/plugin/velero/backupitemaction/v2"
)

const (
	// This annotation is a CSV string.
	// If this annotation is found on a backup resource, any container names
	// matching a value inside the CSV will not be backed up.
	AsyncBIAContainerPruneAnnotation	= "velero.arilence.com/prune-containers"
)

// BackupPluginV2 is a v2 backup item action plugin for Velero.
type BackupPluginV2 struct {
	log logrus.FieldLogger
}

// NewBackupPluginV2 instantiates a v2 BackupPlugin.
func NewBackupPluginV2(log logrus.FieldLogger) *BackupPluginV2 {
	return &BackupPluginV2{log: log}
}

// Name is required to implement the interface, but the Velero pod does not delegate this
// method -- it's used to tell velero what name it was registered under. The plugin implementation
// must define it, but it will never actually be called.
func (p *BackupPluginV2) Name() string {
	return "pruneContainerBackupPlugin"
}

// AppliesTo returns information about which resources this action should be invoked for.
// The IncludedResources and ExcludedResources slices can include both resources
// and resources with group names. These work: "ingresses", "ingresses.extensions".
// A BackupPlugin's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.
func (p *BackupPluginV2) AppliesTo() (velero.ResourceSelector, error) {
	// containers are only found inside pods.
	return velero.ResourceSelector{IncludedResources: []string{"pods"}}, nil
}

func GetClient() (*kubernetes.Clientset, error) {
        loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
        configOverrides := &clientcmd.ConfigOverrides{}
        kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
        clientConfig, err := kubeConfig.ClientConfig()
        if err != nil {
                return nil, errors.WithStack(err)
        }

        client, err := kubernetes.NewForConfig(clientConfig)
        if err != nil {
                return nil, errors.WithStack(err)
        }

        return client, nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
// in this case, setting a custom annotation on the item being backed up.
func (p *BackupPluginV2) Execute(item runtime.Unstructured, backup *v1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, string, []velero.ResourceIdentifier, error) {
	metadata, err := meta.Accessor(item)
	if err != nil {
		return nil, nil, "", nil, err
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	// Operations during finalize aren't supported, so if backup is in a finalize phase, just return the item
	if backup.Status.Phase == v1.BackupPhaseFinalizing ||
		backup.Status.Phase == v1.BackupPhaseFinalizingPartiallyFailed {
		return item, nil, "", nil, nil
	}

	// Item doesn't have the prune annotation, so we ignore it and return the item
	annotationValue, ok := annotations[AsyncBIAContainerPruneAnnotation]
	if !ok {
		return item, nil, "", nil, nil
	}
	var podsToPrune []string = strings.Split(annotationValue, ",")
	p.log.Infof("Found annotation %s on pod", AsyncBIAContainerPruneAnnotation)

	// Retrieve Pod spec
	pod := new(corev1api.Pod)
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), pod); err != nil {
		return nil, nil, "", nil, err
	}

	// Remove matching initContainers (if they exist)
	if pod.Spec.InitContainers != nil {
		for idx, c := range pod.Spec.InitContainers {
			for _, v := range podsToPrune {
				if c.Name == v {
					p.log.Infof("Removed %s container from pod %s", v, pod.Name)
					pod.Spec.InitContainers = append(pod.Spec.InitContainers[:idx], pod.Spec.InitContainers[idx+1:]...)
				}
			}
		}
	}

	// Remove matching sidecar (if they exist)
	if pod.Spec.Containers != nil {
		for idx, c := range pod.Spec.Containers {
			for _, v := range podsToPrune {
				if c.Name == v {
					p.log.Infof("Removed %s container from pod %s", v, pod.Name)
					pod.Spec.Containers = append(pod.Spec.Containers[:idx], pod.Spec.Containers[idx+1:]...)
				}
			}
		}
		for idx, c := range pod.Spec.Volumes {
			for _, v := range podsToPrune {
				if c.Name == v {
					p.log.Infof("Removed %s volume from pod %s", v, pod.Name)
					pod.Spec.Volumes = append(pod.Spec.Volumes[:idx], pod.Spec.Volumes[idx+1:] ...)
				}
			}
		}
	}

	// Return the new result
	res, err := runtime.DefaultUnstructuredConverter.ToUnstructured(pod)
	if err != nil {
		p.log.Errorf("Error converting Pod back to unstructured schema")
		return nil, nil, "", nil, err
	}
	return &unstructured.Unstructured{Object: res}, nil, "", nil, nil
}

func (p *BackupPluginV2) Progress(operationID string, backup *v1.Backup) (velero.OperationProgress, error) {
	progress := velero.OperationProgress{}
	if operationID == "" {
		return progress, biav2.InvalidOperationIDError(operationID)
	}

	return progress, nil
}

func (p *BackupPluginV2) Cancel(operationID string, backup *v1.Backup) error {
	return nil
}
