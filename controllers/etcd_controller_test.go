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
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gardener/gardener/pkg/utils/imagevector"
	"github.com/ghodss/yaml"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	controllersconfig "github.com/gardener/etcd-druid/controllers/config"
	"github.com/gardener/etcd-druid/pkg/chartrenderer"
	"github.com/gardener/etcd-druid/pkg/client/kubernetes"
	"github.com/gardener/etcd-druid/pkg/common"
	mockclient "github.com/gardener/etcd-druid/pkg/mock/controller-runtime/client"
	"github.com/gardener/etcd-druid/pkg/utils"
	v1beta1constants "github.com/gardener/gardener/pkg/apis/core/v1beta1/constants"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"
	appsv1 "k8s.io/api/apps/v1"
	batchv1beta1 "k8s.io/api/batch/v1beta1"
	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/helm/pkg/engine"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

type StatefulSetInitializer int

const (
	WithoutOwner        StatefulSetInitializer = 0
	WithOwnerReference  StatefulSetInitializer = 1
	WithOwnerAnnotation StatefulSetInitializer = 2
)

const (
	etcdConfig    = "etcd.conf.yaml"
	quotaKey      = "quota-backend-bytes"
	backupRestore = "backup-restore"
	metricsKey    = "metrics"
)

var (
	deltaSnapshotPeriod = metav1.Duration{
		Duration: 300 * time.Second,
	}
	garbageCollectionPeriod = metav1.Duration{
		Duration: 43200 * time.Second,
	}
	clientPort              int32 = 2379
	serverPort              int32 = 2380
	backupPort              int32 = 8080
	imageEtcd                     = "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap"
	imageBR                       = "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0"
	snapshotSchedule              = "0 */24 * * *"
	defragSchedule                = "0 */24 * * *"
	container                     = "default.bkp"
	storageCapacity               = resource.MustParse("5Gi")
	defaultStorageCapacity        = resource.MustParse("16Gi")
	storageClass                  = "gardener.fast"
	priorityClassName             = "class_priority"
	deltaSnapShotMemLimit         = resource.MustParse("100Mi")
	autoCompactionMode            = druidv1alpha1.Periodic
	autoCompactionRetention       = "2m"
	quota                         = resource.MustParse("8Gi")
	provider                      = druidv1alpha1.StorageProvider("Local")
	prefix                        = "/tmp"
	volumeClaimTemplateName       = "etcd-main"
	garbageCollectionPolicy       = druidv1alpha1.GarbageCollectionPolicy(druidv1alpha1.GarbageCollectionPolicyExponential)
	metricsBasic                  = druidv1alpha1.Basic
	maxBackups                    = 7
	imageNames                    = []string{
		common.Etcd,
		common.BackupRestore,
	}
	backupCompactionSchedule = "15 */24 * * *"
	etcdSnapshotTimeout      = metav1.Duration{
		Duration: 10 * time.Minute,
	}
	etcdDefragTimeout = metav1.Duration{
		Duration: 10 * time.Minute,
	}
)

func ownerRefIterator(element interface{}) string {
	return (element.(metav1.OwnerReference)).Name
}

func servicePortIterator(element interface{}) string {
	return (element.(corev1.ServicePort)).Name
}

func volumeMountIterator(element interface{}) string {
	return (element.(corev1.VolumeMount)).Name
}

func volumeIterator(element interface{}) string {
	return (element.(corev1.Volume)).Name
}

func keyIterator(element interface{}) string {
	return (element.(corev1.KeyToPath)).Key
}

func envIterator(element interface{}) string {
	return (element.(corev1.EnvVar)).Name
}

func containerIterator(element interface{}) string {
	return (element.(corev1.Container)).Name
}

func hostAliasIterator(element interface{}) string {
	return (element.(corev1.HostAlias)).IP
}

func pvcIterator(element interface{}) string {
	return (element.(corev1.PersistentVolumeClaim)).Name
}

func accessModeIterator(element interface{}) string {
	return string(element.(corev1.PersistentVolumeAccessMode))
}

func cmdIterator(element interface{}) string {
	return string(element.(string))
}

