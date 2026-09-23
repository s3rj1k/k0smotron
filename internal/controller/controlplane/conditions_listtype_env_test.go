//go:build envtest

/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controlplane

import (
	"testing"

	cpv1beta2 "github.com/k0sproject/k0smotron/v2/api/controlplane/v1beta2"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// The merge behaviour lives in the CRD schema rather than in Go, so only a real API
// server shows it. A fake client accepts either shape.
func TestControlPlaneConditionsMergeByType(t *testing.T) {
	ns, err := testEnv.CreateNamespace(t.Context(), "test-conditions-list-type")
	require.NoError(t, err)

	gvk := cpv1beta2.GroupVersion.WithKind("K0sControlPlane")

	_, kcp, gmt := createClusterWithControlPlane(ns.Name)
	require.NoError(t, testEnv.Create(t.Context(), gmt))
	require.NoError(t, testEnv.Create(t.Context(), kcp))

	applyCondition := func(manager, condType string) error {
		u := &unstructured.Unstructured{}
		u.SetGroupVersionKind(gvk)
		u.SetNamespace(kcp.Namespace)
		u.SetName(kcp.Name)
		require.NoError(t, unstructured.SetNestedSlice(u.Object, []any{
			map[string]any{
				"type":               condType,
				"status":             string(metav1.ConditionTrue),
				"reason":             "Testing",
				"message":            "set by " + manager,
				"lastTransitionTime": metav1.Now().UTC().Format("2006-01-02T15:04:05Z"),
			},
		}, "status", "conditions"))

		return testEnv.Status().Patch(t.Context(), u, client.Apply,
			client.FieldOwner(manager), client.ForceOwnership)
	}

	require.NoError(t, applyCondition("owner-a", "FromOwnerA"))
	require.NoError(t, applyCondition("owner-b", "FromOwnerB"))

	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(gvk)
	require.NoError(t, testEnv.Get(t.Context(), client.ObjectKeyFromObject(kcp), got))

	conds, found, err := unstructured.NestedSlice(got.Object, "status", "conditions")
	require.NoError(t, err)
	require.True(t, found, "the control plane reports conditions")

	types := make([]string, 0, len(conds))
	for _, c := range conds {
		types = append(types, c.(map[string]any)["type"].(string))
	}

	// Without listType=map the array is atomic, so the second apply replaces the
	// first and only FromOwnerB survives.
	require.ElementsMatch(t, []string{"FromOwnerA", "FromOwnerB"}, types,
		"two field managers each keep their own condition")
}
