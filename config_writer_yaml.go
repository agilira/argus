// config_writer_yaml.go: editing a YAML document in place
//
// Copyright (c) 2026 AGILira - A. Giordano
// SPDX-License-Identifier: MPL-2.0

package argus

import (
	"bytes"
	"fmt"
	"math"
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
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, yamlErrorSummary(err, data)) // the position, not the text
	}
	if doc.Kind == 0 || len(doc.Content) == 1 && doc.Content[0].ShortTag() == "!!null" {
		// No document, or a null one ("~"), is an empty configuration, as
		// ParseConfig reads it; its comments stay (a file of comments only).
		empty := &yaml.Node{Kind: yaml.DocumentNode, HeadComment: doc.HeadComment, FootComment: doc.FootComment,
			Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
		if doc.Kind == 0 {
			empty.HeadComment = commentLines(data) // a file of comments only gives no node to hang them on
		}
		if len(doc.Content) == 1 {
			root := doc.Content[0]
			empty.HeadComment = strings.TrimSpace(empty.HeadComment + "\n" + root.HeadComment)
			empty.FootComment = strings.TrimSpace(root.LineComment + "\n" + root.FootComment + "\n" + empty.FootComment)
		}
		return empty, nil
	}
	if len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, errors.New(ErrCodeInvalidConfig, "YAML root must be a mapping")
	}
	return &doc, nil
}

// commentLines are data's comment lines, as a node's comment holds them.
func commentLines(data []byte) string {
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
			lines = append(lines, t)
		}
	}
	return strings.Join(lines, "\n")
}

// yamlDocumentOf is config as a document, keys sorted: a file the writer
// makes, or one it converts to YAML.
func yamlDocumentOf(config map[string]interface{}) (*yaml.Node, error) {
	root, err := yamlValueNode(config)
	if err != nil {
		return nil, err
	}
	root.Style &^= yaml.FlowStyle // a file's root in block style, even empty ({} otherwise)
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{root}}, nil
}

