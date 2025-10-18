// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondemandsnapshot

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	"github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/test/utils"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

func createEtcd(name, namespace string, backup bool, healthy bool, isTLSEnabled bool) *druidv1alpha1.Etcd {
	etcdBuilder := utils.EtcdBuilderWithoutDefaults(name, namespace).WithReplicas(1).WithReadyStatus()

	if isTLSEnabled {
		etcdBuilder = etcdBuilder.WithClientTLS()
	}

	etcd := etcdBuilder.Build()

	if backup {
		etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
			Container: ptr.To("test-container"),
			Prefix:    "test-prefix",
			Provider:  ptr.To(druidv1alpha1.StorageProvider("S3")),
		}
	}
	if !healthy {
		etcd.Status.Conditions = append(etcd.Status.Conditions, druidv1alpha1.Condition{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionFalse,
			Message: "etcd is not ready for testing purposes",
		})
	} else {
		etcd.Status.Conditions = append(etcd.Status.Conditions, druidv1alpha1.Condition{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionTrue,
			Message: "etcd is ready for testing purposes",
		})
	}
	return etcd
}

func createEtcdWithBackupTLS(name, namespace string, backup bool, healthy bool) *druidv1alpha1.Etcd {
	etcdBuilder := utils.EtcdBuilderWithoutDefaults(name, namespace).WithReplicas(1).WithReadyStatus()

	// Use backup TLS instead of client TLS
	etcdBuilder = etcdBuilder.WithBackupRestoreTLS()

	etcd := etcdBuilder.Build()

	if backup {
		etcd.Spec.Backup.Store = &druidv1alpha1.StoreSpec{
			Container: ptr.To("test-container"),
			Prefix:    "test-prefix",
			Provider:  ptr.To(druidv1alpha1.StorageProvider("S3")),
		}
	}
	if !healthy {
		etcd.Status.Conditions = append(etcd.Status.Conditions, druidv1alpha1.Condition{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionFalse,
			Message: "etcd is not ready for testing purposes",
		})
	} else {
		etcd.Status.Conditions = append(etcd.Status.Conditions, druidv1alpha1.Condition{
			Type:    druidv1alpha1.ConditionTypeReady,
			Status:  druidv1alpha1.ConditionTrue,
			Message: "etcd is ready for testing purposes",
		})
	}
	return etcd
}

func createEtcdOpsTask(config druidv1alpha1.OnDemandSnapshotConfig) *druidv1alpha1.EtcdOpsTask {
	etcdName := "test-etcd"
	return &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "test-namespace",
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			EtcdName: &etcdName,
			Config: druidv1alpha1.EtcdOpsTaskConfig{
				OnDemandSnapshot: &config,
			},
		},
	}
}

