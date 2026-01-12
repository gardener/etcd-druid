// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ConfirmAllNamespaces prompts the user to confirm operating on all namespaces.
// Returns true if user confirms (y/Y), false otherwise.
func ConfirmAllNamespaces(out io.Writer, in io.Reader, operation string) (bool, error) {
	prompt := fmt.Sprintf("⚠️  You are about to %s ALL etcd resources across ALL namespaces. Continue? [y/N]: ", operation)
	fmt.Fprint(out, prompt)

	reader := bufio.NewReader(in)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read response: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// ErrConfirmationDeclined is returned when confirmation is skipped due to non-interactive mode
var ErrConfirmationDeclined = fmt.Errorf("operation cancelled by user")