// encodeYAMLDocument writes doc with two-space indentation and no line
// width: v4's encoder folds a long string at 80 columns, so a URL or a
// long value written on one line came back on two, changing a file the
// user wrote by hand (the value read the same, the diff did not).
func encodeYAMLDocument(doc *yaml.Node) ([]byte, error) {
	blockWhereQuoted(doc)
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
// walked through. A value replaced keeps its comments, and a string its
// quoting. An anchored value is not replaced: its aliases would point at
// nothing, and the file would not parse (audit 22/11).
func setYAMLPath(doc *yaml.Node, path []string, value interface{}) error {
	v, err := yamlValueNode(value)
	if err != nil {
		return err
	}
	return setYAMLNode(doc, path, v)
}

// yamlValueNode is value as a node. A float is written as a float: the
// encoder gave 1.0 as 1, an integer when read back (audit 22/11).
func yamlValueNode(value interface{}) (*yaml.Node, error) {
	var v yaml.Node
	if err := v.Encode(value); err != nil {
		return nil, errors.Wrap(err, ErrCodeSerializationError, "the value does not encode as YAML")
	}
	floatsAsFloats(&v, value)
	return &v, nil
}

// floatsAsFloats walks n beside the value it was encoded from (as
// normalizeGoValue holds it: float32 is float64 by then) and tags each
// finite float !!float, with the text that reads back as it.
func floatsAsFloats(n *yaml.Node, value interface{}) {
	switch v := value.(type) {
	case float64:
		if n.Kind == yaml.ScalarNode && !math.IsInf(v, 0) && !math.IsNaN(v) {
			n.Tag, n.Value, n.Style = "!!float", formatFloat(v, 64), 0
		}
	case map[string]interface{}: // a mapping, its keys sorted by the encoder
		for i := 0; i+1 < len(n.Content); i += 2 {
			if child, ok := v[n.Content[i].Value]; ok {
				floatsAsFloats(n.Content[i+1], child)
			}
		}
	case []interface{}: // a sequence, item for item
		for i := 0; i < len(n.Content) && i < len(v); i++ {
			floatsAsFloats(n.Content[i], v[i])
		}
	}
}

// blockWhereQuoted writes in block style a flow collection ({...} or
// [...]) holding a value that is not a string but whose text has a
// character flow style reserves (a timestamp's ':'): the encoder quoted
// it there, and the file read it back as a string (audit 22/11).
func blockWhereQuoted(n *yaml.Node) {
	needsBlock(n)
}

// needsBlock reports whether n holds such a value, and writes in block
// style every flow collection on the way to one: a flow parent makes its
// children flow too.
func needsBlock(n *yaml.Node) bool {
	if n.Kind == yaml.ScalarNode {
		return n.ShortTag() != "!!str" && strings.ContainsAny(n.Value, ":,[]{}#")
	}
	found := false
	for _, c := range n.Content {
		if needsBlock(c) {
			found = true
		}
	}
	if found && (n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode) {
		n.Style &^= yaml.FlowStyle
	}
	return found
}

// setYAMLNode puts v at path.
func setYAMLNode(doc *yaml.Node, path []string, v *yaml.Node) error {
	m, err := yamlParent(doc, path, true)
	if err != nil {
		return err
	}
	key := path[len(path)-1]
	if i := yamlPair(m, key); i >= 0 {
		old := m.Content[i]
		if err := editable(old, key); err != nil {
			return err
		}
		v.HeadComment, v.LineComment, v.FootComment = old.HeadComment, old.LineComment, old.FootComment
		if old.Kind == yaml.ScalarNode && v.Kind == yaml.ScalarNode && v.Tag == "!!str" && old.ShortTag() == "!!str" {
			v.Style = old.Style // "x" stays quoted
		}
		m.Content[i] = v
		return nil
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, v)
	return nil
}

// yamlValueOf is the value node v reads back as, by the rules ParseConfig
// applies to a whole file: its keys checked, a null under a key an empty
// mapping.
func yamlValueOf(v *yaml.Node) (interface{}, error) {
	if err := checkYAMLKeys(v); err != nil {
		return nil, err
	}
	var out interface{}
	if err := v.Decode(&out); err != nil {
		return nil, errors.Wrap(err, ErrCodeInvalidConfig, "the value does not read back as YAML")
	}
	holder := normalizeYAMLTypes(map[string]interface{}{"v": out}).(map[string]interface{})
	if err := validateYAMLKeys(holder); err != nil {
		return nil, err
	}
	return holder["v"], nil
}

// yamlShared reports whether doc has entries that others share: an anchor,
// an alias or a merge (<<). Edits of such a document go through a copy
// read back whole; others are made in place.
func yamlShared(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Kind == yaml.ScalarNode && n.ShortTag() == "!!merge" {
		return true
	}
	for _, c := range n.Content {
		if yamlShared(c) {
			return true
		}
	}
	return false
}

// deleteYAMLPath removes the value at path; false when there is none.
func deleteYAMLPath(doc *yaml.Node, path []string) (bool, error) {
	m, err := yamlParent(doc, path, false)
	if err != nil || m == nil {
		return false, err
	}
	key := path[len(path)-1]
	i := yamlPair(m, key)
	if i < 0 {
		return false, nil
	}
	if err := editable(m.Content[i], key); err != nil {
		return false, err
	}
	// The comment above the first key of the file is the file's when there
	// is no blank line between them: YAML hangs it on the key. Deleting the
	// key must not take the file's header with it.
	if i == 1 && m == doc.Content[0] && m.Content[0].HeadComment != "" {
		doc.HeadComment = strings.TrimSpace(doc.HeadComment + "\n" + m.Content[0].HeadComment)
	}
	m.Content = append(m.Content[:i-1], m.Content[i+1:]...)
	return true, nil
}

// editable refuses a value other entries refer to (an anchor), or that is
// another's (an alias): changed here, it would change, or break, them.
func editable(n *yaml.Node, key string) error {
	switch {
	case n.Anchor != "":
		return errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' is an anchor other entries use: edit the file by hand", key))
	case n.Kind == yaml.AliasNode:
		return errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' is an alias of another entry: edit the file by hand", key))
	}
	return nil
}

// yamlParent is the mapping that holds path's last key; make creates the
// mappings missing on the way (nil, nil when not making and one is). The
// path does not go through an alias or a merged mapping (<<): its keys are
// another entry's.
func yamlParent(doc *yaml.Node, path []string, make bool) (*yaml.Node, error) {
	if len(path) == 0 || doc == nil || len(doc.Content) != 1 {
		return nil, errors.New(ErrCodeConfigWriterError, "empty key path")
	}
	m := doc.Content[0]
	for _, key := range path[:len(path)-1] {
		i := yamlPair(m, key)
		switch {
		case i < 0 && hasMerge(m):
			return nil, errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' may be merged from another entry (<<): edit the file by hand", key))
		case i < 0 && !make:
			return nil, nil
		case i < 0:
			next := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}, next)
			m = next
		case m.Content[i].Kind == yaml.AliasNode:
			return nil, errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' is an alias of another entry: edit the file by hand", key))
		case make && m.Content[i].Kind == yaml.ScalarNode && m.Content[i].ShortTag() == "!!null" && m.Content[i].Anchor == "":
			// "a: ~" reads as an empty mapping: it can be filled as one.
			old := m.Content[i]
			next := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", HeadComment: old.HeadComment, LineComment: old.LineComment, FootComment: old.FootComment}
			m.Content[i] = next
			m = next
		case m.Content[i].Kind == yaml.MappingNode:
			m = m.Content[i]
		default:
			return nil, errors.New(ErrCodeConfigWriterError, fmt.Sprintf("key '%s' is not a map, cannot set nested value", strings.TrimSpace(key)))
		}
	}
	return m, nil
}

// hasMerge reports a merge key (<<, not the quoted string "<<") in m.
func hasMerge(m *yaml.Node) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].ShortTag() == "!!merge" {
			return true
		}
	}
	return false
}

// cloneYAML is a deep copy of n, its aliases pointing into the copy: an
// edit is made on it and kept only once the result reads back.
func cloneYAML(n *yaml.Node) *yaml.Node {
	seen := map[*yaml.Node]*yaml.Node{}
	var clone func(*yaml.Node) *yaml.Node
	clone = func(n *yaml.Node) *yaml.Node {
		if n == nil {
			return nil
		}
		if c, ok := seen[n]; ok {
			return c
		}
		c := *n
		seen[n] = &c
		c.Content = make([]*yaml.Node, len(n.Content))
		for i, child := range n.Content {
			c.Content[i] = clone(child)
		}
		c.Alias = clone(n.Alias)
		return &c
	}
	return clone(n)
}
