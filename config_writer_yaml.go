// config_writer_yaml.go: editing a YAML document in place
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/agilira/go-errors"
	"go.yaml.in/yaml/v4"
)

// A YAML file is edited as its document, not rewritten from its values:
// what the writer does not change stays as the user wrote it (comments,
// order, quoting, the other entries). Writing it from the map lost every
// comment and put the keys in a different order at each write, as Go
// maps have none.

// loadYAMLDocument is data's document; an empty file is an empty mapping.
func loadYAMLDocument(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	if doc.Kind == 0 {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}, nil
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New(ErrCodeInvalidConfig, "YAML root must be a mapping")
	}
	return &doc, nil
}

// yamlDocumentOf is config as a document, keys sorted: a file the writer
// makes, or one it converts to YAML.
func yamlDocumentOf(config map[string]interface{}) (*yaml.Node, error) {
	var root yaml.Node
	if err := root.Encode(config); err != nil {
		return nil, err
	}
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{&root}}, nil
}

// encodeYAMLDocument writes doc with two-space indentation and no line
// width: v4's encoder folds a long string at 80 columns, so a URL or a
// long value written on one line came back on two, changing a file the
// user wrote by hand (the value read the same, the diff did not).
func encodeYAMLDocument(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	d, err := yaml.NewDumper(&out, yaml.WithV3Defaults(), yaml.WithIndent(2), yaml.WithLineWidth(-1))
	if err != nil {
		return nil, err
	}
	if err := d.Dump(doc); err != nil {
		return nil, err
	}
	if err := d.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// yamlPair is the index of key's value in mapping m, or -1.
func yamlPair(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i + 1
		}
	}
	return -1
}

// setYAMLPath sets the value at path in doc, as setNestedValue does in
// the map: missing mappings are made, a value that is not a mapping is not
// walked through. A value replaced keeps its comments.
func setYAMLPath(doc *yaml.Node, path []string, value interface{}) error {
	m, err := yamlParent(doc, path, true)
	if err != nil {
		return err
	}
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return errors.Wrap(err, ErrCodeSerializationError, "the value does not encode as YAML")
	}
	key := path[len(path)-1]
	if i := yamlPair(m, key); i >= 0 {
		old := m.Content[i]
		v.HeadComment, v.LineComment, v.FootComment = old.HeadComment, old.LineComment, old.FootComment
		m.Content[i] = &v
		return nil
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, &v)
	return nil
}

// deleteYAMLPath removes the value at path; false when there is none.
func deleteYAMLPath(doc *yaml.Node, path []string) bool {
	m, err := yamlParent(doc, path, false)
	if err != nil || m == nil {
		return false
	}
	i := yamlPair(m, path[len(path)-1])
	if i < 0 {
		return false
	}
	m.Content = append(m.Content[:i-1], m.Content[i+1:]...)
	return true
}

// yamlParent is the mapping that holds path's last key; make creates the
// mappings missing on the way (nil, nil when not making and one is).
func yamlParent(doc *yaml.Node, path []string, make bool) (*yaml.Node, error) {
	if len(path) == 0 || doc == nil || len(doc.Content) != 1 {
		return nil, errors.New(ErrCodeConfigWriterError, "empty key path")
	}
	m := doc.Content[0]
	for _, key := range path[:len(path)-1] {
		i := yamlPair(m, key)
		switch {
		case i < 0 && !make:
			return nil, nil
		case i < 0:
			next := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, next)
			m = next
		case m.Content[i].Kind == yaml.MappingNode:
			m = m.Content[i]
		default:
			return nil, errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' is not a map, cannot set nested value", strings.TrimSpace(key)))
		}
	}
	return m, nil
}
