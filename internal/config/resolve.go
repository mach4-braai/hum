package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/mach4-braai/hum/internal/paths"
)

type Layer string

const (
	LayerDefault Layer = "default"
	LayerGlobal  Layer = "global"
	LayerProject Layer = "project"
	LayerCLI     Layer = "cli"
)

type Provenance map[string]Layer

type layerProject struct {
	Name *string `yaml:"name"`
}

type layerMusic struct {
	Root  *string `yaml:"root"`
	Scale *string `yaml:"scale"`
	Theme *string `yaml:"theme"`
}

type layerAudio struct {
	Volume *float64 `yaml:"volume"`
	Muted  *bool    `yaml:"muted"`
}

type layerData struct {
	Project layerProject `yaml:"project"`
	Music   layerMusic   `yaml:"music"`
	Audio   layerAudio   `yaml:"audio"`
}

func loadLayer(path string) (*layerData, error) {
	f, err := osOpen(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var d layerData
	if err := dec.Decode(&d); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &d, nil
}

func applyLayer(out *Config, prov Provenance, d *layerData, layer Layer) {
	if d == nil {
		return
	}
	if d.Project.Name != nil {
		out.Project.Name = *d.Project.Name
		prov["project.name"] = layer
	}
	if d.Music.Root != nil {
		out.Music.Root = *d.Music.Root
		prov["music.root"] = layer
	}
	if d.Music.Scale != nil {
		out.Music.Scale = *d.Music.Scale
		prov["music.scale"] = layer
	}
	if d.Music.Theme != nil {
		out.Music.Theme = *d.Music.Theme
		prov["music.theme"] = layer
	}
	if d.Audio.Volume != nil {
		out.Audio.Volume = *d.Audio.Volume
		prov["audio.volume"] = layer
	}
	if d.Audio.Muted != nil {
		out.Audio.Muted = *d.Audio.Muted
		prov["audio.muted"] = layer
	}
}

var ErrProjectRoot = errors.New("invalid project root")

type Sources struct {
	GlobalFile  string
	ProjectFrom string
	UseProject  bool
	CLI         map[string]string
}

func CanonicalRoot(projectRoot string) (string, error) {
	if !filepath.IsAbs(projectRoot) {
		return "", fmt.Errorf("%w: %q is not an absolute path", ErrProjectRoot, projectRoot)
	}
	info, err := os.Stat(projectRoot)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrProjectRoot, projectRoot, err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%w: %q is not a directory", ErrProjectRoot, projectRoot)
	}
	resolved, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", fmt.Errorf("%w: %q: %w", ErrProjectRoot, projectRoot, err)
	}
	return resolved, nil
}

func ResolveForSession(globalFile, projectRoot string) (*Config, Provenance, error) {
	if projectRoot == "" {
		return ResolveSources(Sources{GlobalFile: globalFile})
	}
	canonical, err := CanonicalRoot(projectRoot)
	if err != nil {
		return nil, nil, err
	}
	return ResolveSources(Sources{GlobalFile: globalFile, ProjectFrom: canonical, UseProject: true})
}

func Resolve(cliOverrides map[string]string, startDir string) (*Config, Provenance, error) {
	return ResolveSources(Sources{ProjectFrom: startDir, UseProject: true, CLI: cliOverrides})
}

func ResolveSources(s Sources) (*Config, Provenance, error) {
	prov := Provenance{
		"project.name": LayerDefault,
		"music.root":   LayerDefault,
		"music.scale":  LayerDefault,
		"music.theme":  LayerDefault,
		"audio.volume": LayerDefault,
		"audio.muted":  LayerDefault,
	}

	out := Default()

	globalFile := s.GlobalFile
	if globalFile == "" {
		globalFile = paths.GlobalConfigFile()
	}
	global, err := loadLayer(globalFile)
	if err != nil {
		return nil, nil, err
	}
	applyLayer(&out, prov, global, LayerGlobal)

	if s.UseProject {
		if projPath, ok := paths.ProjectConfigFile(s.ProjectFrom); ok {
			proj, err := loadLayer(projPath)
			if err != nil {
				return nil, nil, err
			}
			applyLayer(&out, prov, proj, LayerProject)
		}
	}

	keys := make([]string, 0, len(s.CLI))
	for k := range s.CLI {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := s.CLI[k]
		switch k {
		case "project.name":
			out.Project.Name = v
			prov["project.name"] = LayerCLI
		case "music.root":
			out.Music.Root = v
			prov["music.root"] = LayerCLI
		case "music.scale":
			out.Music.Scale = v
			prov["music.scale"] = LayerCLI
		case "music.theme":
			out.Music.Theme = v
			prov["music.theme"] = LayerCLI
		case "audio.volume":
			fv, parseErr := strconv.ParseFloat(v, 64)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("audio.volume: %q is not a valid number", v)
			}
			if !(fv >= 0 && fv <= 1) {
				return nil, nil, fmt.Errorf("audio.volume: %v out of range [0, 1]", fv)
			}
			out.Audio.Volume = fv
			prov["audio.volume"] = LayerCLI
		case "audio.muted":
			b, parseErr := strconv.ParseBool(v)
			if parseErr != nil {
				return nil, nil, fmt.Errorf("audio.muted: %q is not a valid boolean", v)
			}
			out.Audio.Muted = b
			prov["audio.muted"] = LayerCLI
		default:
			return nil, nil, fmt.Errorf("unknown config key: %q", k)
		}
	}

	if err := out.Validate(); err != nil {
		return nil, nil, err
	}

	return &out, prov, nil
}
