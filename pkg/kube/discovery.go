package kube

import (
	"context"
	"fmt"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

var xrdGVR = schema.GroupVersionResource{
	Group:    "apiextensions.crossplane.io",
	Version:  "v1",
	Resource: "compositeresourcedefinitions",
}

// DiscoverFromXRD discovers claim and XR GVRs from Crossplane XRDs.
func DiscoverFromXRD(ctx context.Context, client dynamic.Interface) ([]schema.GroupVersionResource, []schema.GroupVersionResource, error) {
	list, err := client.Resource(xrdGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, nil, fmt.Errorf("list compositeresourcedefinitions: %w", err)
	}

	claimSet := map[string]schema.GroupVersionResource{}
	xrSet := map[string]schema.GroupVersionResource{}

	for _, item := range list.Items {
		xrGVR, claimGVR, hasClaim, err := xrdToGVRs(item)
		if err != nil {
			name := item.GetName()
			if name == "" {
				name = "<unknown>"
			}
			return nil, nil, fmt.Errorf("derive GVRs from XRD %q: %w", name, err)
		}

		xrSet[gvrKey(xrGVR)] = xrGVR
		if hasClaim {
			claimSet[gvrKey(claimGVR)] = claimGVR
		}
	}

	return mapToSortedSlice(claimSet), mapToSortedSlice(xrSet), nil
}

func xrdToGVRs(xrd unstructured.Unstructured) (schema.GroupVersionResource, schema.GroupVersionResource, bool, error) {
	group, found, err := unstructured.NestedString(xrd.Object, "spec", "group")
	if err != nil || !found || group == "" {
		return schema.GroupVersionResource{}, schema.GroupVersionResource{}, false, fmt.Errorf("missing spec.group")
	}

	xrPlural, found, err := unstructured.NestedString(xrd.Object, "spec", "names", "plural")
	if err != nil || !found || xrPlural == "" {
		return schema.GroupVersionResource{}, schema.GroupVersionResource{}, false, fmt.Errorf("missing spec.names.plural")
	}

	version, err := selectVersion(xrd)
	if err != nil {
		return schema.GroupVersionResource{}, schema.GroupVersionResource{}, false, err
	}

	xrGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: xrPlural,
	}

	claimPlural, found, err := unstructured.NestedString(xrd.Object, "spec", "claimNames", "plural")
	if err != nil || !found || claimPlural == "" {
		return xrGVR, schema.GroupVersionResource{}, false, nil
	}

	claimGVR := schema.GroupVersionResource{
		Group:    group,
		Version:  version,
		Resource: claimPlural,
	}
	return xrGVR, claimGVR, true, nil
}

func selectVersion(xrd unstructured.Unstructured) (string, error) {
	versions, found, err := unstructured.NestedSlice(xrd.Object, "spec", "versions")
	if err != nil || !found || len(versions) == 0 {
		return "", fmt.Errorf("missing spec.versions")
	}

	for _, v := range versions {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		referenceable, _, _ := unstructured.NestedBool(vm, "referenceable")
		name, _, _ := unstructured.NestedString(vm, "name")
		if referenceable && name != "" {
			return name, nil
		}
	}

	for _, v := range versions {
		vm, ok := v.(map[string]interface{})
		if !ok {
			continue
		}
		served, servedFound, _ := unstructured.NestedBool(vm, "served")
		name, _, _ := unstructured.NestedString(vm, "name")
		if servedFound && served && name != "" {
			return name, nil
		}
	}

	return "", fmt.Errorf("no referenceable or served version found")
}

func mapToSortedSlice(items map[string]schema.GroupVersionResource) []schema.GroupVersionResource {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	out := make([]schema.GroupVersionResource, 0, len(keys))
	for _, key := range keys {
		out = append(out, items[key])
	}
	return out
}

func gvrKey(gvr schema.GroupVersionResource) string {
	return gvr.Group + "/" + gvr.Version + "/" + gvr.Resource
}
