package vehicle

import (
	"fmt"
	"strings"
	"time"

	"github.com/andig/evcc/api"
	"github.com/andig/evcc/server/config"
)

const interval = 15 * time.Minute

type typeDesc struct {
	factory func(map[string]interface{}) (api.Vehicle, error)
	config  config.Type
}

type typeRegistry map[string]typeDesc

var registry = make(typeRegistry)

// Types exports the public configuration types
func Types() (types []config.Type) {
	for _, typ := range registry {
		if typ.config.Config != nil {
			types = append(types, typ.config)
		}
	}

	return types
}

func GetConfig(name string) (config.Type, error) {
	desc, err := registry.Get(name)
	if err == nil {
		return desc.config, err
	}

	return config.Type{}, err
}

func (r typeRegistry) Add(name, label string, factory func(map[string]interface{}) (api.Vehicle, error), defaults interface{}) {
	if _, exists := r[name]; exists {
		panic(fmt.Sprintf("cannot register duplicate vehicle type: %s", name))
	}

	desc := typeDesc{
		factory: factory,
		config: config.Type{
			Type:   name,
			Label:  label,
			Config: defaults,
		},
	}

	r[name] = desc
}

func (r typeRegistry) Get(name string) (typeDesc, error) {
	desc, exists := r[name]
	if !exists {
		return typeDesc{}, fmt.Errorf("vehicle type not registered: %s", name)
	}
	return desc, nil
}

// NewFromConfig creates vehicle from configuration
func NewFromConfig(typ string, other map[string]interface{}) (v api.Vehicle, err error) {
	desc, err := registry.Get(strings.ToLower(typ))
	if err == nil {
		if v, err = desc.factory(other); err != nil {
			err = fmt.Errorf("cannot create type '%s': %w", typ, err)
		}
	} else {
		err = fmt.Errorf("invalid vehicle type: %s", typ)
	}

	return
}