// TestOnDemandSnapshotTaskAdmit tests the Admit method of the OnDemandSnapshotTask handler.
func TestOnDemandSnapshotTaskAdmit(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		etcdObject     *druidv1alpha1.Etcd
		expectedResult handler.Result
		expectErr      bool
	}{
		{
			name:       "Etcd not found",
			etcdObject: nil,
			expectedResult: handler.Result{
				Description: "Failed to get etcd object",
				Error: &druiderr.DruidError{
					Code:      ErrGetEtcd,
					Operation: string(druidv1alpha1.OperationPhaseAdmit),
					Message:   "failed to get etcd object",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Backup not enabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", false, true, false),
			expectedResult: handler.Result{
				Description: "Backup is not enabled for etcd",
				Error: &druiderr.DruidError{
					Code:      ErrBackupNotEnabled,
					Operation: string(druidv1alpha1.OperationPhaseAdmit),
					Message:   "backup is not enabled for etcd",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Etcd not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false),
			expectedResult: handler.Result{
				Description: "Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: string(druidv1alpha1.OperationPhaseAdmit),
					Message:   "etcd is not ready",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Admit check passed",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false),
			expectedResult: handler.Result{
				Description: "Admit check passed",
				Requeue:     false,
			},
			expectErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var objs []client.Object
			if tc.etcdObject != nil {
				objs = append(objs, tc.etcdObject)
			}
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()

			etcdOpsTask := createEtcdOpsTask(druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			})

			taskHandler, err := New(cl, etcdOpsTask, nil)
			g.Expect(err).To(BeNil())

			admitResult := taskHandler.Admit(context.TODO())
			g.Expect(admitResult.Requeue).To(Equal(tc.expectedResult.Requeue))
			g.Expect(admitResult.Description).To(Equal(tc.expectedResult.Description))

			if tc.expectErr {
				g.Expect(admitResult.Error).ToNot(BeNil())

				if expectedDruidErr, ok := tc.expectedResult.Error.(*druiderr.DruidError); ok {
					g.Expect(admitResult.Error).To(BeAssignableToTypeOf(&druiderr.DruidError{}))
					druidErr := admitResult.Error.(*druiderr.DruidError)
					g.Expect(druidErr.Code).To(Equal(expectedDruidErr.Code))
					g.Expect(druidErr.Operation).To(Equal(expectedDruidErr.Operation))
					g.Expect(druidErr.Message).To(Equal(expectedDruidErr.Message))

				}
			} else {
				g.Expect(admitResult.Error).To(BeNil())
			}
		})

	}
}

// CreateFakeHandler creates a fake handler to inject a custom http client for testing purposes.
func CreateFakeHandler(cl client.Client, etcdOpsTask *druidv1alpha1.EtcdOpsTask, httpClient http.Client) (handler.Handler, error) {
	handler, err := New(cl, etcdOpsTask, &httpClient)
	if err != nil {
		return nil, fmt.Errorf("failed to create OnDemandSnapshotTask handler: %w", err)
	}
	onDemandSnapshotTask, ok := handler.(*Handler)
	if !ok {
		return nil, fmt.Errorf("handler is not of type OnDemandSnapshotTask: %T", handler)
	}
	onDemandSnapshotTask.httpClient = httpClient
	return handler, nil
}

// TestOnDemandSnapshotTaskRun tests the Run method of the OnDemandSnapshotTask handler.
func TestOnDemandSnapshotTaskRun(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		etcdObject     *druidv1alpha1.Etcd
		FakeResponse   *utils.FakeResponse
		expectedResult handler.Result
		expectErr      bool
	}{
		{
			name:       "Etcd not found",
			etcdObject: nil,
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("etcd object not found"),
			},
			expectedResult: handler.Result{
				Description: "Failed to get etcd object - object not found",
				Error: &druiderr.DruidError{
					Code:      ErrGetEtcd,
					Operation: string(druidv1alpha1.OperationPhaseRunning),
					Message:   "failed to get etcd object - object not found",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Etcd not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("etcd is not ready"),
			},
			expectedResult: handler.Result{
				Description: "Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: string(druidv1alpha1.OperationPhaseRunning),
					Message:   "etcd is not ready",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "HTTP request execution fails",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("connection refused"),
			},
			expectedResult: handler.Result{
				Description: "Failed to execute HTTP request",
				Error:       fmt.Errorf("connection refused"),
				Requeue:     true,
			},
			expectErr: true,
		},
		{
			name:       "Snapshot creation fails (non-200)",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
				},
				Error: nil,
			},
			expectedResult: handler.Result{
				Description: "Failed to create snapshot",
				Error: &druiderr.DruidError{
					Code:      ErrCreateSnapshot,
					Operation: string(druidv1alpha1.OperationPhaseRunning),
					Message:   "failed to create snapshot",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "TLS enabled but CA secret not found",
			etcdObject: createEtcdWithBackupTLS("test-etcd", "test-namespace", true, true),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    nil,
			},
			expectedResult: handler.Result{
				Description: "Failed to get etcdbr CA secret",
				Error: &druiderr.DruidError{
					Code:      ErrGetSecret,
					Operation: string(druidv1alpha1.OperationPhaseRunning),
					Message:   "failed to get etcdbr CA secret test-namespace/ca-etcdbr",
				},
				Requeue: true,
			},
			expectErr: true,
		},
		{
			name:       "Snapshot created successfully with TLS disabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
				},
				Error: nil,
			},
			expectedResult: handler.Result{
				Description: "Snapshot created successfully",
				Requeue:     false,
			},
			expectErr: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var objs []client.Object
			if tc.etcdObject != nil {
				objs = append(objs, tc.etcdObject)
			}
			cl := utils.NewTestClientBuilder().WithScheme(kubernetes.Scheme).WithObjects(objs...).Build()

			etcdOpsTask := createEtcdOpsTask(druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			})

			fakeHttpClient := http.Client{
				Transport: &utils.MockRoundTripper{
					Response: &tc.FakeResponse.Response,
					Err:      tc.FakeResponse.Error,
				},
			}
			taskHandler, err := CreateFakeHandler(cl, etcdOpsTask, fakeHttpClient)
			g.Expect(err).To(BeNil())

			runResult := taskHandler.Run(context.TODO())
			g.Expect(runResult).ToNot(BeNil())
			g.Expect(runResult.Requeue).To(Equal(tc.expectedResult.Requeue))
			g.Expect(runResult.Description).To(Equal(tc.expectedResult.Description))

			if tc.expectErr {
				g.Expect(runResult.Error).ToNot(BeNil())
				if expectedDruidErr, ok := tc.expectedResult.Error.(*druiderr.DruidError); ok {
					g.Expect(runResult.Error).To(BeAssignableToTypeOf(&druiderr.DruidError{}))
					druidErr := runResult.Error.(*druiderr.DruidError)
					g.Expect(druidErr.Code).To(Equal(expectedDruidErr.Code))
					g.Expect(druidErr.Operation).To(Equal(expectedDruidErr.Operation))
					g.Expect(druidErr.Message).To(Equal(expectedDruidErr.Message))
				}
			} else {
				g.Expect(runResult.Error).To(BeNil())
			}
		})
	}
}
