package sample

import (
	druidv1alpha1 "github.com/gardener/etcd-druid/api/v1alpha1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NewRole creates a new sample Role initializing it from the passed in etcd object.
func NewRole(etcd *druidv1alpha1.Etcd) *rbacv1.Role {
	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            etcd.GetRoleName(),
			Namespace:       etcd.Namespace,
			OwnerReferences: []metav1.OwnerReference{etcd.GetAsOwnerReference()},
		},
		Rules: createPolicyRules(),
	}
}

func createPolicyRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{
			APIGroups: []string{"coordination.k8s.io"},
			Resources: []string{"leases"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"statefulsets"},
			Verbs:     []string{"get", "list", "patch", "update", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"pods"},
			Verbs:     []string{"get", "list", "watch"},
		},
	}
}
