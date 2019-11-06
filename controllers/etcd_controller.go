// Copyright (c) 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"time"

	"github.com/sirupsen/logrus"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	kubernetes "github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/gardener/pkg/utils/imagevector"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	etcdChartPath = filepath.Join("charts", "etcd")
	imageYAMLPath = filepath.Join("charts", "images.yaml")
	logger        = logrus.New()
)

const (
	// FinalizerName is the name of the Plant finalizer.
	FinalizerName = "druid.gardener.cloud/etcd-druid"
	timeout       = time.Second * 30
)

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Scheme       *runtime.Scheme
	chartApplier kubernetes.ChartApplier
	Config       *rest.Config
	ChartApplier kubernetes.ChartApplier
}

func NewEtcdReconciler(mgr manager.Manager) (*EtcdReconciler, error) {
	return (&EtcdReconciler{
		Client: mgr.GetClient(),
		Config: mgr.GetConfig(),
		Scheme: mgr.GetScheme(),
	}).InitializeControllerWithChartApplier()
}

// InitializeChartApplier will use EtcdReconciler client to intialize a Kubernetes client as well as
// InitializeControllerWithChartApplier will use EtcdReconciler client to intialize a Kubernetes client as well as
// a Chart renderer.
func (r *EtcdReconciler) InitializeControllerWithChartApplier() (*EtcdReconciler, error) {
	if r.chartApplier != nil {
		return r, nil
	}

	renderer, err := chartrenderer.NewForConfig(r.Config)
	if err != nil {
		return nil, err
	}
	applier, err := kubernetes.NewApplierForConfig(r.Config)
	if err != nil {
		return nil, err
	}
	r.ChartApplier = kubernetes.NewChartApplier(renderer, applier)
	return r, nil
}

// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.gardener.cloud,resources=etcds/status,verbs=get;update;patch

// Reconcile reconciles the <req>.
func (r *EtcdReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// your logic here
	etcd := &druidv1alpha1.Etcd{}
	if err := r.Get(context.TODO(), req.NamespacedName, etcd); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		return ctrl.Result{}, err
	}
	etcdCopy := etcd.DeepCopy()
	// Update the found object and write the result back if there are any changes
	if !reflect.DeepEqual(etcd.Spec, etcdCopy.Spec) {
		etcdCopy.Spec = etcd.Spec
	}
	logger.Infof("Reconciling etcd: %s", etcd.GetName())
	if !etcdCopy.DeletionTimestamp.IsZero() {
		logger.Infof("Deletion timestamp set for etcd: %s", etcd.GetName())
		if err := r.removeFinalizersToDependantSecrets(etcdCopy); err != nil {
			if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, nil
			}
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}

		if sets.NewString(etcd.Finalizers...).Has(FinalizerName) {
			logger.Infof("Removing finalizer (%s) from etcd %s", FinalizerName, etcd.GetName())
			finalizers := sets.NewString(etcdCopy.Finalizers...)
			finalizers.Delete(FinalizerName)
			etcdCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
				if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
					return ctrl.Result{
						Requeue:      true,
						RequeueAfter: time.Second * 5,
					}, nil
				}
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, err
			}
		}
		logger.Infof("Deleted etcd %s successfully.", etcd.GetName())
		return ctrl.Result{}, nil
	}

	// Add Finalizers to Etcd
	if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Infof("Adding finalizer (%s) to etcd %s", FinalizerName, etcd.GetName())
		finalizers.Insert(FinalizerName)
		etcdCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
			if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
				return ctrl.Result{
					Requeue:      true,
					RequeueAfter: time.Second * 5,
				}, err
			}
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
	}
	if err := r.addFinalizersToDependantSecrets(etcdCopy); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
	}

	if err := r.reconcileEtcd(etcdCopy); err != nil {
		if err := r.updateEtcdErrorStatus(etcd, etcdCopy, err); err != nil {
			return ctrl.Result{
				Requeue:      true,
				RequeueAfter: time.Second * 5,
			}, err
		}
	}

	if err := r.updateEtcdStatus(etcd); err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, err
	}

	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Second * 5,
	}, nil
}

func (r *EtcdReconciler) getEtcdService(etcd *druidv1alpha1.Etcd) (*corev1.ServiceList, error) {
	etcdServiceList := &corev1.ServiceList{}
	err := r.List(context.TODO(), etcdServiceList, client.InNamespace(etcd.Namespace), client.MatchingLabels(etcd.Spec.Labels))
	return etcdServiceList, err
}

