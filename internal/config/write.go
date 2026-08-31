package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	configDirPerm  = 0o700
	configFilePerm = 0o600
	documentIndent = 2
)

var ErrUnknownKey = errors.New("unknown config key")

type documentEncoder interface {
	SetIndent(int)
	Encode(any) error
	Close() error
}

func newYAMLEncoder(w io.Writer) documentEncoder {
	return yaml.NewEncoder(w)
}

var newDocumentEncoder = newYAMLEncoder

func Patch(path string, values map[string]string) error {
	scalars := make(map[string]*yaml.Node, len(values))
	keys := make([]string, 0, len(values))
	for key, value := range values {
		scalar, err := scalarFor(key, value)
		if err != nil {
			return err
		}
		scalars[key] = scalar
		keys = append(keys, key)
	}
	sort.Strings(keys)

	doc, err := readDocument(path)
	if err != nil {
		return err
	}
	for _, key := range keys {
		setKey(doc.Content[0], strings.Split(key, "."), scalars[key])
	}

	data, err := encode(doc)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return Write(path, data)
}

func encode(doc *yaml.Node) ([]byte, error) {
	var out bytes.Buffer
	enc := newDocumentEncoder(&out)
	enc.SetIndent(documentIndent)
	if err := enc.Encode(doc); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func scalarFor(key, value string) (*yaml.Node, error) {
	switch key {
	case "project.name", "music.root", "music.scale", "music.theme":
		if value == "" {
			return nil, fmt.Errorf("%s: must not be empty", key)
		}
		return scalar("!!str", value), nil
	case "audio.volume":
		v, err := strconv.ParseFloat(value, 64)
		if err != nil {
			return nil, fmt.Errorf("audio.volume: %q is not a valid number", value)
		}
		if !(v >= 0 && v <= 1) {
			return nil, fmt.Errorf("audio.volume: %v out of range [0, 1]", v)
		}
		return scalar("!!float", strconv.FormatFloat(v, 'f', -1, 64)), nil
	case "audio.muted":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return nil, fmt.Errorf("audio.muted: %q is not a valid boolean", value)
		}
		return scalar("!!bool", strconv.FormatBool(b)), nil
	case "session.max_lease":
		if value != "" {
			d, err := time.ParseDuration(value)
			if err != nil {
				return nil, fmt.Errorf("session.max_lease: %q is not a valid duration", value)
			}
			if d < 0 {
				return nil, fmt.Errorf("session.max_lease: %v must not be negative", d)
			}
		}
		return scalar("!!str", value), nil
	}
	return nil, fmt.Errorf("%w: %q", ErrUnknownKey, key)
}

func scalar(tag, value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tag, Value: value}
}

func mapping() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

func readDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping()}}, nil
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{mapping()}}, nil
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: is not a YAML mapping", path)
	}
	return &doc, nil
}

func setKey(node *yaml.Node, path []string, value *yaml.Node) {
	key := path[0]
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value != key {
			continue
		}
		existing := node.Content[i+1]
		if len(path) > 1 {
			if existing.Kind != yaml.MappingNode {
				*existing = *mapping()
			}
			setKey(existing, path[1:], value)
			return
		}
		value.HeadComment = existing.HeadComment
		value.LineComment = existing.LineComment
		value.FootComment = existing.FootComment
		node.Content[i+1] = value
		return
	}

	node.Content = append(node.Content, scalar("!!str", key))
	if len(path) == 1 {
		node.Content = append(node.Content, value)
		return
	}
	child := mapping()
	node.Content = append(node.Content, child)
	setKey(child, path[1:], value)
}

var osCreateTemp = os.CreateTemp

func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, configDirPerm); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	tmp, err := osCreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create a temporary file in %s: %w", dir, err)
	}
	name := tmp.Name()
	defer os.Remove(name)

	if err := flushAndClose(tmp, data); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}

func flushAndClose(f *os.File, data []byte) error {
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
