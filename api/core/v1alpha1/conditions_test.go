package v1alpha1

import (
	"testing"

	. "github.com/onsi/gomega"
)

const (
	conditionTypeTest = "Test"
)

// TestHasConditionWithStatus tests the function HasConditionWithStatus.
func TestHasConditionWithStatus(t *testing.T) {
	testCases := []struct {
		name               string
		etcd               Etcd
		queryConditionType ConditionType
		queryStatus        ConditionStatus
		actualConditions   []Condition
		expectedResult     bool
	}{
		{
			name: "Condition exists with the given status",
			etcd: Etcd{
				Status: EtcdStatus{
					Conditions: []Condition{
						{
							Type:   conditionTypeTest,
							Status: ConditionTrue,
						},
					},
				},
			},
			queryConditionType: conditionTypeTest,
			queryStatus:        ConditionTrue,
			expectedResult:     true,
		},
		{
			name: "Condition exists without the given status",
			etcd: Etcd{
				Status: EtcdStatus{
					Conditions: []Condition{
						{
							Type:   conditionTypeTest,
							Status: ConditionFalse,
						},
					},
				},
			},
			queryConditionType: conditionTypeTest,
			queryStatus:        ConditionTrue,
			expectedResult:     false,
		},
		{
			name: "Condition does not exist",
			etcd: Etcd{
				Status: EtcdStatus{
					Conditions: []Condition{
						{
							Type:   "AnotherCondition",
							Status: ConditionTrue,
						},
					},
				},
			},
			queryConditionType: conditionTypeTest,
			queryStatus:        ConditionTrue,
			expectedResult:     false,
		},
	}

	g := NewGomegaWithT(t)
	t.Parallel()

	for _, tc := range testCases {
		t.Run(tc.name, func(_ *testing.T) {
			g.Expect(tc.etcd.HasConditionWithStatus(tc.queryConditionType, tc.queryStatus)).To(Equal(tc.expectedResult))
		})
	}
}