func (r *EtcdReconciler) getEtcdStatefulSet(etcd *druidv1alpha1.Etcd) (*appsv1.StatefulSetList, error) {
	etcdStatefulSetList := &appsv1.StatefulSetList{}
	err := r.List(context.TODO(), etcdStatefulSetList, client.InNamespace(etcd.Namespace), client.MatchingLabels(etcd.Spec.Labels))
	return etcdStatefulSetList, err
}

func (r *EtcdReconciler) getEtcdConfigMap(etcd *druidv1alpha1.Etcd) (*corev1.ConfigMapList, error) {
	etcdConfigMapList := &corev1.ConfigMapList{}
	err := r.List(context.TODO(), etcdConfigMapList, client.InNamespace(etcd.Namespace), client.MatchingLabels(etcd.Spec.Labels))
	return etcdConfigMapList, err
}

func (r *EtcdReconciler) getExistingEtcdResourcesAndAnnotate(etcd *druidv1alpha1.Etcd) (bool, error) {
	etcdServiceList, err := r.getEtcdService(etcd)
	if err != nil {
		return false, err
	}

	etcdStatefulsetList, err := r.getEtcdStatefulSet(etcd)
	if err != nil {
		return false, err
	}

	etcdConfigMapList, err := r.getEtcdConfigMap(etcd)
	if err != nil {
		return false, err
	}

	if (len(etcdServiceList.Items) + len(etcdStatefulsetList.Items) + len(etcdConfigMapList.Items)) == 0 {
		return false, nil
	}

	for _, serviceToPatch := range etcdServiceList.Items {
		service := serviceToPatch.DeepCopy()
		if !checkForEtcdOwnerReference(serviceToPatch.OwnerReferences, etcd) {
			if err := controllerutil.SetControllerReference(etcd, &serviceToPatch, r.Scheme); err != nil {
				return true, err
			}
			err := r.Patch(context.TODO(), &serviceToPatch, client.MergeFrom(service))
			if err != nil {
				return true, err
			}
		}
	}

	for _, ssToPatch := range etcdStatefulsetList.Items {
		ss := ssToPatch.DeepCopy()
		if !checkForEtcdOwnerReference(ssToPatch.OwnerReferences, etcd) {
			if err := controllerutil.SetControllerReference(etcd, &ssToPatch, r.Scheme); err != nil {
				return true, err
			}
			err := r.Patch(context.TODO(), &ssToPatch, client.MergeFrom(ss))
			if err != nil {
				return true, err
			}
		}
	}

	for _, cmToPatch := range etcdConfigMapList.Items {
		cm := cmToPatch.DeepCopy()
		if !checkForEtcdOwnerReference(cmToPatch.OwnerReferences, etcd) {
			if err := controllerutil.SetControllerReference(etcd, &cmToPatch, r.Scheme); err != nil {
				return true, err
			}
			err := r.Patch(context.TODO(), &cmToPatch, client.MergeFrom(cm))
			if err != nil {
				return true, err
			}
		}
	}

	return true, nil
}

func checkForEtcdOwnerReference(refs []metav1.OwnerReference, etcd *druidv1alpha1.Etcd) bool {
	for _, ownerRef := range refs {
		if ownerRef.UID == etcd.UID {
			return true
		}
	}
	return false
}

