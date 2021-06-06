package mainhandler

import (
	"context"
	"fmt"
	"k8s-ca-websocket/cautils"
	"time"

	"github.com/armosec/capacketsgo/apis"
	"github.com/armosec/capacketsgo/k8sinterface"

	"github.com/golang/glog"
	corev1 "k8s.io/api/core/v1"

	// corev1beta1 "k8s.io/api/core/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

func (actionHandler *ActionHandler) update() error {
	kind := cautils.GetKindFromWlid(actionHandler.wlid)
	workload, err := actionHandler.k8sAPI.GetWorkloadByWlid(actionHandler.wlid)
	if err != nil {
		glog.Error(err)
		return err
	}

	actionHandler.editWorkload(workload)

	glog.Infof("Command: %s, Updated workload: %s", actionHandler.command.CommandName, workload.Json())

	switch kind {
	case "Pod":
		glog.Infof("updating pod")
		return actionHandler.updatePod(workload)
	default:
		glog.Infof("default")
		return actionHandler.updateWorkload(workload)
	}
}

func (actionHandler *ActionHandler) updateWorkload(workload *k8sinterface.Workload) error {
	deletePods := isForceDelete(actionHandler.command.Args)

	if persistentVolumeFound(workload) {
		deletePods = true
	}

	if deletePods {
		glog.Infof("Updating workload by deleting pods, workloadID: %s", actionHandler.wlid)
		return actionHandler.deletePods(workload)
	}
	_, err := actionHandler.k8sAPI.UpdateWorkload(workload)
	return err
}

func (actionHandler *ActionHandler) updatePod(workload *k8sinterface.Workload) error {
	glog.Infof("in updatePod")
	var err error
	maxTime := float64(360) // wait for 3 minutes
	timer := float64(0)
	sleepTime := time.Second * 1

	if err = actionHandler.k8sAPI.DeleteWorkloadByWlid(actionHandler.wlid); err == nil {
		workload.RemovePodStatus()
		workload.RemoveResourceVersion()
		for {
			_, err = actionHandler.k8sAPI.GetWorkloadByWlid(actionHandler.wlid)
			if err != nil {
				glog.Error(err)
				break
			}
			if maxTime <= timer {
				return fmt.Errorf("Failed to restart pod, time: %v seconds, workloadID: %s", maxTime, actionHandler.wlid)
			}
			time.Sleep(sleepTime)
			timer += sleepTime.Seconds()

		}
		_, err = actionHandler.k8sAPI.CreateWorkload(workload)
	}
	return err
}

func (actionHandler *ActionHandler) editWorkload(workload *k8sinterface.Workload) {
	switch actionHandler.command.CommandName {
	case apis.UPDATE:
		workload.RemoveIgnore()
		workload.SetInject()
		workload.SetWlid(actionHandler.wlid)
		workload.SetUpdateTime()
	case apis.RESTART:
		workload.SetUpdateTime()
	case apis.INJECT:
		workload.RemoveIgnore()
		workload.SetInject()
	case apis.REMOVE:
		workload.SetIgnore()
		workload.RemoveInject()
		workload.RemoveWlid()
		workload.RemoveUpdateTime()
	}
}
func (actionHandler *ActionHandler) deletePods(workload *k8sinterface.Workload) error {
	lisOptions := metav1.ListOptions{}

	selector, err := workload.GetSelector()
	if err == nil && selector != nil {
		lisOptions.LabelSelector = labels.Set(selector.MatchLabels).AsSelector().String()
	}

	return actionHandler.k8sAPI.KubernetesClient.CoreV1().Pods(cautils.GetNamespaceFromWlid(actionHandler.wlid)).DeleteCollection(context.Background(), metav1.DeleteOptions{}, lisOptions)
}

func (actionHandler *ActionHandler) deleteConfigMaps() error {
	confName := cautils.GenarateConfigMapName(actionHandler.wlid)
	return actionHandler.k8sAPI.KubernetesClient.CoreV1().ConfigMaps(cautils.GetNamespaceFromWlid(actionHandler.wlid)).Delete(context.Background(), confName, metav1.DeleteOptions{})
}

func persistentVolumeFound(workload *k8sinterface.Workload) bool {
	volumes, _ := workload.GetVolumes()
	for _, vol := range volumes {
		if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName != "" {
			return true
		}
	}
	return false
}

func injectAnnotation(annotations *map[string]string, key, val string) {
	if *annotations == nil {
		(*annotations) = make(map[string]string)
	}
	(*annotations)[key] = val
}

func removeAnnotation(meatdata *metav1.ObjectMeta, key string) {
	if meatdata.Annotations != nil {
		delete(meatdata.Annotations, key)
	}
}

// UpdateSecret create secret in k8s
func (actionHandler *ActionHandler) UpdateSecret(secret *corev1.Secret, command string) error {
	ctx := context.Background()
	secretUpdate(&secret.ObjectMeta, command)
	_, err := actionHandler.k8sAPI.KubernetesClient.CoreV1().Secrets(secret.Namespace).Update(ctx, secret, metav1.UpdateOptions{})
	return err
}

// DeleteSecret delete secret from k8s
func (actionHandler *ActionHandler) DeleteSecret(namespace, secretName string) error {
	ctx := context.Background()
	err := actionHandler.k8sAPI.KubernetesClient.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	return err
}

// GetSecret get secret from k8s
func (actionHandler *ActionHandler) GetSecret(namespace, secretName string) (*corev1.Secret, error) {
	return actionHandler.k8sAPI.KubernetesClient.CoreV1().Secrets(namespace).Get(context.Background(), secretName, metav1.GetOptions{})
}

// ListSecrets list secret from k8s
func (actionHandler *ActionHandler) ListSecrets(namespace string, labelSelector map[string]string) (*corev1.SecretList, error) {
	ctx := context.Background()

	listOptions := metav1.ListOptions{}
	if labelSelector != nil {
		set := labels.Set(labelSelector)
		listOptions.LabelSelector = set.AsSelector().String()
	}
	return actionHandler.k8sAPI.KubernetesClient.CoreV1().Secrets(namespace).List(ctx, listOptions)
}
func removeLabel(meatdata *metav1.ObjectMeta, key string) {
	if meatdata.Labels != nil {
		delete(meatdata.Labels, key)
	}
}
func secretUpdate(objectMeta *metav1.ObjectMeta, command string) {
	switch command {
	case apis.DECRYPT:
		removeLabel(objectMeta, CAInject)
		injectAnnotation(&objectMeta.Annotations, CAIgnoe, "true")
	case apis.ENCRYPT:
		removeAnnotation(objectMeta, CAIgnoe)
		removeAnnotation(objectMeta, "kubectl.kubernetes.io/last-applied-configuration")
	}
}
