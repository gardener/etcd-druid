// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package ondemandsnapshot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	druidapicommon "github.com/gardener/etcd-druid/api/common"
	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	taskhandler "github.com/gardener/etcd-druid/internal/controller/etcdopstask/handler"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/test/utils"

	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/gomega"
)

// TestOnDemandSnapshotTaskAdmit tests the Admit method of the OnDemandSnapshotTask handler.
func TestOnDemandSnapshotTaskAdmit(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		etcdObject     *druidv1alpha1.Etcd
		expectedResult taskhandler.Result
		expectErr      bool
	}{
		{
			name:       "Should return error without requeue when Etcd object is not found",
			etcdObject: nil,
			expectedResult: taskhandler.Result{
				Description: "Etcd object not found",
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetEtcd,
					Operation: string(druidv1alpha1.LastOperationTypeAdmit),
					Message:   "etcd object not found",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when backup is not enabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", false, true, false, false),
			expectedResult: taskhandler.Result{
				Description: "Backup is not enabled for etcd",
				Error: &druiderr.DruidError{
					Code:      ErrBackupNotEnabled,
					Operation: string(druidv1alpha1.LastOperationTypeAdmit),
					Message:   "backup is not enabled for etcd",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when Etcd is not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false, false),
			expectedResult: taskhandler.Result{
				Description: "Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: string(druidv1alpha1.LastOperationTypeAdmit),
					Message:   "etcd is not ready",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should pass admit check when conditions are met",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false, false),
			expectedResult: taskhandler.Result{
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

			etcdOpsTask := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-namespace").WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			}).Build()

			taskHandler, err := New(cl, etcdOpsTask, nil)
			g.Expect(err).To(BeNil())

			admitResult := taskHandler.Admit(context.Background())
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

// TestOnDemandSnapshotTaskExecute tests the Execute method of the OnDemandSnapshotTask handler.
func TestOnDemandSnapshotTaskExecute(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name                string
		etcdObject          *druidv1alpha1.Etcd
		isSnapshotTypeDelta bool
		FakeResponse        *utils.FakeResponse
		expectedResult      taskhandler.Result
		expectErr           bool
	}{
		{
			name:       "Should return error without requeue when Etcd object is not found",
			etcdObject: nil,
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("etcd object not found"),
			},
			expectedResult: taskhandler.Result{
				Description: "Etcd object not found",
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetEtcd,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "etcd object not found",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when backup is not enabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", false, true, false, false),
			expectedResult: taskhandler.Result{
				Description: "Backup is not enabled for etcd",
				Error: &druiderr.DruidError{
					Code:      ErrBackupNotEnabled,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "backup is not enabled for etcd",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when Etcd is not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false, false),
			expectedResult: taskhandler.Result{
				Description: "Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "etcd is not ready",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should requeue with error when HTTP request execution fails",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("connection refused"),
			},
			expectedResult: taskhandler.Result{
				Description: "Failed to execute HTTP request",
				Error:       fmt.Errorf("connection refused"),
				Requeue:     true,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when snapshot creation fails with non-200 status",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{
					StatusCode: http.StatusInternalServerError,
					Status:     "500 Internal Server Error",
				},
				Error: nil,
			},
			expectedResult: taskhandler.Result{
				Description: "Failed to create snapshot",
				Error: &druiderr.DruidError{
					Code:      ErrCreateSnapshot,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "failed to create snapshot",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:       "Should return error without requeue when TLS is enabled but CA secret is not found",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false, true),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    nil,
			},
			expectedResult: taskhandler.Result{
				Description: "Failed to get etcdbr CA secret",
				Error: &druiderr.DruidError{
					Code:      taskhandler.ErrGetCASecret,
					Operation: string(druidv1alpha1.LastOperationTypeExecution),
					Message:   "failed to get etcdbr CA secret test-namespace/ca-etcdbr",
				},
				Requeue: false,
			},
			expectErr: true,
		},
		{
			name:                "Should log correct responnse in result description and return when delta snapshot is skipped by backup-restore server",
			etcdObject:          createEtcd("test-etcd", "test-namespace", true, true, false, false),
			isSnapshotTypeDelta: true,
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Body:       io.NopCloser(strings.NewReader("null")),
				},
				Error: nil,
			},
			expectedResult: taskhandler.Result{
				Description: "Delta snapshot was skipped by backup-restore server",
				Requeue:     false,
			},
		},
		{
			name:       "Should succeed in creating snapshot when TLS is disabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false, false),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
				},
				Error: nil,
			},
			expectedResult: taskhandler.Result{
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

			ondemandSnapshotConfig := &druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			}
			if tc.isSnapshotTypeDelta {
				ondemandSnapshotConfig.Type = druidv1alpha1.OnDemandSnapshotTypeDelta
				ondemandSnapshotConfig.TimeoutSecondsDelta = ptr.To(int32(2))
			}
			etcdOpsTask := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-namespace").WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(ondemandSnapshotConfig).Build()

			fakeHttpClient := http.Client{}
			if tc.FakeResponse != nil {
				fakeHttpClient = http.Client{
					Transport: &utils.MockRoundTripper{
						Response: &tc.FakeResponse.Response,
						Err:      tc.FakeResponse.Error,
					},
				}
			}
			taskHandler, err := New(cl, etcdOpsTask, &fakeHttpClient)
			g.Expect(err).To(BeNil())

			runResult := taskHandler.Execute(context.Background())
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

func TestOndemandSnapshotTaskCleanup(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name           string
		etcdObject     *druidv1alpha1.Etcd
		expectedResult taskhandler.Result
	}{
		{
			name: "Cleanup is a no-op and should always succeed",
			expectedResult: taskhandler.Result{
				Description: "Cleanup completed",
				Requeue:     false,
			},
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

			etcdOpsTask := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-namespace").WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			}).Build()

			taskHandler, err := New(cl, etcdOpsTask, nil)
			g.Expect(err).To(BeNil())

			cleanupResult := taskHandler.Cleanup(context.Background())
			g.Expect(cleanupResult.Requeue).To(Equal(tc.expectedResult.Requeue))
			g.Expect(cleanupResult.Description).To(Equal(tc.expectedResult.Description))
			g.Expect(cleanupResult.Error).To(BeNil())
		})
	}
}

// TestCheckPrerequisitesForSnapshot tests the validateEtcdForSnapshot method of the OnDemandSnapshotTask handler.
func TestCheckPrerequisitesForSnapshot(t *testing.T) {
	g := NewGomegaWithT(t)
	tests := []struct {
		name          string
		etcdObject    *druidv1alpha1.Etcd
		phase         druidapicommon.LastOperationType
		expectedError *druiderr.DruidError
		expectRequeue bool
	}{
		{
			name:       "Should return error when backup is not enabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", false, true, false, false),
			phase:      druidv1alpha1.LastOperationTypeAdmit,
			expectedError: &druiderr.DruidError{
				Code:      ErrBackupNotEnabled,
				Operation: string(druidv1alpha1.LastOperationTypeAdmit),
				Message:   "backup is not enabled for etcd",
			},
			expectRequeue: false,
		},
		{
			name:       "Should return error when etcd is not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false, false),
			phase:      druidv1alpha1.LastOperationTypeExecution,
			expectedError: &druiderr.DruidError{
				Code:      ErrEtcdNotReady,
				Operation: string(druidv1alpha1.LastOperationTypeExecution),
				Message:   "etcd is not ready",
			},
			expectRequeue: false,
		},
		{
			name:          "Should successfully validate etcd when all conditions are met",
			etcdObject:    createEtcd("test-etcd", "test-namespace", true, true, false, false),
			phase:         druidv1alpha1.LastOperationTypeExecution,
			expectedError: nil,
			expectRequeue: false,
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

			etcdOpsTask := utils.EtcdOpsTaskBuilderWithDefaults("test-task", "test-namespace").WithEtcdName("test-etcd").WithOnDemandSnapshotConfig(&druidv1alpha1.OnDemandSnapshotConfig{
				Type:               druidv1alpha1.OnDemandSnapshotTypeFull,
				TimeoutSecondsFull: ptr.To(int32(5)),
				IsFinal:            ptr.To(false),
			}).Build()

			taskHandler, err := New(cl, etcdOpsTask, nil)
			g.Expect(err).To(BeNil())

			h := taskHandler.(*handler)
			result := h.checkPrerequisitesForSnapshot(tc.etcdObject, tc.phase)
			if tc.expectedError != nil {
				g.Expect(result).ToNot(BeNil())
				g.Expect(result.Error).To(BeAssignableToTypeOf(&druiderr.DruidError{}))

				druidErr := result.Error.(*druiderr.DruidError)
				g.Expect(druidErr.Code).To(Equal(tc.expectedError.Code))

				g.Expect(result.Requeue).To(Equal(tc.expectRequeue))
				return
			}
			g.Expect(result).To(BeNil())

		})
	}
}

func createEtcd(name, namespace string, backup bool, healthy bool, clientTLS bool, backupTLS bool) *druidv1alpha1.Etcd {
	etcdBuilder := utils.EtcdBuilderWithoutDefaults(name, namespace).WithReplicas(1).WithReadyStatus()

	if clientTLS {
		etcdBuilder = etcdBuilder.WithClientTLS()
	}

	if backupTLS {
		etcdBuilder = etcdBuilder.WithBackupRestoreTLS()
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
