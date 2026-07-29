package theme

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/mach4-braai/hum/internal/paths"
)

//go:embed themes/*.yaml
var embeddedThemes embed.FS

var readEmbedded = embeddedThemes.ReadFile

var ErrInvalidName = errors.New("invalid theme name")

func checkName(name string) error {
	if name == "" || name == "." || name == ".." {
		return fmt.Errorf("%w: %q", ErrInvalidName, name)
	}
	if name != filepath.Base(name) || strings.ContainsRune(name, '/') || strings.ContainsRune(name, filepath.Separator) {
		return fmt.Errorf("%w: %q must not name a path", ErrInvalidName, name)
	}
	return nil
}

func Load(name string) (Theme, error) {
	if err := checkName(name); err != nil {
		return Theme{}, err
	}

	userPath := filepath.Join(paths.GlobalConfigDir(), "themes", name+".yaml")
	data, err := os.ReadFile(userPath)
	if err == nil {
		t, decErr := decodeYAML(data)
		if decErr != nil {
			return Theme{}, fmt.Errorf("user theme %q (%s): %w", name, userPath, decErr)
		}
		return t, nil
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return Theme{}, fmt.Errorf("read user theme %q: %w", name, err)
	}

	embedded, embErr := readEmbedded("themes/" + name + ".yaml")
	if embErr != nil {
		return Theme{}, fmt.Errorf("unknown theme %q; available: %s", name, strings.Join(List(), ", "))
	}
	t, decErr := decodeYAML(embedded)
	if decErr != nil {
		return Theme{}, fmt.Errorf("embedded theme %q: %w", name, decErr)
	}
	return t, nil
}

func List() []string {
	names := make(map[string]struct{})

	entries, _ := fs.ReadDir(embeddedThemes, "themes")
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
			names[strings.TrimSuffix(e.Name(), ".yaml")] = struct{}{}
		}
	}

	userDir := filepath.Join(paths.GlobalConfigDir(), "themes")
	if dirEntries, err := os.ReadDir(userDir); err == nil {
		for _, e := range dirEntries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".yaml") {
				names[strings.TrimSuffix(e.Name(), ".yaml")] = struct{}{}
			}
		}
	}

	result := make([]string, 0, len(names))
	for n := range names {
		result = append(result, n)
	}
	sort.Strings(result)
	return result
}

func decodeYAML(data []byte) (Theme, error) {
	var t Theme
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&t); err != nil {
		return Theme{}, err
	}
	return t, t.Validate()
}