func (r *EtcdReconciler) reconcileEtcd(etcd *druidv1alpha1.Etcd) error {
	hasEtcdResources, err := r.getExistingEtcdResourcesAndAnnotate(etcd)
	if err != nil {
		return err
	}
	if hasEtcdResources {
		logger.Infof("Found existing etcd resources for %s in %s", etcd.Name, etcd.Namespace)
		return nil
	}
	var statefulsetReplicas int
	if etcd.Spec.Replicas != 0 {
		statefulsetReplicas = 1
	}

	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(imageYAMLPath)
	if err != nil {
	}

	images, err := imagevector.FindImages(imageVector, []string{"etcd", "backup-restore"})
	if err != nil {
		return err
	}

	etcdValues := map[string]interface{}{
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"serverPort":              etcd.Spec.Etcd.ServerPort,
		"clientPort":              etcd.Spec.Etcd.ClientPort,
		"imageRepository":         images["etcd"].Repository,
		"imageVersion":            etcd.Spec.Etcd.Version,
		"metrics":                 etcd.Spec.Etcd.Metrics,
		"resources":               etcd.Spec.Etcd.Resources,
		"enableTLS":               (etcd.Spec.Etcd.TLS != nil),
		"pullPolicy":              corev1.PullIfNotPresent,
		// "username":                etcd.Spec.Etcd.Username,
		// "password":                etcd.Spec.Etcd.Password,
	}

	var quota int64 = 2 * 1024 * 1024 * 1024 // 2Gib
	if etcd.Spec.Etcd.Quota != nil {
		quota = etcd.Spec.Etcd.Quota.Value()
	}
	backupValues := map[string]interface{}{
		"imageVersion":             etcd.Spec.Backup.Version,
		"imageRepository":          images["backup-restore"].Repository,
		"fullSnapshotSchedule":     etcd.Spec.Backup.FullSnapshotSchedule,
		"port":                     etcd.Spec.Backup.Port,
		"resources":                etcd.Spec.Backup.Resources,
		"pullPolicy":               corev1.PullIfNotPresent,
		"garbageCollectionPolicy":  etcd.Spec.Backup.GarbageCollectionPolicy,
		"etcdQuotaBytes":           quota,
		"etcdConnectionTimeout":    "30s",
		"snapstoreTempDir":         "/tmp",
		"garbageCollectionPeriod":  etcd.Spec.Backup.GarbageCollectionPeriod,
		"deltaSnapshotPeriod":      etcd.Spec.Backup.DeltaSnapshotPeriod,
		"deltaSnapshotMemoryLimit": etcd.Spec.Backup.DeltaSnapshotMemoryLimit,
	}

	storeValues := map[string]interface{}{
		"storageContainer": etcd.Spec.Backup.Store.Container,
		"storePrefix":      etcd.Spec.Backup.Store.Prefix,
		"storageProvider":  etcd.Spec.Backup.Store.Provider,
		"storeSecret":      etcd.Spec.Backup.Store.SecretRef.Name,
	}

	values := map[string]interface{}{
		"etcd":                etcdValues,
		"backup":              backupValues,
		"store":               storeValues,
		"name":                etcd.Name,
		"replicas":            etcd.Spec.Replicas,
		"labels":              etcd.Spec.Labels,
		"annotations":         etcd.Spec.Annotations,
		"storageClass":        etcd.Spec.StorageClass,
		"storageCapacity":     etcd.Spec.StorageCapacity,
		"uid":                 etcd.UID,
		"statefulsetReplicas": statefulsetReplicas,
	}

	if etcd.Spec.Etcd.TLS != nil {
		values["tlsServerSecret"] = etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name
		values["tlsClientSecret"] = etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name
	}
	if err := r.ChartApplier.ApplyChart(context.TODO(), etcdChartPath, etcd.Namespace, etcd.Name, nil, values); err != nil {
		return err
	}
	if err := r.updateEtcdStatus(etcd); err != nil {
		return err
	}
	return nil
}