var _ = Describe("setup", func() {
	var (
		ctx      context.Context
		mockCtrl *gomock.Controller
		cl       *mockclient.MockClient
		sw       *mockclient.MockStatusWriter
		instance *druidv1alpha1.Etcd
	)

	BeforeEach(func() {
		ctx = context.Background()
		mockCtrl = gomock.NewController(GinkgoT())
		cl = mockclient.NewMockClient(mockCtrl)
		sw = mockclient.NewMockStatusWriter(mockCtrl)
		instance = getEtcd("foo1", "default", false)

		cl.EXPECT().Status().Return(sw).AnyTimes()
		cl.EXPECT().Scheme().Return(scheme.Scheme).AnyTimes()

		Expect(metav1.LabelSelectorAsSelector(instance.Spec.Selector)).ToNot(BeNil())
	})

	Describe("EtcdCustodian", func() {
		var etcdCustodian *EtcdCustodian

		BeforeEach(func() {
			etcdCustodian = createEtcdCustodian(cl)
		})

		Describe("Reconcile", func() {
			var (
				reconcileResult             ctrl.Result
				reconcileErr                error
				reconcileShouldSucceed      func()
				reconcileShouldRequeueAfter func()
			)

			JustBeforeEach(func() {
				mockGetAndUpdateEtcd(cl, sw, instance).AnyTimes()

				reconcileResult, reconcileErr = etcdCustodian.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(instance)})
			})

			reconcileShouldSucceed = func() {
				It("should reconcile successfully", func() {
					Expect(reconcileErr).ToNot(HaveOccurred())
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			}

			reconcileShouldRequeueAfter = func() {
				It("should reconcile successfully but should requeue after", func() {
					Expect(reconcileErr).ToNot(HaveOccurred())
					Expect(reconcileResult.RequeueAfter).ToNot(BeNumerically("==", 0))
				})
			}

			Describe("when listing leases", func() {
				var (
					lease        *coordinationv1.Lease
					leaseListErr error

					describeLeaseVariations = func(embedFn func()) {
						Describe("fails", func() {
							BeforeEach(func() {
								leaseListErr = errors.New("error")
							})

							AfterEach(func() {
								Expect(instance.Status.Members).To(Or(BeNil(), BeEmpty()), "Members status must be empty")
							})

							embedFn()
						})

						Describe("succeeds", func() {
							BeforeEach(func() {
								leaseListErr = nil
							})

							Describe("with no leases", func() {
								BeforeEach(func() {
									lease = nil
								})

								AfterEach(func() {
									Expect(instance.Status.Members).To(Or(BeNil(), BeEmpty()), "Members status must be empty")
								})

								embedFn()
							})

							Describe("with a lease", func() {
								var (
									memberName string
									memberID   string
									memberRole druidv1alpha1.EtcdRole
									renewTime  metav1.MicroTime
								)

								BeforeEach(func() {
									memberName = fmt.Sprintf("%s-%d", instance.Name, 0)
									memberID = "0"
									memberRole = druidv1alpha1.EtcdRoleLeader
									renewTime = metav1.NewMicroTime(time.Now())

									lease = &coordinationv1.Lease{
										ObjectMeta: metav1.ObjectMeta{
											Name:      memberName,
											Namespace: instance.Namespace,
										},
										Spec: coordinationv1.LeaseSpec{
											HolderIdentity:       pointer.StringPtr(fmt.Sprintf("%s:%s", memberID, memberRole)),
											LeaseDurationSeconds: pointer.Int32Ptr(300),
											RenewTime:            &renewTime,
										},
									}
								})

								AfterEach(func() {
									Expect(instance.Status.Members).To(ConsistOf(MatchFields(IgnoreExtras, Fields{
										"Name":   Equal(memberName),
										"ID":     PointTo(Equal(memberID)),
										"Role":   PointTo(Equal(memberRole)),
										"Status": Equal(druidv1alpha1.EtcdMemberStatusReady),
										"Reason": Equal("LeaseSucceeded"),
									})))
								})

								embedFn()
							})
						})
					}
				)

				BeforeEach(func() {
					cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&coordinationv1.LeaseList{}), gomock.Any()).DoAndReturn(
						func(_ context.Context, target *coordinationv1.LeaseList, _ ...client.ListOption) error {
							if leaseListErr != nil {
								return leaseListErr
							}

							if lease == nil {
								target.Items = nil
							} else {
								target.Items = []coordinationv1.Lease{*lease.DeepCopy()}
							}

							return nil
						},
					)
				})

				Describe("without any statefulset", func() {
					BeforeEach(func() {
						mockListGetAndUpdateStatefulSet(cl).AnyTimes()
					})

					describeLeaseVariations(func() {
						reconcileShouldRequeueAfter()

						It("should mark etcd resource as not ready", func() {
							Expect(instance.Status).To(MatchFields(IgnoreExtras, Fields{
								"Ready":         PointTo(BeFalse()),
								"ReadyReplicas": Equal(int32(0)),
							}))
						})
					})
				})

				Describe("with a statefulset", func() {
					var (
						sts *appsv1.StatefulSet
					)

					BeforeEach(func() {
						sts = &appsv1.StatefulSet{
							ObjectMeta: metav1.ObjectMeta{
								Name:      instance.Name,
								Namespace: instance.Namespace,
							},
						}
						mockListGetAndUpdateStatefulSet(cl, sts).AnyTimes()
					})

					Describe("if the statefulset is not claimed", func() {
						describeLeaseVariations(func() {
							reconcileShouldSucceed()

							It("should mark etcd resource as not ready", func() {
								Expect(instance.Status).To(MatchFields(IgnoreExtras, Fields{
									"Ready":         PointTo(BeFalse()),
									"ReadyReplicas": Equal(int32(0)),
								}))
							})
						})
					})

					Describe("if the statefulset is claimed", func() {
						BeforeEach(func() {
							sts.Annotations = map[string]string{
								common.GardenerOwnedBy:   client.ObjectKeyFromObject(instance).String(),
								common.GardenerOwnerType: strings.ToLower(etcdGVK.Kind),
							}
						})

						Describe("if statefulset is ready", func() {
							BeforeEach(func() {
								setStatefulSetReady(sts)
							})

							describeLeaseVariations(func() {
								reconcileShouldSucceed()

								It("should mark etcd resource as ready", func() {
									Expect(instance.Status).To(MatchFields(IgnoreExtras, Fields{
										"Ready":         PointTo(BeTrue()),
										"ReadyReplicas": Equal(int32(instance.Spec.Replicas)),
									}))
								})
							})
						})

						Describe("if statefulset is not ready", func() {
							describeLeaseVariations(func() {
								reconcileShouldSucceed()

								It("should mark etcd resource as not ready", func() {
									Expect(instance.Status).To(MatchFields(IgnoreExtras, Fields{
										"Ready":         PointTo(BeFalse()),
										"ReadyReplicas": Equal(int32(0)),
									}))
								})
							})
						})
					})

				})
			})
		})
	})

	Describe("EtcdReconciler", func() {
		var etcdReconciler *EtcdReconciler

		BeforeEach(func() {
			var err error

			etcdReconciler, err = createEtcdReconciler(cl)
			Expect(err).ToNot(HaveOccurred())
		})

		Describe("Reconcile", func() {
			var (
				reconcileResult               ctrl.Result
				reconcileErr                  error
				reconcileShouldSucceed        func()
				reconcileShouldFailAndRequeue func()
			)

			JustBeforeEach(func() {
				mockGetAndUpdateEtcd(cl, sw, instance).AnyTimes()

				reconcileResult, reconcileErr = etcdReconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKeyFromObject(instance)})
			})

			reconcileShouldSucceed = func() {
				It("should reconcile successfully", func() {
					Expect(reconcileErr).ToNot(HaveOccurred())
					Expect(reconcileResult).To(Equal(ctrl.Result{}))
				})
			}

			reconcileShouldFailAndRequeue = func() {
				It("should fail to reconcile and should requeue", func() {
					Expect(reconcileErr).To(HaveOccurred())
					Expect(reconcileResult).To(Equal(ctrl.Result{Requeue: true}))
				})
			}

			Describe("when backup secrets are present", func() {
				var backupSecret *corev1.Secret

				BeforeEach(func() {
					backupSecret = getSecret(instance.Spec.Backup.Store.SecretRef.Name, instance.Namespace)
					mockGetAndUpdateSecrets(cl, backupSecret).AnyTimes()
				})

				Describe("delete", func() {
					BeforeEach(func() {
						instance.DeletionTimestamp = &metav1.Time{Time: time.Now()}
						instance.Finalizers = append(instance.Finalizers, FinalizerName)
						backupSecret.Finalizers = append(backupSecret.Finalizers, FinalizerName)
					})

					Describe("on patching finalizers successfully", func() {
						BeforeEach(func() {
							expectPatchType := types.MergePatchType
							expectPatchData := []byte(`{"metadata":{"finalizers":null}}`)

							cl.EXPECT().Patch(gomock.Any(), gomock.AssignableToTypeOf(instance), gomock.Any()).DoAndReturn(
								func(_ context.Context, src *druidv1alpha1.Etcd, p client.Patch) error {
									Expect(p).ToNot(BeNil())
									Expect(p.Type()).To(Equal(expectPatchType))
									Expect(p.Data(src)).To(Equal(expectPatchData))

									instance.Finalizers = nil
									return nil
								},
							)

							cl.EXPECT().Patch(gomock.Any(), gomock.AssignableToTypeOf(backupSecret), gomock.Any()).DoAndReturn(
								func(_ context.Context, src *corev1.Secret, p client.Patch) error {
									Expect(p).ToNot(BeNil())
									Expect(p.Type()).To(Equal(expectPatchType))
									Expect(p.Data(src)).To(Equal(expectPatchData))

									backupSecret.Finalizers = nil
									return nil
								},
							)
						})

						AfterEach(func() {
							Expect(backupSecret.Finalizers).To(BeEmpty(), "finalizers should be removed from the backup secret")
							Expect(instance.Finalizers).To(BeEmpty(), "finalizer should be removed from the etcd resource")
						})

						Describe("without any statefulset", func() {
							BeforeEach(func() {
								mockListGetAndUpdateStatefulSet(cl).AnyTimes()
							})

							reconcileShouldSucceed()
						})

						Describe("with a statefulset", func() {
							var (
								sts *appsv1.StatefulSet
							)

							BeforeEach(func() {
								sts = &appsv1.StatefulSet{
									ObjectMeta: metav1.ObjectMeta{
										Name:      instance.Name,
										Namespace: instance.Namespace,
									},
								}
								mockListGetAndUpdateStatefulSet(cl, sts).AnyTimes()
							})

							Describe("if statefulset is not claimed", func() {
								reconcileShouldSucceed()

								It("should not delete the statefulset", func() {
									Expect(sts.DeletionTimestamp).To(BeNil())
								})
							})

							Describe("if statefulset is claimed", func() {
								BeforeEach(func() {
									cl.EXPECT().Delete(gomock.Any(), gomock.AssignableToTypeOf(sts), gomock.Any()).DoAndReturn(
										func(_ context.Context, _ *appsv1.StatefulSet, _ ...client.DeleteOption) error {
											sts.DeletionTimestamp = instance.DeletionTimestamp
											return nil
										},
									)
								})

								AfterEach(func() {
									Expect(sts.DeletionTimestamp).ToNot(BeNil(), "statefulset should be deleted")
								})

								Describe("statefulset has owner references", func() {
									BeforeEach(func() {
										sts.OwnerReferences = []metav1.OwnerReference{
											{
												APIVersion:         etcdGVK.GroupVersion().String(),
												Kind:               strings.ToLower(etcdGVK.Kind),
												Name:               instance.Name,
												UID:                instance.UID,
												Controller:         pointer.BoolPtr(true),
												BlockOwnerDeletion: pointer.BoolPtr(true),
											},
										}
									})

									reconcileShouldSucceed()
								})

								Describe("statefulset has owner annotations", func() {
									BeforeEach(func() {
										sts.Annotations = map[string]string{
											common.GardenerOwnedBy:   client.ObjectKeyFromObject(instance).String(),
											common.GardenerOwnerType: strings.ToLower(etcdGVK.Kind),
										}
									})

									reconcileShouldSucceed()
								})
							})
						})
					})
				})

				Describe("create or update", func() {
					AfterEach(func() {
						Expect(instance.Finalizers).To(ContainElement(FinalizerName), "etcd resource should have finalizer added")

						if instance.Spec.Backup.Store != nil {
							Expect(backupSecret.Finalizers).To(ContainElement(FinalizerName), "backup secret should have finalizer added")
						}
					})

					Describe("without services and configmaps", func() {
						var (
							svc *corev1.Service
							cm  *corev1.ConfigMap
						)

						BeforeEach(func() {
							mockListGetAndUpdateService(cl).Times(1)
							mockListGetAndUpdateConfigMap(cl).Times(1)

							svc = &corev1.Service{
								ObjectMeta: metav1.ObjectMeta{
									Name:      getServiceNameFor(instance),
									Namespace: instance.Namespace,
								},
							}
							mockListGetAndUpdateService(cl, svc).AnyTimes().After(
								cl.EXPECT().Create(gomock.Any(), gomock.AssignableToTypeOf(svc), gomock.Any()).DoAndReturn(
									func(_ context.Context, src *corev1.Service) error {
										src.DeepCopyInto(svc)
										return nil
									},
								),
							)

							cm = &corev1.ConfigMap{
								ObjectMeta: metav1.ObjectMeta{
									Name:      getConfigMapNameFor(instance),
									Namespace: instance.Namespace,
								},
							}
							mockListGetAndUpdateConfigMap(cl, cm).AnyTimes().After(
								cl.EXPECT().Create(gomock.Any(), gomock.AssignableToTypeOf(cm), gomock.Any()).DoAndReturn(
									func(_ context.Context, src *corev1.ConfigMap) error {
										src.DeepCopyInto(cm)
										return nil
									},
								),
							)
						})

						AfterEach(func() {
							Expect(client.ObjectKeyFromObject(svc)).To(Equal(client.ObjectKey{
								Name:      getServiceNameFor(instance),
								Namespace: instance.Namespace,
							}))
							Expect(client.ObjectKeyFromObject(cm)).To(Equal(client.ObjectKey{
								Name:      getConfigMapNameFor(instance),
								Namespace: instance.Namespace,
							}))

							Expect(svc).To(PointTo(MatchFields(IgnoreExtras, Fields{
								"ObjectMeta": MatchFields(IgnoreExtras, Fields{
									"OwnerReferences": ContainElement(MatchFields(IgnoreExtras, Fields{
										"UID": Equal(instance.UID),
									})),
								}),
							})))

							Expect(cm).To(PointTo(MatchFields(IgnoreExtras, Fields{
								"ObjectMeta": MatchFields(IgnoreExtras, Fields{
									"OwnerReferences": ContainElement(MatchFields(IgnoreExtras, Fields{
										"UID": Equal(instance.UID),
									})),
								}),
							})))
						})

						Describe("statefulset success", func() {
							var (
								sts                                 *appsv1.StatefulSet
								describeStatefulSetReady            func(doMarkStatefulSetAsReady func())
								describeStatefulSetReadyAndNotReady func(doMarkStatefulSetAsReady func(), checkPVC bool)
								cj                                  *batchv1beta1.CronJob
							)

							describeStatefulSetReady = func(doMarkStatefulSetAsReady func()) {
								Describe("when statefulset is ready", func() {
									BeforeEach(func() {
										doMarkStatefulSetAsReady()
									})

									reconcileShouldSucceed()

									It("should mark etcd resource as ready", func() {
										Expect(instance.Status.Ready).To(PointTo(BeTrue()))
									})
								})
							}
							describeStatefulSetReadyAndNotReady = func(doMarkStatefulSetAsReady func(), checkPVC bool) {
								describeStatefulSetReady(doMarkStatefulSetAsReady)

								Describe("when statefulset is not ready", func() {
									var (
										pvcName string

										// Save original wait period so that it can be restored later
										oldDefaultInterval = DefaultInterval
										oldDefaultTimeout  = DefaultTimeout
									)

									BeforeEach(func() {
										// Shorten the wait period because statefulset is not going to be ready anyway
										DefaultInterval = 1 * time.Millisecond
										DefaultTimeout = 2 * DefaultInterval

										if checkPVC {
											// Create PVC
											pvcName := fmt.Sprintf("%s-%s-%d", volumeClaimTemplateName, sts.Name, 0)
											mockListPVC(cl, &corev1.PersistentVolumeClaim{
												ObjectMeta: metav1.ObjectMeta{
													Name:      pvcName,
													Namespace: sts.Namespace,
												},
												Spec: corev1.PersistentVolumeClaimSpec{
													AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
													Resources: corev1.ResourceRequirements{
														Requests: corev1.ResourceList{
															corev1.ResourceStorage: resource.MustParse("1Gi"),
														},
													},
												},
											}).AnyTimes()
										} else {
											mockListPVC(cl).AnyTimes()
										}
									})

									AfterEach(func() {
										// Restore original wait period
										DefaultInterval = oldDefaultInterval
										DefaultTimeout = oldDefaultTimeout

										Expect(instance.Status).To(MatchFields(IgnoreExtras, Fields{
											"Ready":     Not(PointTo(BeTrue())), // should not be ready
											"LastError": Not(BeNil()),           // should report error
										}))
									})

									if checkPVC {
										Describe("when PVCs are provisioned properly", func() {
											BeforeEach(func() {
												mockListEvents(cl).AnyTimes()
											})

											reconcileShouldFailAndRequeue()
										})

										Describe("when PVCs fail to provision", func() {
											var pvcMessage string

											BeforeEach(func() {
												// Create PVC warning Event
												pvcMessage = "Failed to provision volume"
												mockListEvents(cl, &corev1.Event{
													ObjectMeta: metav1.ObjectMeta{
														Name:      "pvc-event-1",
														Namespace: sts.Namespace,
													},
													InvolvedObject: corev1.ObjectReference{
														APIVersion: "v1",
														Kind:       "PersistentVolumeClaim",
														Name:       pvcName,
														Namespace:  sts.Namespace,
													},
													Type:    corev1.EventTypeWarning,
													Message: pvcMessage,
												}).AnyTimes()
											})

											reconcileShouldFailAndRequeue()

											It("should report error events in etcd resource status", func() {
												Expect(instance.Status.LastError).To(PointTo(ContainSubstring(pvcMessage)))
											})
										})
									} else {
										reconcileShouldFailAndRequeue()
									}
								})
							}

							AfterEach(func() {
								Expect(client.ObjectKeyFromObject(sts)).To(Equal(client.ObjectKeyFromObject(instance)))

								Expect(sts).To(PointTo(MatchFields(IgnoreExtras, Fields{
									"ObjectMeta": MatchFields(IgnoreExtras, Fields{
										"Annotations": MatchKeys(IgnoreExtras, Keys{
											common.GardenerOwnedBy:   Equal(client.ObjectKeyFromObject(instance).String()),
											common.GardenerOwnerType: Equal(strings.ToLower(etcdGVK.Kind)),
										}),
										"OwnerReferences": Not(ContainElement(MatchFields(IgnoreExtras, Fields{
											"UID": Equal(instance.UID),
										}))),
									}),
									"Spec": MatchFields(IgnoreExtras, Fields{
										"Replicas": PointTo(Equal(int32(instance.Spec.Replicas))),
									}),
								})))
							})

							Describe("without statefulset", func() {
								var (
									createStatefulSetCall          *gomock.Call
									setStatefulSetReadyAfterCreate = func() {
										createStatefulSetCall.Do(
											func(_ context.Context, _ *appsv1.StatefulSet) {
												setStatefulSetReady(sts)
											},
										)
									}
								)

								BeforeEach(func() {
									mockListGetAndUpdateStatefulSet(cl).Times(1)

									sts = &appsv1.StatefulSet{
										ObjectMeta: metav1.ObjectMeta{
											Name:      instance.Name,
											Namespace: instance.Namespace,
										},
									}

									createStatefulSetCall = cl.EXPECT().Create(gomock.Any(), gomock.AssignableToTypeOf(sts), gomock.Any()).DoAndReturn(
										func(_ context.Context, src *appsv1.StatefulSet) error {
											src.DeepCopyInto(sts)

											return nil
										},
									)
									mockListGetAndUpdateStatefulSet(cl, sts).AnyTimes().After(createStatefulSetCall)
								})

								AfterEach(func() {
									if instance.Status.Ready != nil && *instance.Status.Ready {
										Expect(instance.Status.ClusterSize).To(
											PointTo(Equal(int32(instance.Spec.Replicas))),
											"Status cluster size should be re-initialized in the bootstrap case",
										)
									}
								})

								Describe("with cronjob", func() {
									BeforeEach(func() {
										cj = &batchv1beta1.CronJob{
											ObjectMeta: metav1.ObjectMeta{
												Name:      getCronJobName(instance),
												Namespace: instance.Namespace,
											},
										}
										mockListGetAndUpdateCronJob(cl, cj).AnyTimes()

										cl.EXPECT().Delete(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(cj), HasSameNamespacedNameAs(cj)), gomock.Any()).DoAndReturn(
											func(_ context.Context, _ *batchv1beta1.CronJob, _ ...client.DeleteOption) error {
												cj.DeletionTimestamp = &metav1.Time{Time: time.Now()}
												return nil
											},
										).AnyTimes()
									})

									AfterEach(func() {
										Expect(cj.DeletionTimestamp).ToNot(BeNil(), "cronjob should be deleted")
										cj = nil
									})

									Describe("without backup compaction schedule", func() {
										describeStatefulSetReady(setStatefulSetReadyAfterCreate)
									})
								})

								Describe("without cronjob", func() {
									var describeGeneratedResourceValidation = func(
										spec string,
										generateEtcd func(string, string) *druidv1alpha1.Etcd,
										validate func(*appsv1.StatefulSet, *corev1.ConfigMap, *corev1.Service, *batchv1beta1.CronJob, *druidv1alpha1.Etcd),
									) {
										Describe(spec, func() {
											var tlsSecrets []*corev1.Secret

											BeforeEach(func() {
												generateEtcd(instance.Name, instance.Namespace).DeepCopyInto(instance)

												if instance.Spec.Etcd.TLS != nil {
													var tls = instance.Spec.Etcd.TLS
													for _, secretName := range []string{
														tls.ServerTLSSecretRef.Name,
														tls.ClientTLSSecretRef.Name,
														tls.TLSCASecretRef.Name,
													} {
														tlsSecrets = append(tlsSecrets, getSecret(secretName, instance.Namespace))
													}

													mockGetAndUpdateSecrets(cl, tlsSecrets...).AnyTimes()
												}
											})

											AfterEach(func() {
												validate(sts, cm, svc, cj, instance)

												if instance.Spec.Etcd.TLS != nil {
													for _, secret := range tlsSecrets {
														Expect(secret.Finalizers).To(ContainElement(FinalizerName), "TLS secret should have finalizer added")
													}
												}

												tlsSecrets = nil
											})

											describeStatefulSetReady(setStatefulSetReadyAfterCreate)
										})
									}

									BeforeEach(func() {
										cj = nil
										mockListGetAndUpdateCronJob(cl).Times(1).After(
											cl.EXPECT().Get(
												gomock.Any(),
												types.NamespacedName{
													Name:      getCronJobName(instance),
													Namespace: instance.Namespace,
												},
												gomock.AssignableToTypeOf(cj),
											).Return(errors.New("error")),
										)
									})

									Describe("without backup compaction schedule", func() {
										describeStatefulSetReadyAndNotReady(setStatefulSetReadyAfterCreate, true)

										Describe("validation of generated resources", func() {
											describeGeneratedResourceValidation("if fields are not set in etcd.Spec, the statefulset should reflect the spec changes", getEtcdWithDefault, validateEtcdWithDefaults)
											describeGeneratedResourceValidation("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", getEtcdWithTLS, validateEtcd)
											describeGeneratedResourceValidation("if the store is GCS, the statefulset should reflect the spec changes", getEtcdWithGCS, validateStoreGCP)
											describeGeneratedResourceValidation("if the store is S3, the statefulset should reflect the spec changes", getEtcdWithS3, validateStoreAWS)
											describeGeneratedResourceValidation("if the store is ABS, the statefulset should reflect the spec changes", getEtcdWithABS, validateStoreAzure)
											describeGeneratedResourceValidation("if the store is Swift, the statefulset should reflect the spec changes", getEtcdWithSwift, validateStoreOpenstack)
											describeGeneratedResourceValidation("if the store is OSS, the statefulset should reflect the spec changes", getEtcdWithOSS, validateStoreAlicloud)
										})
									})

									Describe("with backup compaction schedule", func() {
										BeforeEach(func() {
											cj = &batchv1beta1.CronJob{
												ObjectMeta: metav1.ObjectMeta{
													Name:      getCronJobName(instance),
													Namespace: instance.Namespace,
												},
											}
											mockListGetAndUpdateCronJob(cl, cj).AnyTimes().After(
												cl.EXPECT().Create(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(cj), HasSameNamespacedNameAs(cj)), gomock.Any()).DoAndReturn(
													func(_ context.Context, src *batchv1beta1.CronJob) error {
														src.DeepCopyInto(cj)
														return nil
													},
												),
											)
										})

										AfterEach(func() {
											Expect(cj).To(PointTo(MatchFields(IgnoreExtras, Fields{
												"ObjectMeta": MatchFields(IgnoreExtras, Fields{
													"OwnerReferences": ContainElement(MatchFields(IgnoreExtras, Fields{
														"UID": Equal(instance.UID),
													})),
												}),
											})))

											cj = nil
										})

										Describe("validation of generated resources", func() {
											describeGeneratedResourceValidation("if fields are set in etcd.Spec and TLS enabled, the resources should reflect the spec changes", getEtcdWithCmpctScheduleTLS, validateEtcdWithCronjob)
											describeGeneratedResourceValidation("if the store is GCS, the statefulset and cronjob should reflect the spec changes", getEtcdWithCmpctScheduleGCS, validateStoreGCPWithCronjob)
											describeGeneratedResourceValidation("if the store is S3, the statefulset and cronjob should reflect the spec changes", getEtcdWithCmpctScheduleS3, validateStoreAWSWithCronjob)
											describeGeneratedResourceValidation("if the store is ABS, the statefulset and cronjob should reflect the spec changes", getEtcdWithCmpctScheduleABS, validateStoreAzureWithCronjob)
											describeGeneratedResourceValidation("if the store is Swift, the statefulset and cronjob should reflect the spec changes", getEtcdWithCmpctScheduleSwift, validateStoreOpenstackWithCronjob)
											describeGeneratedResourceValidation("if the store is OSS, the statefulset and cronjob should reflect the spec changes", getEtcdWithCmpctScheduleOSS, validateStoreAlicloudWithCronjob)
										})
									})
								})
							})

							Describe("with a statefulset", func() {
								var (
									patchStatefulSetCall          *gomock.Call
									describeWithAndWithoutPods    func()
									setStatefulSetReadyAfterPatch = func() {
										patchStatefulSetCall.Do(
											func(_ context.Context, _ *appsv1.StatefulSet, _ client.Patch) {
												setStatefulSetReady(sts)
											},
										)
									}
								)

								BeforeEach(func() {
									sts = createStatefulset(instance.Name, "default", instance.Spec.Labels)
									mockListGetAndUpdateStatefulSet(cl, sts).AnyTimes()

									patchStatefulSetCall = cl.EXPECT().Patch(gomock.Any(), gomock.AssignableToTypeOf(sts), gomock.Any()).DoAndReturn(
										func(_ context.Context, src *appsv1.StatefulSet, _ client.Patch) error {
											src.DeepCopyInto(sts)
											return nil
										},
									).AnyTimes()

									//Without cronjob
									mockListGetAndUpdateCronJob(cl).Times(1).After(
										cl.EXPECT().Get(
											gomock.Any(),
											types.NamespacedName{
												Name:      getCronJobName(instance),
												Namespace: instance.Namespace,
											},
											gomock.AssignableToTypeOf(cj),
										).Return(errors.New("error")),
									)
								})

								describeWithAndWithoutPods = func() {
									Describe("when there are no pods", func() {
										BeforeEach(func() {
											mockListPods(cl)
										})

										describeStatefulSetReadyAndNotReady(func() {
											patchStatefulSetCall.Do(
												func(_ context.Context, _ *appsv1.StatefulSet, _ client.Patch) {
													setStatefulSetReady(sts)
												},
											)
										}, false)
									})

									Describe("when there are enough pods", func() {
										var pods []*corev1.Pod

										BeforeEach(func() {
											pods = append(pods, createPod(fmt.Sprintf("%s-0", instance.Name), instance.Namespace, instance.Spec.Labels))
											mockListPods(cl, pods...)
										})

										AfterEach(func() {
											pods = nil
										})

										Describe("when pods are not in CrashloopBackoff", func() {
											describeStatefulSetReadyAndNotReady(setStatefulSetReadyAfterPatch, false)
										})

										Describe("when pods are in CrashloopBackoff", func() {
											BeforeEach(func() {
												// In the multi-node scenario, this can be tweaked to only crash a subset of pods
												for _, pod := range pods {
													pod.Status.ContainerStatuses = []corev1.ContainerStatus{
														{
															Name: "Container-0",
															State: corev1.ContainerState{
																Waiting: &corev1.ContainerStateWaiting{
																	Reason:  "CrashLoopBackOff",
																	Message: "Container is in CrashLoopBackOff.",
																},
															},
														},
													}
												}

												cl.EXPECT().Delete(gomock.Any(), gomock.AssignableToTypeOf(&corev1.Pod{}), gomock.Any()).DoAndReturn(
													func(_ context.Context, src *corev1.Pod, _ ...client.DeleteOption) error {
														for _, p := range pods {
															if p.Name == src.Name {
																p.DeletionTimestamp = &metav1.Time{Time: time.Now()}
															}
														}
														return nil
													},
												).AnyTimes()
											})

											AfterEach(func() {
												for _, p := range pods {
													Expect(p.DeletionTimestamp).ToNot(BeNil(), "crashing pod should be deleted")
												}
											})

											describeStatefulSetReadyAndNotReady(setStatefulSetReadyAfterPatch, false)
										})
									})
								}

								Describe("if the statefulset is not claimed in anyway", func() {
									describeWithAndWithoutPods()
								})

								Describe("if the statefulset has been claimed", func() {
									Describe("if the statefulset has owner references", func() {
										BeforeEach(func() {
											sts.OwnerReferences = []metav1.OwnerReference{
												{
													APIVersion:         etcdGVK.GroupVersion().String(),
													Kind:               strings.ToLower(etcdGVK.Kind),
													Name:               instance.Name,
													UID:                instance.UID,
													Controller:         pointer.BoolPtr(true),
													BlockOwnerDeletion: pointer.BoolPtr(true),
												},
											}
										})

										describeWithAndWithoutPods()
									})

									Describe("if the statefulset has owner annotations", func() {
										BeforeEach(func() {
											sts.Annotations = map[string]string{
												common.GardenerOwnedBy:   client.ObjectKeyFromObject(instance).String(),
												common.GardenerOwnerType: strings.ToLower(etcdGVK.Kind),
											}
										})

										describeWithAndWithoutPods()
									})
								})
							})
						})
					})
				})
			})
		})
	})
})

func validateEtcdWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateEtcd(s, cm, svc, cj, instance)

	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(getCronJobName(instance)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchKeys(IgnoreExtras, Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"ConcurrencyPolicy": Equal(batchv1beta1.ForbidConcurrent),
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"BackoffLimit": PointTo(Equal(int32(0))),
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"RestartPolicy": Equal(corev1.RestartPolicyNever),
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--data-dir=/var/etcd/data":                                                                                 Equal("--data-dir=/var/etcd/data"),
										"--snapstore-temp-directory=/var/etcd/data/tmp":                                                             Equal("--snapstore-temp-directory=/var/etcd/data/tmp"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                   Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--storage-provider", store):                                                           Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container):                            Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
										fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                      Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
										fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
										fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):       Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
									}),
									"Ports": ConsistOf([]corev1.ContainerPort{
										corev1.ContainerPort{
											Name:          "server",
											Protocol:      corev1.ProtocolTCP,
											HostPort:      0,
											ContainerPort: backupPort,
										},
									}),
									//"Image":           Equal(*instance.Spec.Backup.Image),
									"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
									"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
										"etcd-config-file": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-config-file"),
											"MountPath": Equal("/var/etcd/config/"),
										}),
										"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-workspace-dir"),
											"MountPath": Equal("/var/etcd/data"),
										}),
									}),
									"Env": MatchElements(envIterator, IgnoreExtras, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
									}),
								}),
							}),
							"Volumes": MatchAllElements(volumeIterator, Elements{
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("etcd-config-file"),
									"VolumeSource": MatchFields(IgnoreExtras, Fields{
										"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
											}),
											"DefaultMode": PointTo(Equal(int32(0644))),
											"Items": MatchAllElements(keyIterator, Elements{
												"etcd.conf.yaml": MatchFields(IgnoreExtras, Fields{
													"Key":  Equal("etcd.conf.yaml"),
													"Path": Equal("etcd.conf.yaml"),
												}),
											}),
										})),
									}),
								}),
								"etcd-workspace-dir": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("etcd-workspace-dir"),
									"VolumeSource": MatchFields(IgnoreExtras, Fields{
										"HostPath": BeNil(),
										"EmptyDir": PointTo(MatchFields(IgnoreExtras, Fields{
											"SizeLimit": BeNil(),
										})),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreGCPWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreGCP(s, cm, svc, cj, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=GCS": Equal("--storage-provider=GCS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
										"etcd-backup": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-backup"),
											"MountPath": Equal("/root/.gcp/"),
										}),
										"etcd-config-file": MatchFields(IgnoreExtras, Fields{
											"Name":      Equal("etcd-config-file"),
											"MountPath": Equal("/var/etcd/config/"),
										}),
									}),
									"Env": MatchAllElements(envIterator, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
										"GOOGLE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("GOOGLE_APPLICATION_CREDENTIALS"),
											"Value": Equal("/root/.gcp/serviceaccount.json"),
										}),
									}),
								}),
							}),
							"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("etcd-backup"),
									"VolumeSource": MatchFields(IgnoreExtras, Fields{
										"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
											"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
										})),
									}),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("etcd-config-file"),
									"VolumeSource": MatchFields(IgnoreExtras, Fields{
										"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
											}),
											"DefaultMode": PointTo(Equal(int32(0644))),
											"Items": MatchAllElements(keyIterator, Elements{
												"etcd.conf.yaml": MatchFields(IgnoreExtras, Fields{
													"Key":  Equal("etcd.conf.yaml"),
													"Path": Equal("etcd.conf.yaml"),
												}),
											}),
										})),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAWSWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAWS(s, cm, svc, cj, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=S3": Equal("--storage-provider=S3"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"Env": MatchAllElements(envIterator, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
										"AWS_REGION": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("AWS_REGION"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("region"),
												})),
											})),
										}),
										"AWS_SECRET_ACCESS_KEY": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("AWS_SECRET_ACCESS_KEY"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("secretAccessKey"),
												})),
											})),
										}),
										"AWS_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("AWS_ACCESS_KEY_ID"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("accessKeyID"),
												})),
											})),
										}),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAzureWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAzure(s, cm, svc, cj, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=ABS": Equal("--storage-provider=ABS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"Env": MatchAllElements(envIterator, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
										"STORAGE_ACCOUNT": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("STORAGE_ACCOUNT"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("storageAccount"),
												})),
											})),
										}),
										"STORAGE_KEY": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("STORAGE_KEY"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("storageKey"),
												})),
											})),
										}),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreOpenstackWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreOpenstack(s, cm, svc, cj, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=Swift": Equal("--storage-provider=Swift"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"Env": MatchAllElements(envIterator, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
										"OS_AUTH_URL": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("OS_AUTH_URL"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("authURL"),
												})),
											})),
										}),
										"OS_USERNAME": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("OS_USERNAME"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("username"),
												})),
											})),
										}),
										"OS_TENANT_NAME": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("OS_TENANT_NAME"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("tenantName"),
												})),
											})),
										}),
										"OS_PASSWORD": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("OS_PASSWORD"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("password"),
												})),
											})),
										}),
										"OS_DOMAIN_NAME": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("OS_DOMAIN_NAME"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("domainName"),
												})),
											})),
										}),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAlicloudWithCronjob(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, cj *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	validateStoreAlicloud(s, cm, svc, cj, instance)

	Expect(*cj).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"JobTemplate": MatchFields(IgnoreExtras, Fields{
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Template": MatchFields(IgnoreExtras, Fields{
						"Spec": MatchFields(IgnoreExtras, Fields{
							"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
								"compact-backup": MatchFields(IgnoreExtras, Fields{
									"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
										"--storage-provider=OSS": Equal("--storage-provider=OSS"),
										fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):        Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
										fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container): Equal(fmt.Sprintf("%s=%s", "--store-container", *instance.Spec.Backup.Store.Container)),
									}),
									"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
									"Env": MatchAllElements(envIterator, Elements{
										"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
											"Name":  Equal("STORAGE_CONTAINER"),
											"Value": Equal(*instance.Spec.Backup.Store.Container),
										}),
										"ALICLOUD_ENDPOINT": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("ALICLOUD_ENDPOINT"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("storageEndpoint"),
												})),
											})),
										}),
										"ALICLOUD_ACCESS_KEY_SECRET": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("ALICLOUD_ACCESS_KEY_SECRET"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("accessKeySecret"),
												})),
											})),
										}),
										"ALICLOUD_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
											"Name": Equal("ALICLOUD_ACCESS_KEY_ID"),
											"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
												"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
													"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
														"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
													}),
													"Key": Equal("accessKeyID"),
												})),
											})),
										}),
									}),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateEtcdWithDefaults(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	// Validate Quota
	configYML := cm.Data[etcdConfig]
	config := map[string]string{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())
	Expect(instance.Spec.Etcd.Quota).To(BeNil())
	Expect(config).To(HaveKeyWithValue(quotaKey, fmt.Sprintf("%d", int64(quota.Value()))))

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).To(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, string(druidv1alpha1.Basic)))

	// Validate DefragmentationSchedule *string
	Expect(instance.Spec.Etcd.DefragmentationSchedule).To(BeNil())

	// Validate ServerPort and ClientPort
	Expect(instance.Spec.Etcd.ServerPort).To(BeNil())
	Expect(instance.Spec.Etcd.ClientPort).To(BeNil())

	Expect(instance.Spec.Etcd.Image).To(BeNil())
	imageVector, err := imagevector.ReadGlobalImageVectorWithEnvOverride(getImageYAMLPath())
	Expect(err).NotTo(HaveOccurred())
	images, err := imagevector.FindImages(imageVector, imageNames)
	Expect(err).NotTo(HaveOccurred())

	// Validate Resources
	// resources:
	//	  limits:
	//		cpu: 100m
	//		memory: 512Gi
	//	  requests:
	//		cpu: 50m
	//		memory: 128Mi
	Expect(instance.Spec.Etcd.Resources).To(BeNil())

	// Validate TLS. Ensure that enableTLS flag is not triggered in the go-template
	Expect(instance.Spec.Etcd.TLS).To(BeNil())

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                      Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(string(druidv1alpha1.Basic)),
		"snapshot-count":            Equal("75000"),
		"enable-v2":                 Equal("false"),
		"quota-backend-bytes":       Equal("8589934592"),
		"listen-client-urls":        Equal(fmt.Sprintf("http://0.0.0.0:%d", clientPort)),
		"advertise-client-urls":     Equal(fmt.Sprintf("http://0.0.0.0:%d", clientPort)),
		"initial-cluster-token":     Equal("initial"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(druidv1alpha1.Periodic)),
		"auto-compaction-retention": Equal(DefaultAutoCompactionRetention),
	}))

	Expect(*svc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector": MatchKeys(IgnoreExtras, Keys{
				"instance": Equal(instance.Name),
				"name":     Equal("etcd"),
			}),
			"Ports": MatchElements(servicePortIterator, IgnoreExtras, Elements{
				"client": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("client"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(clientPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(clientPort),
					}),
				}),
				"server": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("server"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(serverPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(serverPort),
					}),
				}),
				"backuprestore": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("backuprestore"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(backupPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(backupPort),
					}),
				}),
			}),
		}),
	}))

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.Name),
			"Namespace": Equal(instance.Namespace),
			"Annotations": MatchAllKeys(Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", instance.Namespace, instance.Name)),
				"gardener.cloud/owner-type": Equal("etcd"),
				"app":                       Equal("etcd-statefulset"),
				"role":                      Equal("test"),
				"instance":                  Equal(instance.Name),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Replicas":    PointTo(Equal(int32(instance.Spec.Replicas))),
			"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
				"MatchLabels": MatchAllKeys(Keys{
					"name":     Equal("etcd"),
					"instance": Equal(instance.Name),
				}),
			})),
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Annotations": MatchKeys(IgnoreExtras, Keys{
						"app":      Equal("etcd-statefulset"),
						"role":     Equal("test"),
						"instance": Equal(instance.Name),
					}),
					"Labels": MatchAllKeys(Keys{
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(hostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(cmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(""),
					"Containers": MatchAllElements(containerIterator, Elements{
						common.Etcd: MatchFields(IgnoreExtras, Fields{
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: serverPort,
								},
								corev1.ContainerPort{
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: clientPort,
								},
							}),
							"Command": MatchAllElements(cmdIterator, Elements{
								"/var/etcd/bin/bootstrap.sh": Equal("/var/etcd/bin/bootstrap.sh"),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.Etcd].Repository, *images[common.Etcd].Tag)),
							"Resources": MatchFields(IgnoreExtras, Fields{
								"Requests": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(resource.MustParse("50m")),
									corev1.ResourceMemory: Equal(resource.MustParse("128Mi")),
								}),
								"Limits": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(resource.MustParse("100m")),
									corev1.ResourceMemory: Equal(resource.MustParse("512Gi")),
								}),
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"HTTPGet": PointTo(MatchFields(IgnoreExtras, Fields{
										"Path": Equal("/healthz"),
										"Port": MatchFields(IgnoreExtras, Fields{
											"IntVal": Equal(int32(8080)),
										}),
										"Scheme": Or(BeEmpty(), Equal(corev1.URISchemeHTTP)),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"LivenessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
										"Command": MatchAllElements(cmdIterator, Elements{
											"/bin/sh":       Equal("/bin/sh"),
											"-ec":           Equal("-ec"),
											"ETCDCTL_API=3": Equal("ETCDCTL_API=3"),
											"etcdctl":       Equal("etcdctl"),
											fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort): Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
											"get": Equal("get"),
											"foo": Equal("foo"),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data/"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchAllElements(cmdIterator, Elements{
								"etcdbrctl":                                      Equal("etcdbrctl"),
								"server":                                         Equal("server"),
								"--data-dir=/var/etcd/data/new.etcd":             Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=true":                      Equal("--insecure-transport=true"),
								"--insecure-skip-tls-verify=true":                Equal("--insecure-skip-tls-verify=true"),
								"--etcd-connection-timeout=5m":                   Equal("--etcd-connection-timeout=5m"),
								"--snapstore-temp-directory=/var/etcd/data/temp": Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value()):                 Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", deltaSnapShotMemLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased): Equal(fmt.Sprintf("--garbage-collection-policy=%s", druidv1alpha1.GarbageCollectionPolicyLimitBased)),
								fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort):                       Equal(fmt.Sprintf("--endpoints=http://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value())):                            Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(quota.Value()))),
								fmt.Sprintf("--max-backups=%d", maxBackups):                                                    Equal(fmt.Sprintf("--max-backups=%d", maxBackups)),
								fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic):                               Equal(fmt.Sprintf("--auto-compaction-mode=%s", druidv1alpha1.Periodic)),
								fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention):                  Equal(fmt.Sprintf("--auto-compaction-retention=%s", DefaultAutoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "8m"):                                          Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", "8m")),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "8m"):                                            Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", "8m")),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(fmt.Sprintf("%s:%s", images[common.BackupRestore].Repository, *images[common.BackupRestore].Tag)),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								instance.Name: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(instance.Name),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(""),
								}),
							}),
						}),
					}),
					"Volumes": MatchAllElements(volumeIterator, Elements{
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(0644))),
									"Items": MatchAllElements(keyIterator, Elements{
										"etcd.conf.yaml": MatchFields(IgnoreExtras, Fields{
											"Key":  Equal("etcd.conf.yaml"),
											"Path": Equal("etcd.conf.yaml"),
										}),
									}),
								})),
							}),
						}),
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(pvcIterator, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(instance.Name),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"AccessModes": MatchAllElements(accessModeIterator, Elements{
							"ReadWriteOnce": Equal(corev1.ReadWriteOnce),
						}),
						"Resources": MatchFields(IgnoreExtras, Fields{
							"Requests": MatchKeys(IgnoreExtras, Keys{
								corev1.ResourceStorage: Equal(defaultStorageCapacity),
							}),
						}),
					}),
				}),
			}),
		}),
	}))

}

