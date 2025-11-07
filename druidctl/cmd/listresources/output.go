// SPDX-FileCopyrightText: 2025 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package listresources

import (
	"fmt"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	cmdutils "github.com/gardener/etcd-druid/druidctl/cmd/utils"
)

func SetupListResourcesTable(keyList []ResourceListPerKey) *table.Table {
	columns := []string{"Kind", "NamespacedName", "Age"}
	var rows [][]string
	for _, resourceKey := range keyList {
		resourceList := resourceKey.Resources
		for _, r := range resourceList {
			age := cmdutils.ShortDuration(r.Age)
			namespace := r.Namespace
			if namespace == "" {
				namespace = "N/A"
			}
			rows = append(rows, []string{
				resourceKey.Key.Kind,
				fmt.Sprintf("%s/%s", namespace, r.Name),
				age,
			})
		}
	}
	t := table.New().
		Border(lipgloss.NormalBorder()).
		Headers(columns...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle()
		})
	return t
}
