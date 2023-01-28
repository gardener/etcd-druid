package utils

import (
	"context"
	"fmt"
	"time"

	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func ClientServiceIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, svc *corev1.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s-client", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, svc); err != nil {
		return err
	}

	if !CheckEtcdOwnerReference(svc.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists")
	}
	return nil
}

func PeerServiceIsCorrectlyReconciled(c client.Client, timeout time.Duration, instance *druidv1alpha1.Etcd, svc *corev1.Service) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	req := types.NamespacedName{
		Name:      fmt.Sprintf("%s-peer", instance.Name),
		Namespace: instance.Namespace,
	}

	if err := c.Get(ctx, req, svc); err != nil {
		return err
	}

	if !CheckEtcdOwnerReference(svc.GetOwnerReferences(), instance) {
		return fmt.Errorf("ownerReference does not exists")
	}
	return nil
}
