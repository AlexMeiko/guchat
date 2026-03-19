package generator

import (
	"github.com/AlexMeiko/guchat/internal/entity"
	"github.com/AlexMeiko/guchat/internal/service"
)

type Factory struct {
	generators map[string]service.Generator
}

func NewFactory(generators map[string]service.Generator) *Factory {
	return &Factory{
		generators: generators,
	}
}

func (f *Factory) Get(model *entity.ModelConfig) (service.Generator, error) {
	generator, ok := f.generators[model.Provider]
	if !ok {
		return nil, service.ErrUnsupportedModelProvider
	}

	return generator, nil
}