func validateEtcd(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {

	// Validate Quota
	configYML := cm.Data[etcdConfig]
	config := map[string]interface{}{}
	err := yaml.Unmarshal([]byte(configYML), &config)
	Expect(err).NotTo(HaveOccurred())
	Expect(instance.Spec.Etcd.Quota).NotTo(BeNil())
	Expect(config).To(HaveKeyWithValue(quotaKey, float64(instance.Spec.Etcd.Quota.Value())))

	// Validate Metrics MetricsLevel
	Expect(instance.Spec.Etcd.Metrics).NotTo(BeNil())
	Expect(config).To(HaveKeyWithValue(metricsKey, string(*instance.Spec.Etcd.Metrics)))

	// Validate DefragmentationSchedule *string
	Expect(instance.Spec.Etcd.DefragmentationSchedule).NotTo(BeNil())

	// Validate Image
	Expect(instance.Spec.Etcd.Image).NotTo(BeNil())

	// Validate Resources
	Expect(instance.Spec.Etcd.Resources).NotTo(BeNil())

	store, err := utils.StorageProviderFromInfraProvider(instance.Spec.Backup.Store.Provider)
	Expect(err).NotTo(HaveOccurred())

	Expect(*cm).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
	}))

	Expect(config).To(MatchKeys(IgnoreExtras, Keys{
		"name":                      Equal(fmt.Sprintf("etcd-%s", instance.UID[:6])),
		"data-dir":                  Equal("/var/etcd/data/new.etcd"),
		"metrics":                   Equal(string(*instance.Spec.Etcd.Metrics)),
		"snapshot-count":            Equal(float64(75000)),
		"enable-v2":                 Equal(false),
		"quota-backend-bytes":       Equal(float64(instance.Spec.Etcd.Quota.Value())),
		"listen-client-urls":        Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"advertise-client-urls":     Equal(fmt.Sprintf("https://0.0.0.0:%d", *instance.Spec.Etcd.ClientPort)),
		"initial-cluster-token":     Equal("initial"),
		"initial-cluster-state":     Equal("new"),
		"auto-compaction-mode":      Equal(string(*instance.Spec.Common.AutoCompactionMode)),
		"auto-compaction-retention": Equal(*instance.Spec.Common.AutoCompactionRetention),

		"client-transport-security": MatchKeys(IgnoreExtras, Keys{
			"cert-file":        Equal("/var/etcd/ssl/server/tls.crt"),
			"key-file":         Equal("/var/etcd/ssl/server/tls.key"),
			"client-cert-auth": Equal(true),
			"trusted-ca-file":  Equal("/var/etcd/ssl/ca/ca.crt"),
			"auto-tls":         Equal(false),
		}),
	}))

	Expect(*svc).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Namespace": Equal(instance.Namespace),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
			"OwnerReferences": MatchElements(ownerRefIterator, IgnoreExtras, Elements{
				instance.Name: MatchFields(IgnoreExtras, Fields{
					"APIVersion":         Equal("druid.gardener.cloud/v1alpha1"),
					"Kind":               Equal("Etcd"),
					"Name":               Equal(instance.Name),
					"UID":                Equal(instance.UID),
					"Controller":         PointTo(Equal(true)),
					"BlockOwnerDeletion": PointTo(Equal(true)),
				}),
			}),
		}),
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Type":            Equal(corev1.ServiceTypeClusterIP),
			"SessionAffinity": Equal(corev1.ServiceAffinityNone),
			"Selector": MatchKeys(IgnoreExtras, Keys{
				"instance": Equal(instance.Name),
				"name":     Equal("etcd"),
			}),
			"Ports": MatchElements(servicePortIterator, IgnoreExtras, Elements{
				"client": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("client"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Etcd.ClientPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Etcd.ClientPort),
					}),
				}),
				"server": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("server"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Etcd.ServerPort),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Etcd.ServerPort),
					}),
				}),
				"backuprestore": MatchFields(IgnoreExtras, Fields{
					"Name":     Equal("backuprestore"),
					"Protocol": Equal(corev1.ProtocolTCP),
					"Port":     Equal(*instance.Spec.Backup.Port),
					"TargetPort": MatchFields(IgnoreExtras, Fields{
						"IntVal": Equal(*instance.Spec.Backup.Port),
					}),
				}),
			}),
		}),
	}))

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"ObjectMeta": MatchFields(IgnoreExtras, Fields{
			"Name":      Equal(instance.Name),
			"Namespace": Equal(instance.Namespace),
			"Annotations": MatchAllKeys(Keys{
				"gardener.cloud/owned-by":   Equal(fmt.Sprintf("%s/%s", instance.Namespace, instance.Name)),
				"gardener.cloud/owner-type": Equal("etcd"),
				"app":                       Equal("etcd-statefulset"),
				"role":                      Equal("test"),
				"instance":                  Equal(instance.Name),
			}),
			"Labels": MatchAllKeys(Keys{
				"name":     Equal("etcd"),
				"instance": Equal(instance.Name),
			}),
		}),

		"Spec": MatchFields(IgnoreExtras, Fields{
			"UpdateStrategy": MatchFields(IgnoreExtras, Fields{
				"Type": Equal(appsv1.RollingUpdateStatefulSetStrategyType),
			}),
			"ServiceName": Equal(fmt.Sprintf("%s-client", instance.Name)),
			"Replicas":    PointTo(Equal(int32(instance.Spec.Replicas))),
			"Selector": PointTo(MatchFields(IgnoreExtras, Fields{
				"MatchLabels": MatchAllKeys(Keys{
					"name":     Equal("etcd"),
					"instance": Equal(instance.Name),
				}),
			})),
			"Template": MatchFields(IgnoreExtras, Fields{
				"ObjectMeta": MatchFields(IgnoreExtras, Fields{
					"Annotations": MatchKeys(IgnoreExtras, Keys{
						"app":      Equal("etcd-statefulset"),
						"role":     Equal("test"),
						"instance": Equal(instance.Name),
					}),
					"Labels": MatchAllKeys(Keys{
						"name":     Equal("etcd"),
						"instance": Equal(instance.Name),
					}),
				}),
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"HostAliases": MatchAllElements(hostAliasIterator, Elements{
						"127.0.0.1": MatchFields(IgnoreExtras, Fields{
							"IP": Equal("127.0.0.1"),
							"Hostnames": MatchAllElements(cmdIterator, Elements{
								fmt.Sprintf("%s-local", instance.Name): Equal(fmt.Sprintf("%s-local", instance.Name)),
							}),
						}),
					}),
					"PriorityClassName": Equal(*instance.Spec.PriorityClassName),
					"Containers": MatchAllElements(containerIterator, Elements{
						common.Etcd: MatchFields(IgnoreExtras, Fields{
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *instance.Spec.Etcd.ServerPort,
								},
								corev1.ContainerPort{
									Name:          "client",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: *instance.Spec.Etcd.ClientPort,
								},
							}),
							"Command": MatchAllElements(cmdIterator, Elements{
								"/var/etcd/bin/bootstrap.sh": Equal("/var/etcd/bin/bootstrap.sh"),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Image":           Equal(*instance.Spec.Etcd.Image),
							"Resources": MatchFields(IgnoreExtras, Fields{
								"Requests": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(instance.Spec.Etcd.Resources.Requests[corev1.ResourceCPU]),
									corev1.ResourceMemory: Equal(instance.Spec.Etcd.Resources.Requests[corev1.ResourceMemory]),
								}),
								"Limits": MatchKeys(IgnoreExtras, Keys{
									corev1.ResourceCPU:    Equal(instance.Spec.Etcd.Resources.Limits[corev1.ResourceCPU]),
									corev1.ResourceMemory: Equal(instance.Spec.Etcd.Resources.Limits[corev1.ResourceMemory]),
								}),
							}),
							"ReadinessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"HTTPGet": PointTo(MatchFields(IgnoreExtras, Fields{
										"Path": Equal("/healthz"),
										"Port": MatchFields(IgnoreExtras, Fields{
											"IntVal": Equal(int32(8080)),
										}),
										"Scheme": Equal(corev1.URISchemeHTTPS),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"LivenessProbe": PointTo(MatchFields(IgnoreExtras, Fields{
								"Handler": MatchFields(IgnoreExtras, Fields{
									"Exec": PointTo(MatchFields(IgnoreExtras, Fields{
										"Command": MatchAllElements(cmdIterator, Elements{
											"/bin/sh":                             Equal("/bin/sh"),
											"-ec":                                 Equal("-ec"),
											"ETCDCTL_API=3":                       Equal("ETCDCTL_API=3"),
											"etcdctl":                             Equal("etcdctl"),
											"--cert=/var/etcd/ssl/client/tls.crt": Equal("--cert=/var/etcd/ssl/client/tls.crt"),
											"--key=/var/etcd/ssl/client/tls.key":  Equal("--key=/var/etcd/ssl/client/tls.key"),
											"--cacert=/var/etcd/ssl/ca/ca.crt":    Equal("--cacert=/var/etcd/ssl/ca/ca.crt"),
											fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort): Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
											"get": Equal("get"),
											"foo": Equal("foo"),
										}),
									})),
								}),
								"InitialDelaySeconds": Equal(int32(15)),
								"PeriodSeconds":       Equal(int32(5)),
							})),
							"VolumeMounts": MatchAllElements(volumeMountIterator, Elements{
								*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(*instance.Spec.VolumeClaimTemplate),
									"MountPath": Equal("/var/etcd/data/"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
								"ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/ca"),
								}),
								"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/server"),
								}),
								"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-client-tls"),
									"MountPath": Equal("/var/etcd/ssl/client"),
								}),
							}),
						}),

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchAllElements(cmdIterator, Elements{
								"etcdbrctl":                                      Equal("etcdbrctl"),
								"server":                                         Equal("server"),
								"--cert=/var/etcd/ssl/client/tls.crt":            Equal("--cert=/var/etcd/ssl/client/tls.crt"),
								"--key=/var/etcd/ssl/client/tls.key":             Equal("--key=/var/etcd/ssl/client/tls.key"),
								"--cacert=/var/etcd/ssl/ca/ca.crt":               Equal("--cacert=/var/etcd/ssl/ca/ca.crt"),
								"--server-cert=/var/etcd/ssl/server/tls.crt":     Equal("--server-cert=/var/etcd/ssl/server/tls.crt"),
								"--server-key=/var/etcd/ssl/server/tls.key":      Equal("--server-key=/var/etcd/ssl/server/tls.key"),
								"--data-dir=/var/etcd/data/new.etcd":             Equal("--data-dir=/var/etcd/data/new.etcd"),
								"--insecure-transport=false":                     Equal("--insecure-transport=false"),
								"--insecure-skip-tls-verify=false":               Equal("--insecure-skip-tls-verify=false"),
								"--snapstore-temp-directory=/var/etcd/data/temp": Equal("--snapstore-temp-directory=/var/etcd/data/temp"),
								"--etcd-connection-timeout=5m":                   Equal("--etcd-connection-timeout=5m"),
								fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule):                           Equal(fmt.Sprintf("--defragmentation-schedule=%s", *instance.Spec.Etcd.DefragmentationSchedule)),
								fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule):                                            Equal(fmt.Sprintf("--schedule=%s", *instance.Spec.Backup.FullSnapshotSchedule)),
								fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy):                  Equal(fmt.Sprintf("%s=%s", "--garbage-collection-policy", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("%s=%s", "--storage-provider", store):                                                                   Equal(fmt.Sprintf("%s=%s", "--storage-provider", store)),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix):                                           Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
								fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value()):              Equal(fmt.Sprintf("--delta-snapshot-memory-limit=%d", instance.Spec.Backup.DeltaSnapshotMemoryLimit.Value())),
								fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy):                        Equal(fmt.Sprintf("--garbage-collection-policy=%s", *instance.Spec.Backup.GarbageCollectionPolicy)),
								fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort):                                           Equal(fmt.Sprintf("--endpoints=https://%s-local:%d", instance.Name, clientPort)),
								fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value())):                              Equal(fmt.Sprintf("--embedded-etcd-quota-bytes=%d", int64(instance.Spec.Etcd.Quota.Value()))),
								fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--delta-snapshot-period", instance.Spec.Backup.DeltaSnapshotPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String()): Equal(fmt.Sprintf("%s=%s", "--garbage-collection-period", instance.Spec.Backup.GarbageCollectionPeriod.Duration.String())),
								fmt.Sprintf("%s=%s", "--auto-compaction-mode", *instance.Spec.Common.AutoCompactionMode):                            Equal(fmt.Sprintf("%s=%s", "--auto-compaction-mode", autoCompactionMode)),
								fmt.Sprintf("%s=%s", "--auto-compaction-retention", *instance.Spec.Common.AutoCompactionRetention):                  Equal(fmt.Sprintf("%s=%s", "--auto-compaction-retention", autoCompactionRetention)),
								fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String()):         Equal(fmt.Sprintf("%s=%s", "--etcd-snapshot-timeout", instance.Spec.Backup.EtcdSnapshotTimeout.Duration.String())),
								fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String()):               Equal(fmt.Sprintf("%s=%s", "--etcd-defrag-timeout", instance.Spec.Etcd.EtcdDefragTimeout.Duration.String())),
							}),
							"Ports": ConsistOf([]corev1.ContainerPort{
								corev1.ContainerPort{
									Name:          "server",
									Protocol:      corev1.ProtocolTCP,
									HostPort:      0,
									ContainerPort: backupPort,
								},
							}),
							"Image":           Equal(*instance.Spec.Backup.Image),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
									"Name":      Equal(*instance.Spec.VolumeClaimTemplate),
									"MountPath": Equal("/var/etcd/data"),
								}),
								"etcd-config-file": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-config-file"),
									"MountPath": Equal("/var/etcd/config/"),
								}),
								"ca-etcd": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("ca-etcd"),
									"MountPath": Equal("/var/etcd/ssl/ca"),
								}),
								"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-server-tls"),
									"MountPath": Equal("/var/etcd/ssl/server"),
								}),
								"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-client-tls"),
									"MountPath": Equal("/var/etcd/ssl/client"),
								}),
							}),
							"Env": MatchElements(envIterator, IgnoreExtras, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
							}),
						}),
					}),
					"Volumes": MatchAllElements(volumeIterator, Elements{
						"etcd-config-file": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-config-file"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"ConfigMap": PointTo(MatchFields(IgnoreExtras, Fields{
									"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
										"Name": Equal(fmt.Sprintf("etcd-bootstrap-%s", string(instance.UID[:6]))),
									}),
									"DefaultMode": PointTo(Equal(int32(0644))),
									"Items": MatchAllElements(keyIterator, Elements{
										"etcd.conf.yaml": MatchFields(IgnoreExtras, Fields{
											"Key":  Equal("etcd.conf.yaml"),
											"Path": Equal("etcd.conf.yaml"),
										}),
									}),
								})),
							}),
						}),
						"etcd-server-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-server-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.ServerTLSSecretRef.Name),
								})),
							}),
						}),
						"etcd-client-tls": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-client-tls"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.ClientTLSSecretRef.Name),
								})),
							}),
						}),
						"ca-etcd": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("ca-etcd"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Etcd.TLS.TLSCASecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
			"VolumeClaimTemplates": MatchAllElements(pvcIterator, Elements{
				*instance.Spec.VolumeClaimTemplate: MatchFields(IgnoreExtras, Fields{
					"ObjectMeta": MatchFields(IgnoreExtras, Fields{
						"Name": Equal(*instance.Spec.VolumeClaimTemplate),
					}),
					"Spec": MatchFields(IgnoreExtras, Fields{
						"StorageClassName": PointTo(Equal(*instance.Spec.StorageClass)),
						"AccessModes": MatchAllElements(accessModeIterator, Elements{
							"ReadWriteOnce": Equal(corev1.ReadWriteOnce),
						}),
						"Resources": MatchFields(IgnoreExtras, Fields{
							"Requests": MatchKeys(IgnoreExtras, Keys{
								corev1.ResourceStorage: Equal(*instance.Spec.StorageCapacity),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreGCP(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {

	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=GCS": Equal("--storage-provider=GCS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"VolumeMounts": MatchElements(volumeMountIterator, IgnoreExtras, Elements{
								"etcd-backup": MatchFields(IgnoreExtras, Fields{
									"Name":      Equal("etcd-backup"),
									"MountPath": Equal("/root/.gcp/"),
								}),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"GOOGLE_APPLICATION_CREDENTIALS": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("GOOGLE_APPLICATION_CREDENTIALS"),
									"Value": Equal("/root/.gcp/serviceaccount.json"),
								}),
							}),
						}),
					}),
					"Volumes": MatchElements(volumeIterator, IgnoreExtras, Elements{
						"etcd-backup": MatchFields(IgnoreExtras, Fields{
							"Name": Equal("etcd-backup"),
							"VolumeSource": MatchFields(IgnoreExtras, Fields{
								"Secret": PointTo(MatchFields(IgnoreExtras, Fields{
									"SecretName": Equal(instance.Spec.Backup.Store.SecretRef.Name),
								})),
							}),
						}),
					}),
				}),
			}),
		}),
	}))

}

