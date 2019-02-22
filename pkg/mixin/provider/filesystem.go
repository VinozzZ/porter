package mixinprovider

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/deislabs/porter/pkg/config"
	"github.com/deislabs/porter/pkg/context"
	"github.com/deislabs/porter/pkg/mixin"
	"github.com/pkg/errors"
)

func NewFileSystem(config *config.Config) *FileSystem {
	return &FileSystem{
		Config: config,
	}
}

type FileSystem struct {
	*config.Config
}

func (p *FileSystem) GetMixins() ([]mixin.Metadata, error) {
	mixinsDir, err := p.GetMixinsDir()
	if err != nil {
		return nil, err
	}

	files, err := p.FileSystem.ReadDir(mixinsDir)
	if err != nil {
		return nil, errors.Wrapf(err, "could not list the contents of the mixins directory %q", mixinsDir)
	}

	mixins := make([]mixin.Metadata, 0, len(files))
	for _, file := range files {
		if !file.IsDir() {
			continue
		}

		mixinDir := filepath.Join(mixinsDir, file.Name())
		mixins = append(mixins, mixin.Metadata{
			Name:       file.Name(),
			ClientPath: filepath.Join(mixinDir, file.Name()),
			Dir:        mixinDir,
		})
	}

	return mixins, nil
}

func (p *FileSystem) GetMixinSchema(m mixin.Metadata) (map[string]interface{}, error) {
	r := mixin.NewRunner(m.Name, m.Dir, false)
	r.Command = "schema"

	// Copy the existing context and tweak to pipe the output differently
	mixinSchema := &bytes.Buffer{}
	var mixinContext context.Context
	mixinContext = *p.Context
	mixinContext.Out = mixinSchema
	if !p.Debug {
		mixinContext.Err = ioutil.Discard
	}
	r.Context = &mixinContext

	err := r.Run()
	if err != nil {
		return nil, err
	}

	schemaMap := make(map[string]interface{})
	err = json.Unmarshal(mixinSchema.Bytes(), &schemaMap)
	if err != nil {
		return nil, errors.Wrapf(err, "could not unmarshal mixin schema for %s, %q", m.Name, mixinSchema.String())
	}

	return schemaMap, nil
}
