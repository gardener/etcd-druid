// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import "net/http"

// FakeResponse is a struct to mock the HTTP response for testing purposes.
type FakeResponse struct {
	Response http.Response
	Error    error
}

// MockRoundTripper is a mock implementation of http.RoundTripper for testing purposes.
type MockRoundTripper struct {
	Response *http.Response
	Err      error
}

// RoundTrip is a mock implementation of the RoundTrip method for the MockRoundTripper.
func (m *MockRoundTripper) RoundTrip(_ *http.Request) (*http.Response, error) {
	return m.Response, m.Err
}
