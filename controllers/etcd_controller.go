// Copyright (c) 2018 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

	druidv1 "github.com/gardener/etcd-druid/api/v1"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	kubernetes "github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	etcdChartPath = filepath.Join("charts", "etcd")
)

// FinalizerName is the name of the Plant finalizer.
const FinalizerName = "druid.sapcloud.io/etcd-druid"

// EtcdReconciler reconciles a Etcd object
type EtcdReconciler struct {
	client.Client
	Logger       *logrus.Logger
	chartApplier kubernetes.ChartApplier
	Config       *rest.Config
	ChartApplier kubernetes.ChartApplier
}

// InitializeChartApplier will use EtcdReconciler client to intialize a Kubernetes client as well as
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

// +kubebuilder:rbac:groups=druid.sapcloud.io,resources=etcds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=druid.sapcloud.io,resources=etcds/status,verbs=get;update;patch

func (r *EtcdReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	// your logic here
	etcd := &druidv1.Etcd{}
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
	r.Logger.Infof("Reconciling etcd: %s", etcd.GetName())
	if !etcdCopy.DeletionTimestamp.IsZero() {
		r.Logger.Infof("Deletion timestamp set for etcd: %s", etcd.GetName())
		if err := r.removeFinalizersToDependantSecrets(etcdCopy); err != nil {
			if err := r.updateEtcdErrorStatus(etcdCopy, err); err != nil {
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
			r.Logger.Infof("Removing finalizer (%s) from etcd %s", FinalizerName, etcd.GetName())
			finalizers := sets.NewString(etcdCopy.Finalizers...)
			finalizers.Delete(FinalizerName)
			etcdCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), etcdCopy); err != nil {
				if err := r.updateEtcdErrorStatus(etcdCopy, err); err != nil {
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
		r.Logger.Infof("Deleted etcd %s successfully.", etcd.GetName())
		return ctrl.Result{}, nil
	}

	// Add Finalizers to Etcd
	if etcd.DeletionTimestamp.IsZero() {
		if finalizers := sets.NewString(etcd.Finalizers...); !finalizers.Has(FinalizerName) {
			r.Logger.Infof("Adding finalizer (%s) to etcd %s", FinalizerName, etcd.GetName())
			finalizers.Insert(FinalizerName)
			etcdCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), etcdCopy); err != nil {
				if err := r.updateEtcdErrorStatus(etcdCopy, err); err != nil {
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
		if err := r.addFinalizersToDependantSecrets(etcdCopy); err != nil {
			if err := r.updateEtcdErrorStatus(etcdCopy, err); err != nil {
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
	etcdValues := map[string]interface{}{
		"defragmentationSchedule": etcd.Spec.Etcd.DefragmentationSchedule,
		"serverPort":              etcd.Spec.Etcd.ServerPort,
		"clientPort":              etcd.Spec.Etcd.ClientPort,
		"imageRepository":         etcd.Spec.Etcd.ImageRepository,
		"imageVersion":            etcd.Spec.Etcd.ImageVersion,
		"metrics":                 etcd.Spec.Etcd.MetricLevel,
		"resources":               etcd.Spec.Etcd.Resources,
		"enableTLS":               etcd.Spec.Etcd.EnableTLS,
		"pullPolicy":              etcd.Spec.Etcd.PullPolicy,
		"initialClusterToken":     etcd.Spec.Etcd.InitialClusterToken,
		"initialClusterState":     etcd.Spec.Etcd.InitialClusterToken,
		// "username":                etcd.Spec.Etcd.Username,
		// "password":                etcd.Spec.Etcd.Password,
	}

	backupValues := map[string]interface{}{
		"imageRepository":          etcd.Spec.Backup.ImageRepository,
		"imageVersion":             etcd.Spec.Backup.ImageVersion,
		"fullSnapshotSchedule":     etcd.Spec.Backup.FullSnapshotSchedule,
		"port":                     etcd.Spec.Backup.Port,
		"resources":                etcd.Spec.Backup.Resources,
		"pullPolicy":               etcd.Spec.Backup.PullPolicy,
		"garbageCollectionPolicy":  etcd.Spec.Backup.GarbageCollectionPolicy,
		"etcdQuotaBytes":           etcd.Spec.Backup.EtcdQuotaBytes,
		"etcdConnectionTimeout":    etcd.Spec.Backup.EtcdConnectionTimeout,
		"snapstoreTempDir":         etcd.Spec.Backup.SnapstoreTempDir,
		"garbageCollectionPeriod":  etcd.Spec.Backup.GarbageCollectionPeriod,
		"deltaSnapshotPeriod":      etcd.Spec.Backup.DeltaSnapshotPeriod,
		"deltaSnapshotMemoryLimit": etcd.Spec.Backup.DeltaSnapshotMemoryLimit,
	}

	storeValues := map[string]interface{}{
		"storageContainer": etcd.Spec.Store.StorageContainer,
		"storePrefix":      etcd.Spec.Store.StorePrefix,
		"storageProvider":  etcd.Spec.Store.StorageProvider,
		"storeSecret":      etcd.Spec.Store.StoreSecret,
	}

	values := map[string]interface{}{
		"etcd":               etcdValues,
		"backup":             backupValues,
		"store":              storeValues,
		"name":               etcd.Name,
		"pvcRetentionPolicy": etcd.Spec.PVCRetentionPolicy,
		"replicas":           etcd.Spec.Replicas,
		"labels":             etcd.Spec.Labels,
		"annotations":        etcd.Spec.Annotations,
		"storageClass":       etcd.Spec.StorageClass,
		"tlsServerSecret":    etcd.Spec.TLSServerSecretName,
		"tlsClientSecret":    etcd.Spec.TLSClientSecretName,
		"storageCapacity":    etcd.Spec.StorageCapacity,
		"uid":                etcd.UID,
	}

	if err := r.ChartApplier.ApplyChart(context.TODO(), etcdChartPath, etcd.Namespace, etcdCopy.Name, nil, values); err != nil {
		if err := r.updateEtcdErrorStatus(etcdCopy, err); err != nil {
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
	if err := r.updateEtcdStatus(etcdCopy); err != nil {
		return ctrl.Result{
			Requeue:      true,
			RequeueAfter: time.Second * 5,
		}, nil
	}
	return ctrl.Result{
		Requeue:      true,
		RequeueAfter: time.Second * 5,
	}, nil
}

func (r *EtcdReconciler) addFinalizersToDependantSecrets(etcd *druidv1.Etcd) error {

	storeSecret := corev1.Secret{}
	r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      etcd.Spec.Store.StoreSecret,
		Namespace: etcd.Namespace,
	}, &storeSecret)
	if finalizers := sets.NewString(storeSecret.Finalizers...); !finalizers.Has(FinalizerName) {
		r.Logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, storeSecret.GetName())
		storeSecretCopy := storeSecret.DeepCopy()
		finalizers.Insert(FinalizerName)
		storeSecretCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
			return err
		}
	}
	if etcd.Spec.Etcd.EnableTLS {
		clientSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.TLSClientSecretName,
			Namespace: etcd.Namespace,
		}, &clientSecret)
		if finalizers := sets.NewString(clientSecret.Finalizers...); !finalizers.Has(FinalizerName) {
			r.Logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, clientSecret.GetName())
			clientSecretCopy := clientSecret.DeepCopy()
			finalizers.Insert(FinalizerName)
			clientSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
				return err
			}
		}

		serverSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.TLSServerSecretName,
			Namespace: etcd.Namespace,
		}, &serverSecret)
		if finalizers := sets.NewString(serverSecret.Finalizers...); !finalizers.Has(FinalizerName) {
			r.Logger.Infof("Adding finalizer (%s) for secret %s", FinalizerName, serverSecret.GetName())
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

func (r *EtcdReconciler) removeFinalizersToDependantSecrets(etcd *druidv1.Etcd) error {
	storeSecret := corev1.Secret{}
	if err := r.Client.Get(context.TODO(), types.NamespacedName{
		Name:      etcd.Spec.Store.StoreSecret,
		Namespace: etcd.Namespace,
	}, &storeSecret); err != nil {
		return err
	}
	if finalizers := sets.NewString(storeSecret.Finalizers...); finalizers.Has(FinalizerName) {
		r.Logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, storeSecret.GetName())
		storeSecretCopy := storeSecret.DeepCopy()
		finalizers.Delete(FinalizerName)
		storeSecretCopy.Finalizers = finalizers.UnsortedList()
		if err := r.Update(context.TODO(), storeSecretCopy); err != nil {
			return err
		}
	}
	if etcd.Spec.Etcd.EnableTLS {
		clientSecret := corev1.Secret{}
		if err := r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.TLSClientSecretName,
			Namespace: etcd.Namespace,
		}, &clientSecret); err != nil {
			return err
		}
		if finalizers := sets.NewString(clientSecret.Finalizers...); finalizers.Has(FinalizerName) {
			r.Logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, clientSecret.GetName())
			clientSecretCopy := clientSecret.DeepCopy()
			finalizers.Delete(FinalizerName)
			clientSecretCopy.Finalizers = finalizers.UnsortedList()
			if err := r.Update(context.TODO(), clientSecretCopy); err != nil {
				return err
			}
		}

		serverSecret := corev1.Secret{}
		r.Client.Get(context.TODO(), types.NamespacedName{
			Name:      etcd.Spec.TLSServerSecretName,
			Namespace: etcd.Namespace,
		}, &serverSecret)
		if finalizers := sets.NewString(serverSecret.Finalizers...); finalizers.Has(FinalizerName) {
			r.Logger.Infof("Removing finalizer (%s) from secret %s", FinalizerName, serverSecret.GetName())
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

func (r *EtcdReconciler) updateStatusFromServices(etcd *druidv1.Etcd) error {

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
	r.Logger.Infof("etcd endpoints: %v", etcd.Status.Endpoints)
	return nil
}

func (r *EtcdReconciler) updateEtcdErrorStatus(etcd *druidv1.Etcd, lastError error) error {

	etcdStatus := etcd.Status

	etcdStatus.LastError = fmt.Sprintf("%v", lastError)

	etcd.Status = etcdStatus
	if err := r.Status().Update(context.TODO(), etcd); err != nil {
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

func (r *EtcdReconciler) updateEtcdStatus(etcd *druidv1.Etcd) error {

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

	etcdStatus := druidv1.Status{}
	etcdStatus.Etcd = druidv1.CrossVersionObjectReference{
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
	if err := r.updateStatusFromServices(etcd); err != nil {
		return err
	}
	etcd.Status = etcdStatus
	if err := r.Status().Update(context.TODO(), etcd); err != nil {
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

func convertConditionsToEtcd(condition *appsv1.StatefulSetCondition) druidv1.Condition {
	return druidv1.Condition{
		Type:               druidv1.ConditionType(condition.Type),
		Status:             druidv1.ConditionStatus(condition.Status),
		LastTransitionTime: condition.LastTransitionTime,
		Reason:             condition.Reason,
		Message:            condition.Message,
	}
}

func (r *EtcdReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&druidv1.Etcd{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