func validateStoreAzure(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=ABS": Equal("--storage-provider=ABS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"STORAGE_ACCOUNT": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("STORAGE_ACCOUNT"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageAccount"),
										})),
									})),
								}),
								"STORAGE_KEY": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("STORAGE_KEY"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageKey"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreOpenstack(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{
						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=Swift": Equal("--storage-provider=Swift"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"OS_AUTH_URL": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_AUTH_URL"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("authURL"),
										})),
									})),
								}),
								"OS_USERNAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_USERNAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("username"),
										})),
									})),
								}),
								"OS_TENANT_NAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_TENANT_NAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("tenantName"),
										})),
									})),
								}),
								"OS_PASSWORD": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_PASSWORD"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("password"),
										})),
									})),
								}),
								"OS_DOMAIN_NAME": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("OS_DOMAIN_NAME"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("domainName"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAlicloud(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=OSS": Equal("--storage-provider=OSS"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"ALICLOUD_ENDPOINT": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ENDPOINT"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("storageEndpoint"),
										})),
									})),
								}),
								"ALICLOUD_ACCESS_KEY_SECRET": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ACCESS_KEY_SECRET"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeySecret"),
										})),
									})),
								}),
								"ALICLOUD_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("ALICLOUD_ACCESS_KEY_ID"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeyID"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func validateStoreAWS(s *appsv1.StatefulSet, cm *corev1.ConfigMap, svc *corev1.Service, _ *batchv1beta1.CronJob, instance *druidv1alpha1.Etcd) {
	Expect(*s).To(MatchFields(IgnoreExtras, Fields{
		"Spec": MatchFields(IgnoreExtras, Fields{
			"Template": MatchFields(IgnoreExtras, Fields{
				//s.Spec.Template.Spec.HostAliases
				"Spec": MatchFields(IgnoreExtras, Fields{
					"Containers": MatchElements(containerIterator, IgnoreExtras, Elements{

						backupRestore: MatchFields(IgnoreExtras, Fields{
							"Command": MatchElements(cmdIterator, IgnoreExtras, Elements{
								"--storage-provider=S3": Equal("--storage-provider=S3"),
								fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix): Equal(fmt.Sprintf("%s=%s", "--store-prefix", instance.Spec.Backup.Store.Prefix)),
							}),
							"ImagePullPolicy": Equal(corev1.PullIfNotPresent),
							"Env": MatchAllElements(envIterator, Elements{
								"STORAGE_CONTAINER": MatchFields(IgnoreExtras, Fields{
									"Name":  Equal("STORAGE_CONTAINER"),
									"Value": Equal(*instance.Spec.Backup.Store.Container),
								}),
								"AWS_REGION": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_REGION"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("region"),
										})),
									})),
								}),
								"AWS_SECRET_ACCESS_KEY": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_SECRET_ACCESS_KEY"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("secretAccessKey"),
										})),
									})),
								}),
								"AWS_ACCESS_KEY_ID": MatchFields(IgnoreExtras, Fields{
									"Name": Equal("AWS_ACCESS_KEY_ID"),
									"ValueFrom": PointTo(MatchFields(IgnoreExtras, Fields{
										"SecretKeyRef": PointTo(MatchFields(IgnoreExtras, Fields{
											"LocalObjectReference": MatchFields(IgnoreExtras, Fields{
												"Name": Equal(instance.Spec.Backup.Store.SecretRef.Name),
											}),
											"Key": Equal("accessKeyID"),
										})),
									})),
								}),
							}),
						}),
					}),
				}),
			}),
		}),
	}))
}

