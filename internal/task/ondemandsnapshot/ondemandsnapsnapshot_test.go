package ondemandsnapshot

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/core/v1alpha1"
	"github.com/gardener/etcd-druid/internal/client/kubernetes"
	druiderr "github.com/gardener/etcd-druid/internal/errors"
	"github.com/gardener/etcd-druid/internal/task"
	"github.com/gardener/etcd-druid/test/utils"

	"github.com/go-logr/logr"
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

func createEtcdOpsTask(config druidv1alpha1.OnDemandSnapshotConfig) *druidv1alpha1.EtcdOpsTask {
	etcdRef := druidv1alpha1.EtcdReference{
		Name:      "test-etcd",
		Namespace: "test-namespace",
	}
	return &druidv1alpha1.EtcdOpsTask{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-task",
			Namespace: "test-namespace",
		},
		Spec: druidv1alpha1.EtcdOpsTaskSpec{
			EtcdRef: &etcdRef,
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
		expectedResult task.Result
		expectErr      bool
	}{
		{
			name:       "Etcd not found",
			etcdObject: nil,
			expectedResult: task.Result{
				Description: "Admit Operation: Failed to get etcd object",
				Error: &druiderr.DruidError{
					Code:      ErrGetEtcd,
					Operation: "Admit",
					Message:   "failed to get etcd object",
				},
				Completed: true,
			},
			expectErr: true,
		},
		{
			name:       "Backup not enabled",
			etcdObject: createEtcd("test-etcd", "test-namespace", false, true, false),
			expectedResult: task.Result{
				Description: "Admit Operation: Backup is not enabled for etcd",
				Error: &druiderr.DruidError{
					Code:      ErrBackupNotEnabled,
					Operation: "Admit",
					Message:   "backup is not enabled for etcd",
				},
				Completed: true,
			},
			expectErr: true,
		},
		{
			name:       "Etcd not ready",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, false, false),
			expectedResult: task.Result{
				Description: "Admit Operation: Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: "Admit",
					Message:   "etcd is not ready",
				},
				Completed: true,
			},
			expectErr: true,
		},
		{
			name:       "Admit check passed",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, false),
			expectedResult: task.Result{
				Description: "Admit check passed",
				Completed:   true,
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

			mgr := utils.FakeManager{
				Client: cl,
				Scheme: kubernetes.Scheme,
			}

			etcdOpsTask := createEtcdOpsTask(druidv1alpha1.OnDemandSnapshotConfig{
				Type:           "Full",
				TimeoutSeconds: ptr.To(int32(5)),
				IsFinal:        ptr.To(false),
			})

			taskHandler, err := New(cl, mgr.GetLogger(), etcdOpsTask)
			g.Expect(err).To(BeNil())

			admitResult := taskHandler.Admit(context.TODO())
			g.Expect(admitResult).ToNot(BeNil())
			g.Expect(admitResult.Completed).To(Equal(tc.expectedResult.Completed))
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
func CreateFakeHandler(cl client.Client, logger logr.Logger, etcdOpsTask *druidv1alpha1.EtcdOpsTask, httpClient http.Client) (task.Handler, error) {
	handler, err := New(cl, logger, etcdOpsTask)
	if err != nil {
		return nil, fmt.Errorf("failed to create OnDemandSnapshotTask handler: %w", err)
	}
	onDemandSnapshotTask, ok := handler.(*OnDemandSnapshotTask)
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
		expectedResult task.Result
		expectErr      bool
	}{
		{
			name:       "Etcd not found",
			etcdObject: nil,
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    fmt.Errorf("etcd object not found"),
			},
			expectedResult: task.Result{
				Description: "Run Operation: Failed to get etcd object",
				Error: &druiderr.DruidError{
					Code:      ErrGetEtcd,
					Operation: "Run",
					Message:   "failed to get etcd object",
				},
				Completed: false,
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
			expectedResult: task.Result{
				Description: "Run Operation: Etcd is not ready",
				Error: &druiderr.DruidError{
					Code:      ErrEtcdNotReady,
					Operation: "Run",
					Message:   "etcd is not ready",
				},
				Completed: true,
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
			expectedResult: task.Result{
				Description: "Run Operation: Failed to execute HTTP request",
				Error:       fmt.Errorf("connection refused"),
				Completed:   false,
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
			expectedResult: task.Result{
				Description: "Run Operation: Failed to create snapshot",
				Error: &druiderr.DruidError{
					Code:      ErrCreateSnapshot,
					Operation: "Run",
					Message:   "failed to create snapshot",
				},
				Completed: true,
			},
			expectErr: true,
		},
		{
			name:       "TLS enabled but CA secret not found",
			etcdObject: createEtcd("test-etcd", "test-namespace", true, true, true),
			FakeResponse: &utils.FakeResponse{
				Response: http.Response{},
				Error:    nil,
			},
			expectedResult: task.Result{
				Description: "Run Operation: Failed to get etcdbr CA secret",
				Error: &druiderr.DruidError{
					Code:      ErrGetSecret,
					Operation: "Run",
					Message:   "failed to get etcdbr CA secret test-namespace/client-url-ca-etcd",
				},
				Completed: false,
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
			expectedResult: task.Result{
				Description: "Snapshot created successfully",
				Completed:   true,
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

			mgr := utils.FakeManager{
				Client: cl,
				Scheme: kubernetes.Scheme,
			}

			etcdOpsTask := createEtcdOpsTask(druidv1alpha1.OnDemandSnapshotConfig{
				Type:           "Full",
				TimeoutSeconds: ptr.To(int32(5)),
				IsFinal:        ptr.To(false),
			})

			fakeHttpClient := http.Client{
				Transport: &utils.MockRoundTripper{
					Response: &tc.FakeResponse.Response,
					Err:      tc.FakeResponse.Error,
				},
			}
			taskHandler, err := CreateFakeHandler(cl, mgr.GetLogger(), etcdOpsTask, fakeHttpClient)
			g.Expect(err).To(BeNil())

			runResult := taskHandler.Run(context.TODO())
			g.Expect(runResult).ToNot(BeNil())
			g.Expect(runResult.Completed).To(Equal(tc.expectedResult.Completed))
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
