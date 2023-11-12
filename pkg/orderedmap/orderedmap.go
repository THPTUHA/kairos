package orderedmap

import (
	"encoding/json"
	"fmt"

	"github.com/THPTUHA/kairos/pkg/deepcopy"
	"golang.org/x/exp/constraints"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
	"gopkg.in/yaml.v3"
)

type OrderedMap[K constraints.Ordered, V any] struct {
	s []K
	m map[K]V
}

func New[K constraints.Ordered, V any]() OrderedMap[K, V] {
	return OrderedMap[K, V]{
		s: make([]K, 0),
		m: make(map[K]V),
	}
}

func FromMap[K constraints.Ordered, V any](m map[K]V) OrderedMap[K, V] {
	om := New[K, V]()
	om.m = m
	om.s = maps.Keys(m)
	return om
}

func FromMapWithOrder[K constraints.Ordered, V any](m map[K]V, order []K) OrderedMap[K, V] {
	om := New[K, V]()
	if len(m) != len(order) {
		panic("length of map and order must be equal")
	}
	om.m = m
	om.s = order
	for key := range om.m {
		if !slices.Contains(om.s, key) {
			panic("order keys must match map keys")
		}
	}
	return om
}

func (om *OrderedMap[K, V]) Len() int {
	return len(om.s)
}

func (om *OrderedMap[K, V]) Set(key K, value V) {
	if om.m == nil {
		om.m = make(map[K]V)
	}
	if _, ok := om.m[key]; !ok {
		om.s = append(om.s, key)
	}
	om.m[key] = value
}

func (om *OrderedMap[K, V]) Get(key K) V {
	value, ok := om.m[key]
	if !ok {
		var zero V
		return zero
	}
	return value
}

func (om *OrderedMap[K, V]) Exists(key K) bool {
	_, ok := om.m[key]
	return ok
}

func (om *OrderedMap[K, V]) Sort() {
	slices.Sort(om.s)
}

func (om *OrderedMap[K, V]) SortFunc(less func(i, j K) int) {
	slices.SortFunc(om.s, less)
}

func (om *OrderedMap[K, V]) Keys() []K {
	return om.s
}

func (om *OrderedMap[K, V]) Values() []V {
	var values []V
	for _, key := range om.s {
		values = append(values, om.m[key])
	}
	return values
}

func (om *OrderedMap[K, V]) Range(fn func(key K, value V) error) error {
	for _, key := range om.s {
		if err := fn(key, om.m[key]); err != nil {
			return err
		}
	}
	return nil
}

func (om *OrderedMap[K, V]) Merge(other OrderedMap[K, V]) {
	other.Range(func(key K, value V) error {
		om.Set(key, value)
		return nil
	})
}

func (om *OrderedMap[K, V]) DeepCopy() OrderedMap[K, V] {
	return OrderedMap[K, V]{
		s: deepcopy.Slice(om.s),
		m: deepcopy.Map(om.m),
	}
}

func (om *OrderedMap[K, V]) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			var k K
			if err := keyNode.Decode(&k); err != nil {
				return err
			}

			valueNode := node.Content[i+1]
			var v V
			if err := valueNode.Decode(&v); err != nil {
				return err
			}

			om.Set(k, v)
		}
		return nil
	}

	return fmt.Errorf("yaml: line %d: cannot unmarshal %s into variables", node.Line, node.ShortTag())
}

func (om *OrderedMap[K, V]) MarshalJSON() ([]byte, error) {
	mjson := make(map[K]V, 0)
	om.Range(func(key K, value V) error {
		mjson[key] = value
		return nil
	})
	return json.Marshal(mjson)
}

func (om *OrderedMap[K, V]) UnmarshalJSON(data []byte) error {
	var value map[K]V
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	for k, v := range value {
		om.Set(k, v)
	}
	return nil
}
