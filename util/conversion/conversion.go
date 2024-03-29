/*
Copyright 2019 The Kubernetes Authors.

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

// Package conversion implements conversion utilities.
package conversion

import (
	"math/rand"
	"sort"
	"strings"

	fuzz "github.com/google/gofuzz"
	clusterv1 "github.com/muxinc/cluster-api/api/v1beta1"
	"github.com/muxinc/cluster-api/util"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/api/apitesting/fuzzer"
	metafuzzer "k8s.io/apimachinery/pkg/apis/meta/fuzzer"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	runtimeserializer "k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/util/json"
)

const (
	DataAnnotation = "cluster.x-k8s.io/conversion-data"
)

var (
	contract = clusterv1.GroupVersion.String()
)

func getLatestAPIVersionFromContract(metadata metav1.Object) (string, error) {
	labels := metadata.GetLabels()

	// If there is no label, return early without changing the reference.
	supportedVersions, ok := labels[contract]
	if !ok || supportedVersions == "" {
		return "", errors.Errorf("cannot find any versions matching contract %q for GVK %v as contract version label(s) are either missing or empty", contract, metadata.GetName())
	}

	// Pick the latest version in the slice and validate it.
	kubeVersions := util.KubeAwareAPIVersions(strings.Split(supportedVersions, "_"))
	sort.Sort(kubeVersions)
	return kubeVersions[len(kubeVersions)-1], nil
}

// MarshalData stores the source object as json data in the destination object annotations map.
// It ignores the metadata of the source object.
func MarshalData(src metav1.Object, dst metav1.Object) error {
	u, err := runtime.DefaultUnstructuredConverter.ToUnstructured(src)
	if err != nil {
		return err
	}
	delete(u, "metadata")

	data, err := json.Marshal(u)
	if err != nil {
		return err
	}
	annotations := dst.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	annotations[DataAnnotation] = string(data)
	dst.SetAnnotations(annotations)
	return nil
}

// UnmarshalData tries to retrieve the data from the annotation and unmarshals it into the object passed as input.
func UnmarshalData(from metav1.Object, to interface{}) (bool, error) {
	annotations := from.GetAnnotations()
	data, ok := annotations[DataAnnotation]
	if !ok {
		return false, nil
	}
	if err := json.Unmarshal([]byte(data), to); err != nil {
		return false, err
	}
	delete(annotations, DataAnnotation)
	from.SetAnnotations(annotations)
	return true, nil
}

// GetFuzzer returns a new fuzzer to be used for testing.
func GetFuzzer(scheme *runtime.Scheme, funcs ...fuzzer.FuzzerFuncs) *fuzz.Fuzzer {
	funcs = append([]fuzzer.FuzzerFuncs{
		metafuzzer.Funcs,
		func(_ runtimeserializer.CodecFactory) []interface{} {
			return []interface{}{
				// Custom fuzzer for metav1.Time pointers which weren't
				// fuzzed and always resulted in `nil` values.
				// This implementation is somewhat similar to the one provided
				// in the metafuzzer.Funcs.
				func(input *metav1.Time, c fuzz.Continue) {
					if input != nil {
						var sec, nsec uint32
						c.Fuzz(&sec)
						c.Fuzz(&nsec)
						fuzzed := metav1.Unix(int64(sec), int64(nsec)).Rfc3339Copy()
						input.Time = fuzzed.Time
					}
				},
			}
		},
	}, funcs...)
	return fuzzer.FuzzerFor(
		fuzzer.MergeFuzzerFuncs(funcs...),
		rand.NewSource(rand.Int63()),
		runtimeserializer.NewCodecFactory(scheme),
	)
}