func createStatefulset(name, namespace string, labels map[string]string) *appsv1.StatefulSet {
	var replicas int32 = 0
	ss := appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("%s-0", name),
					Namespace: namespace,
					Labels:    labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "etcd",
							Image: "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap",
						},
						{
							Name:  "backup-restore",
							Image: "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0",
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{},
			ServiceName:          "etcd-client",
			UpdateStrategy:       appsv1.StatefulSetUpdateStrategy{},
		},
	}
	return &ss
}

func createPod(name, namespace string, labels map[string]string) *corev1.Pod {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "etcd",
					Image: "eu.gcr.io/gardener-project/gardener/etcd:v3.4.13-bootstrap",
				},
				{
					Name:  "backup-restore",
					Image: "eu.gcr.io/gardener-project/gardener/etcdbrctl:v0.12.0",
				},
			},
		},
	}
	return &pod
}

func getEtcdWithCmpctScheduleTLS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleGCS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithGCS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleS3(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithS3(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleABS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithABS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleSwift(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithSwift(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithCmpctScheduleOSS(name, namespace string) *druidv1alpha1.Etcd {
	etcd := getEtcdWithOSS(name, namespace)
	etcd.Spec.Backup.BackupCompactionSchedule = &backupCompactionSchedule
	return etcd
}

func getEtcdWithGCS(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("gcp")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithABS(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("azure")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithS3(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("aws")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithSwift(name, namespace string) *druidv1alpha1.Etcd {
	provider := druidv1alpha1.StorageProvider("openstack")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithOSS(name, namespace string) *druidv1alpha1.Etcd {
	container := fmt.Sprintf("%s-container", name)
	provider := druidv1alpha1.StorageProvider("alicloud")
	etcd := getEtcdWithTLS(name, namespace)
	etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
		Container: &container,
		Prefix:    name,
		Provider:  &provider,
		SecretRef: &corev1.SecretReference{
			Name: "etcd-backup",
		},
	}
	return etcd
}

func getEtcdWithTLS(name, namespace string) *druidv1alpha1.Etcd {
	return getEtcd(name, namespace, true)
}

func getEtcdWithDefault(name, namespace string) *druidv1alpha1.Etcd {
	instance := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			UID:       types.UID("1234567890"),
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":      "etcd-statefulset",
				"role":     "test",
				"instance": name,
			},
			Labels: map[string]string{
				"name":     "etcd",
				"instance": name,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     "etcd",
					"instance": name,
				},
			},
			Replicas: 1,
			Backup:   druidv1alpha1.BackupSpec{},
			Etcd:     druidv1alpha1.EtcdConfig{},
			Common:   druidv1alpha1.SharedConfig{},
		},
	}
	return instance
}

func getEtcd(name, namespace string, tlsEnabled bool) *druidv1alpha1.Etcd {

	instance := &druidv1alpha1.Etcd{
		ObjectMeta: metav1.ObjectMeta{
			Name:       name,
			Namespace:  namespace,
			Generation: 1,
			UID:        types.UID("1234567890"),
		},
		Spec: druidv1alpha1.EtcdSpec{
			Annotations: map[string]string{
				"app":      "etcd-statefulset",
				"role":     "test",
				"instance": name,
			},
			Labels: map[string]string{
				"name":     "etcd",
				"instance": name,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name":     "etcd",
					"instance": name,
				},
			},
			Replicas:            1,
			StorageCapacity:     &storageCapacity,
			StorageClass:        &storageClass,
			PriorityClassName:   &priorityClassName,
			VolumeClaimTemplate: &volumeClaimTemplateName,
			Backup: druidv1alpha1.BackupSpec{
				Image:                    &imageBR,
				Port:                     &backupPort,
				FullSnapshotSchedule:     &snapshotSchedule,
				GarbageCollectionPolicy:  &garbageCollectionPolicy,
				GarbageCollectionPeriod:  &garbageCollectionPeriod,
				DeltaSnapshotPeriod:      &deltaSnapshotPeriod,
				DeltaSnapshotMemoryLimit: &deltaSnapShotMemLimit,
				EtcdSnapshotTimeout:      &etcdSnapshotTimeout,

				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("2Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("23m"),
						"memory": parseQuantity("128Mi"),
					},
				},
				Store: &druidv1alpha1.StoreSpec{
					SecretRef: &corev1.SecretReference{
						Name: "etcd-backup",
					},
					Container: &container,
					Provider:  &provider,
					Prefix:    prefix,
				},
			},
			Etcd: druidv1alpha1.EtcdConfig{
				Quota:                   &quota,
				Metrics:                 &metricsBasic,
				Image:                   &imageEtcd,
				DefragmentationSchedule: &defragSchedule,
				EtcdDefragTimeout:       &etcdDefragTimeout,
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						"cpu":    parseQuantity("2500m"),
						"memory": parseQuantity("4Gi"),
					},
					Requests: corev1.ResourceList{
						"cpu":    parseQuantity("500m"),
						"memory": parseQuantity("1000Mi"),
					},
				},
				ClientPort: &clientPort,
				ServerPort: &serverPort,
			},
			Common: druidv1alpha1.SharedConfig{
				AutoCompactionMode:      &autoCompactionMode,
				AutoCompactionRetention: &autoCompactionRetention,
			},
		},
	}

	if tlsEnabled {
		tlsConfig := &druidv1alpha1.TLSConfig{
			ClientTLSSecretRef: corev1.SecretReference{
				Name: "etcd-client-tls",
			},
			ServerTLSSecretRef: corev1.SecretReference{
				Name: "etcd-server-tls",
			},
			TLSCASecretRef: corev1.SecretReference{
				Name: "ca-etcd",
			},
		}
		instance.Spec.Etcd.TLS = tlsConfig
	}
	return instance
}

