package version

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestCheckVersionMeetsConstraint(t *testing.T) {

	tests := []struct {
		name           string
		version        string
		constraint     string
		expectedResult bool
	}{
		{
			name:           "version matches constraint",
			version:        "1.2.3",
			constraint:     ">1.2.2",
			expectedResult: true,
		},
		{
			name:           "version does not match constraint",
			version:        "1.2.3",
			constraint:     ">1.2.4",
			expectedResult: false,
		},
		{
			name:           "version with suffix matches constraint",
			version:        "1.2.3-foo.12",
			constraint:     ">1.2.2-foo.23",
			expectedResult: true,
		},
		{
			name:           "version with suffix does not match constraint",
			version:        "1.2.3-foo.12",
			constraint:     ">1.2.4-foo.34",
			expectedResult: false,
		},
	}

	g := NewWithT(t)
	t.Parallel()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			result, err := CheckVersionMeetsConstraint(test.version, test.constraint)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(Equal(test.expectedResult))
		})
	}
}