func (r *EtcdReconciler) addFinalizersToDependantSecrets(etcd *druidv1alpha1.Etcd) error {

	storeSecret := corev1.Secret{}
	r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      etcd.Spec.Backup.Store.SecretRef.Name,
		Namespace: etcd.Namespace,
	}, &storeSecret)
	if finalizers := sets.NewString(storeSecret.Finalizers...); !finalizers.Has(FinalizerName) {
		logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, storeSecret.GetName())
		storeSecretCopy := storeSecret.DeepCopy()
		finalizers.Insert(FinalizerName)
		storeSecretCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
			return err
		}
	}
	if etcd.Spec.Etcd.TLS != nil {
		clientSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name,
			Namespace: etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Namespace,
		}, &clientSecret)
		if finalizers := sets.NewString(clientSecret.Finalizers...); !finalizers.Has(FinalizerName) {
			logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, clientSecret.GetName())
			clientSecretCopy := clientSecret.DeepCopy()
			finalizers.Insert(FinalizerName)
			clientSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
				return err
			}
		}

		serverSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name,
			Namespace: etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Namespace,
		}, &serverSecret)
		if finalizers := sets.NewString(serverSecret.Finalizers...); !finalizers.Has(FinalizerName) {
			logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, serverSecret.GetName())
			serverSecretCopy := serverSecret.DeepCopy()
			finalizers.Insert(FinalizerName)
			serverSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), serverSecretCopy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) removeFinalizersToDependantSecrets(etcd *druidv1alpha1.Etcd) error {
	storeSecret := corev1.Secret{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      etcd.Spec.Backup.Store.SecretRef.Name,
		Namespace: etcd.Namespace,
	}, &storeSecret); err != nil {
		return err
	}
	if finalizers := sets.NewString(storeSecret.Finalizers...); finalizers.Has(FinalizerName) {
		logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, storeSecret.GetName())
		storeSecretCopy := storeSecret.DeepCopy()
		finalizers.Delete(FinalizerName)
		storeSecretCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
			return err
		}
	}
	if etcd.Spec.Etcd.TLS != nil {
		clientSecret := corev1.Secret{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ClientTLSSecretRef.Name,
			Namespace: etcd.Namespace,
		}, &clientSecret); err != nil {
			return err
		}
		if finalizers := sets.NewString(clientSecret.Finalizers...); finalizers.Has(FinalizerName) {
			logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, clientSecret.GetName())
			clientSecretCopy := clientSecret.DeepCopy()
			finalizers.Delete(FinalizerName)
			clientSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
				return err
			}
		}

		serverSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.Etcd.TLS.ServerTLSSecretRef.Name,
			Namespace: etcd.Namespace,
		}, &serverSecret)
		if finalizers := sets.NewString(serverSecret.Finalizers...); finalizers.Has(FinalizerName) {
			logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, serverSecret.GetName())
			serverSecretCopy := serverSecret.DeepCopy()
			finalizers.Delete(FinalizerName)
			serverSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), serverSecretCopy); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *EtcdReconciler) updateStatusFromServices(etcd *druidv1alpha1.Etcd) error {

	// Delete the statefulset
	endpoints := corev1.Endpoints{}
	req := types.NamespacedName{
		Name:      fmt.Sprintf("etcd-%s-client", etcd.Name),
		Namespace: etcd.Namespace,
	}
	err := r.Get(context.TODO(), req, &endpoints)
	if err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	etcd.Status.Endpoints = append(etcd.Status.Endpoints, endpoints)
	logger.Infof("etcd endpoints: %v", etcd.Status.Endpoints)
	return nil
}

func (r *EtcdReconciler) updateEtcdErrorStatus(etcd, etcdCopy *druidv1alpha1.Etcd, lastError error) error {

	etcdStatus := etcdCopy.Status
	lastErrStr := fmt.Sprintf("%v", lastError)
	etcdStatus.LastError = &lastErrStr

	etcdCopy.Status = etcdStatus
	if err := r.Status().Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	return nil
}

func (r *EtcdReconciler) updateEtcdStatus(etcd *druidv1alpha1.Etcd) error {

	// Delete the statefulset
	ss := appsv1.StatefulSet{}
	req := types.NamespacedName{
		Name:      fmt.Sprintf("etcd-%s", etcd.Name),
		Namespace: etcd.Namespace,
	}
	if err := r.Get(context.TODO(), req, &ss); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	etcdCopy := etcd.DeepCopy()
	etcdStatus := &etcdCopy.Status
	etcdStatus.Etcd = druidv1alpha1.CrossVersionObjectReference{
		APIVersion: ss.APIVersion,
		Kind:       ss.Kind,
		Name:       ss.Name,
	}
	for _, condition := range ss.Status.Conditions {
		etcdStatus.Conditions = append(etcdStatus.Conditions, convertConditionsToEtcd(&condition))
	}

	// To be changed once we have multiple replicas.
	etcdStatus.CurrentReplicas = ss.Status.CurrentReplicas
	etcdStatus.ReadyReplicas = ss.Status.ReadyReplicas
	etcdStatus.UpdatedReplicas = ss.Status.UpdatedReplicas
	etcdStatus.Ready = (ss.Status.ReadyReplicas == ss.Status.Replicas)
	if err := r.updateStatusFromServices(etcdCopy); err != nil {
		return err
	}
	if err := r.Status().Patch(context.TODO(), etcdCopy, client.MergeFrom(etcd)); err != nil {
		if errors.IsNotFound(err) {
			// Object not found, return.  Created objects are automatically garbage collected.
			// For additional cleanup logic use finalizers.
			return nil
		}
		// Error reading the object - requeue the request.
		return err
	}
	return nil
}

func convertConditionsToEtcd(condition *appsv1.StatefulSetCondition) druidv1alpha1.Condition {
	return druidv1alpha1.Condition{
		Type:               druidv1alpha1.ConditionType(condition.Type),
		Status:             druidv1alpha1.ConditionStatus(condition.Status),
		LastTransitionTime: condition.LastTransitionTime,
		Reason:             condition.Reason,
		Message:            condition.Message,
	}
}

func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&druidv1alpha1.Etcd{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