func parseQuantity(q string) resource.Quantity {
	val, _ := resource.ParseQuantity(q)
	return val
}

type calls []*gomock.Call

func (c calls) AnyTimes() calls {
	return c.Times(0)
}

func (c calls) Times(times int) calls {
	for _, call := range c {
		if times > 0 {
			call.Times(times)
		} else {
			call.AnyTimes()
		}
	}

	return c
}

func (c calls) After(preReq *gomock.Call) calls {
	if preReq == nil {
		return c
	}

	for _, call := range c {
		call.After(preReq)
	}

	return c
}

func getSecret(name, namespace string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"test": []byte("test"),
		},
	}
}

func mockGetAndUpdateSecrets(cl *mockclient.MockClient, secrets ...*corev1.Secret) calls {
	var (
		ret       []*gomock.Call
		expectGet = func(src *corev1.Secret) *gomock.Call {
			return cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(src), gomock.AssignableToTypeOf(src)).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, target *corev1.Secret) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
		expectUpdate = func(target *corev1.Secret) *gomock.Call {
			return cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(target), HasSameNamespacedNameAs(target))).DoAndReturn(
				func(_ context.Context, src *corev1.Secret) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
	)

	for _, secret := range secrets {
		ret = append(ret, expectGet(secret), expectUpdate(secret))
	}

	return calls(ret)
}

func mockGetAndUpdateEtcd(cl *mockclient.MockClient, sw *mockclient.MockStatusWriter, etcd *druidv1alpha1.Etcd) calls {
	var ret []*gomock.Call

	ret = append(
		ret,
		cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(etcd), gomock.AssignableToTypeOf(etcd)).DoAndReturn(
			func(_ context.Context, _ client.ObjectKey, target *druidv1alpha1.Etcd) error {
				etcd.DeepCopyInto(target)
				return nil
			},
		),
		cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(etcd), HasSameNamespacedNameAs(etcd)), gomock.Any()).DoAndReturn(
			func(_ context.Context, src *druidv1alpha1.Etcd) error {
				src.DeepCopyInto(etcd)
				return nil
			},
		),
		sw.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(etcd), HasSameNamespacedNameAs(etcd))).DoAndReturn(
			func(_ context.Context, src *druidv1alpha1.Etcd) error {
				src.Status.DeepCopyInto(&etcd.Status)
				return nil
			},
		),
	)

	return calls(ret)
}

func mockListGetAndUpdateStatefulSet(cl *mockclient.MockClient, ss ...*appsv1.StatefulSet) calls {
	var (
		ret       []*gomock.Call
		expectGet = func(src *appsv1.StatefulSet) *gomock.Call {
			return cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(src), gomock.AssignableToTypeOf(src)).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, target *appsv1.StatefulSet) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
		expectUpdate = func(target *appsv1.StatefulSet) *gomock.Call {
			return cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(target), HasSameNamespacedNameAs(target))).DoAndReturn(
				func(_ context.Context, src *appsv1.StatefulSet) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
	)

	for _, s := range ss {
		ret = append(ret, expectGet(s), expectUpdate(s))
	}

	return calls(append(
		ret,
		cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&appsv1.StatefulSetList{}), gomock.Any()).DoAndReturn(
			func(_ context.Context, target *appsv1.StatefulSetList, opts ...client.ListOption) error {
				var listOpts = &client.ListOptions{}
				for _, opt := range opts {
					opt.ApplyToList(listOpts)
				}

				for _, s := range ss {
					target.Items = append(target.Items, *s)
				}
				return nil
			},
		),
	))
}

func mockListGetAndUpdateService(cl *mockclient.MockClient, ss ...*corev1.Service) calls {
	var (
		ret       []*gomock.Call
		expectGet = func(src *corev1.Service) *gomock.Call {
			return cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(src), gomock.AssignableToTypeOf(src)).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, target *corev1.Service) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
		expectUpdate = func(target *corev1.Service) *gomock.Call {
			return cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(target), HasSameNamespacedNameAs(target))).DoAndReturn(
				func(_ context.Context, src *corev1.Service) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
	)

	for _, s := range ss {
		ret = append(ret, expectGet(s), expectUpdate(s))
	}

	return calls(append(
		ret,
		cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.ServiceList{}), gomock.Any()).DoAndReturn(
			func(_ context.Context, target *corev1.ServiceList, opts ...client.ListOption) error {
				var listOpts = &client.ListOptions{}
				for _, opt := range opts {
					opt.ApplyToList(listOpts)
				}

				for _, s := range ss {
					target.Items = append(target.Items, *s)
				}
				return nil
			},
		),
	))
}

func mockListGetAndUpdateConfigMap(cl *mockclient.MockClient, cms ...*corev1.ConfigMap) calls {
	var (
		ret       []*gomock.Call
		expectGet = func(src *corev1.ConfigMap) *gomock.Call {
			return cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(src), gomock.AssignableToTypeOf(src)).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, target *corev1.ConfigMap) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
		expectUpdate = func(target *corev1.ConfigMap) *gomock.Call {
			return cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(target), HasSameNamespacedNameAs(target))).DoAndReturn(
				func(_ context.Context, src *corev1.ConfigMap) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
	)

	for _, cm := range cms {
		ret = append(ret, expectGet(cm), expectUpdate(cm))
	}

	return calls(append(
		ret,
		cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.ConfigMapList{}), gomock.Any()).DoAndReturn(
			func(_ context.Context, target *corev1.ConfigMapList, opts ...client.ListOption) error {
				var listOpts = &client.ListOptions{}
				for _, opt := range opts {
					opt.ApplyToList(listOpts)
				}

				for _, cm := range cms {
					target.Items = append(target.Items, *cm)
				}
				return nil
			},
		),
	))
}

func mockListGetAndUpdateCronJob(cl *mockclient.MockClient, cjs ...*batchv1beta1.CronJob) calls {
	var (
		ret       []*gomock.Call
		expectGet = func(src *batchv1beta1.CronJob) *gomock.Call {
			return cl.EXPECT().Get(gomock.Any(), client.ObjectKeyFromObject(src), gomock.AssignableToTypeOf(src)).DoAndReturn(
				func(_ context.Context, _ client.ObjectKey, target *batchv1beta1.CronJob) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
		expectUpdate = func(target *batchv1beta1.CronJob) *gomock.Call {
			return cl.EXPECT().Update(gomock.Any(), gomock.All(gomock.AssignableToTypeOf(target), HasSameNamespacedNameAs(target))).DoAndReturn(
				func(_ context.Context, src *batchv1beta1.CronJob) error {
					src.DeepCopyInto(target)
					return nil
				},
			)
		}
	)

	for _, cj := range cjs {
		ret = append(ret, expectGet(cj), expectUpdate(cj))
	}

	return calls(append(
		ret,
		cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&batchv1beta1.CronJobList{}), gomock.Any()).DoAndReturn(
			func(_ context.Context, target *batchv1beta1.CronJobList, opts ...client.ListOption) error {
				var listOpts = &client.ListOptions{}
				for _, opt := range opts {
					opt.ApplyToList(listOpts)
				}

				for _, cj := range cjs {
					target.Items = append(target.Items, *cj)
				}
				return nil
			},
		),
	))
}

func mockListPVC(cl *mockclient.MockClient, pvcs ...*corev1.PersistentVolumeClaim) *gomock.Call {
	return cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PersistentVolumeClaimList{}), gomock.Any()).DoAndReturn(
		func(_ context.Context, target *corev1.PersistentVolumeClaimList, _ ...client.ListOption) error {
			for _, pvc := range pvcs {
				target.Items = append(target.Items, *pvc)
			}
			return nil
		},
	)
}

func mockListEvents(cl *mockclient.MockClient, events ...*corev1.Event) *gomock.Call {
	return cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.EventList{}), gomock.Any()).DoAndReturn(
		func(_ context.Context, target *corev1.EventList, _ ...client.ListOption) error {
			for _, event := range events {
				target.Items = append(target.Items, *event)
			}
			return nil
		},
	)
}

func mockListPods(cl *mockclient.MockClient, pods ...*corev1.Pod) *gomock.Call {
	return cl.EXPECT().List(gomock.Any(), gomock.AssignableToTypeOf(&corev1.PodList{}), gomock.Any()).DoAndReturn(
		func(_ context.Context, target *corev1.PodList, _ ...client.ListOption) error {
			for _, pod := range pods {
				target.Items = append(target.Items, *pod)
			}
			return nil
		},
	)
}

func createEtcdReconciler(cl *mockclient.MockClient) (*EtcdReconciler, error) {
	applier, err := kubernetes.NewApplierWithAllFields(cl, nil)
	if err != nil {
		return nil, err
	}

	return NewEtcdReconcilerWithAllFields(
		cl,
		scheme.Scheme,
		kubernetes.NewChartApplier(
			chartrenderer.New(engine.New(), nil),
			applier,
		),
		nil,
		false,
		nil,
		log.Log.WithName("etcd-controller"),
	).InitializeControllerWithImageVector()
}

func createEtcdCustodian(cl *mockclient.MockClient) *EtcdCustodian {
	return NewEtcdCustodianWithAllFields(
		cl,
		scheme.Scheme,
		log.Log.WithName("custodian-controller"),
		controllersconfig.EtcdCustodianController{
			EtcdMember: controllersconfig.EtcdMemberConfig{
				EtcdMemberNotReadyThreshold: 1 * time.Minute,
			},
		},
	)
}

type namespacedNameMatcher types.NamespacedName

func (m namespacedNameMatcher) Matches(x interface{}) bool {
	if k, ok := x.(types.NamespacedName); ok {
		return m.equal(types.NamespacedName(m), k)
	}
	if obj, ok := x.(client.Object); ok {
		return m.equal(types.NamespacedName(m), client.ObjectKeyFromObject(obj))
	}

	return false
}

func (m namespacedNameMatcher) equal(a, b types.NamespacedName) bool {
	return a.String() == b.String()
}

func (m namespacedNameMatcher) String() string {
	return fmt.Sprintf("matches key %q", types.NamespacedName(m).String())
}

func HasNamespacedName(key types.NamespacedName) gomock.Matcher {
	return namespacedNameMatcher(key)
}

func HasSameNamespacedNameAs(obj client.Object) gomock.Matcher {
	return HasNamespacedName(client.ObjectKeyFromObject(obj))
}

// WithWd sets the working directory and returns a function to revert to the previous one.
func WithWd(path string) func() {
	oldPath, err := os.Getwd()
	if err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	if err := os.Chdir(path); err != nil {
		Expect(err).NotTo(HaveOccurred())
	}

	return func() {
		if err := os.Chdir(oldPath); err != nil {
			Expect(err).NotTo(HaveOccurred())
		}
	}
}

func setStatefulSetReady(s *appsv1.StatefulSet) {
	s.Status.ObservedGeneration = s.Generation

	replicas := int32(1)
	if s.Spec.Replicas != nil {
		replicas = *s.Spec.Replicas
	}
	s.Status.Replicas = replicas
	s.Status.ReadyReplicas = replicas
}

var _ = Describe("buildPredicate", func() {
	var (
		etcd                              *druidv1alpha1.Etcd
		evalCreate                        = func(p predicate.Predicate, obj client.Object) bool { return p.Create(event.CreateEvent{Object: obj}) }
		evalDelete                        = func(p predicate.Predicate, obj client.Object) bool { return p.Delete(event.DeleteEvent{Object: obj}) }
		evalGeneric                       = func(p predicate.Predicate, obj client.Object) bool { return p.Generic(event.GenericEvent{Object: obj}) }
		evalUpdateWithoutGenerationChange = func(p predicate.Predicate, obj client.Object) bool {
			return p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: obj.DeepCopyObject().(client.Object)})
		}
		evalUpdateWithGenerationChange = func(p predicate.Predicate, obj client.Object) bool {
			objCopy := obj.DeepCopyObject().(client.Object)
			objCopy.SetGeneration(obj.GetGeneration() + 1)
			return p.Update(event.UpdateEvent{ObjectOld: obj, ObjectNew: objCopy})
		}
	)

	BeforeEach(func() {
		etcd = &druidv1alpha1.Etcd{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{},
			},
		}
	})

	DescribeTable(
		"with ignoreOperationAnnotation true",
		func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
			Expect(evalFn(buildPredicate(true), etcd)).To(Equal(expect))
		},
		Entry("Create should match", evalCreate, true),
		Entry("Delete should match", evalDelete, true),
		Entry("Generic should match", evalGeneric, true),
		Entry("Update without generation change should not match", evalUpdateWithoutGenerationChange, false),
		Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
	)

	Describe("with ignoreOperationAnnotation false", func() {
		DescribeTable(
			"without operation annotation or last error or deletion timestamp",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should not match", evalCreate, false),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should  not match", evalGeneric, false),
			Entry("Update without generation change should not match", evalUpdateWithoutGenerationChange, false),
			Entry("Update with generation change should not match", evalUpdateWithGenerationChange, false),
		)
		DescribeTable(
			"with operation annotation",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				etcd.Annotations[v1beta1constants.GardenerOperation] = v1beta1constants.GardenerOperationReconcile
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
		DescribeTable(
			"with last error",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				etcd.Status.LastError = pointer.StringPtr("error")
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
		DescribeTable(
			"with deletion timestamp",
			func(evalFn func(p predicate.Predicate, obj client.Object) bool, expect bool) {
				now := metav1.Time{Time: time.Now()}
				etcd.DeletionTimestamp = &now
				Expect(evalFn(buildPredicate(false), etcd)).To(Equal(expect))
			},
			Entry("Create should match", evalCreate, true),
			Entry("Delete should match", evalDelete, true),
			Entry("Generic should match", evalGeneric, true),
			Entry("Update without generation change should match", evalUpdateWithoutGenerationChange, true),
			Entry("Update with generation change should match", evalUpdateWithGenerationChange, true),
		)
	})
})
