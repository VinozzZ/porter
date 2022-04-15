package sanitizer

import (
	"get.porter.sh/porter/pkg/claims"
	"get.porter.sh/porter/pkg/cnab"
	"get.porter.sh/porter/pkg/parameters"
	"get.porter.sh/porter/pkg/secrets"
	"github.com/pkg/errors"
)

// Service for sanitizing sensitive data.
type Service struct {
	parameter parameters.Provider
	secrets   secrets.Store
}

// NewService creates a new service for sanitizing sensitive data and save them
// to a secret store.
func NewService(parameterstore parameters.Provider, secretstore secrets.Store) *Service {
	return &Service{
		parameter: parameterstore,
		secrets:   secretstore,
	}
}

// RawParameters clears out sensitive data in raw parameter values before
// transform the raw value into secret strategies.
func (s *Service) RawParameters(params map[string]interface{}, bun cnab.ExtendedBundle, id string) ([]secrets.Strategy, error) {
	strategies := make([]secrets.Strategy, 0, len(params))
	for name, value := range params {
		stringVal, err := bun.WriteParameterToString(name, value)
		if err != nil {
			return nil, err
		}
		strategy := parameters.DefaultStrategy(name, stringVal)
		strategies = append(strategies, strategy)
	}

	strategies, err := s.Parameters(strategies, bun, id)
	if err != nil {
		return nil, err
	}

	return strategies, nil

}

// Parameters clears out sensitive data in strategized parameter data and return
// sanitized value after saving sensitive datat to secrets store.
func (s *Service) Parameters(params []secrets.Strategy, bun cnab.ExtendedBundle, id string) ([]secrets.Strategy, error) {
	strategies := make([]secrets.Strategy, 0, len(params))
	for _, param := range params {

		strategy := parameters.DefaultStrategy(param.Name, param.Value)
		if bun.IsSensitiveParameter(param.Name) {
			encodedStrategy := encodeSecretParam(strategy, id)
			err := s.secrets.Create(encodedStrategy.Source.Key, encodedStrategy.Source.Value, encodedStrategy.Value)
			if err != nil {
				return nil, errors.Wrap(err, "failed to save sensitive param to secrete store")
			}
			strategy = encodedStrategy
		}

		strategies = append(strategies, strategy)
	}

	if len(strategies) == 0 {
		return nil, nil
	}

	return strategies, nil

}

func (s *Service) ResolveParameterSet(pset parameters.ParameterSet, bun cnab.ExtendedBundle) (map[string]interface{}, error) {
	params, err := s.parameter.ResolveAll(pset)
	if err != nil {
		return nil, err
	}

	resolved := make(map[string]interface{})
	for name, value := range params {
		paramValue, err := bun.ConvertParameterValue(name, value)
		if err != nil {
			paramValue = value
		}

		resolved[name] = paramValue

	}
	return resolved, nil

}

func (s *Service) Output(output claims.Output, bun cnab.ExtendedBundle) (claims.Output, error) {
	sensitive, err := bun.IsOutputSensitive(output.Name)
	if err != nil || !sensitive {
		return output, err
	}

	secretOt := encodeOutput(output)

	err = s.secrets.Create(secrets.SourceSecret, secretOt.Key, string(output.Value))
	if err != nil {
		return secretOt, err
	}

	return secretOt, nil
}

func encodeOutput(output claims.Output) claims.Output {
	output.Key = output.RunID + output.Name
	output.Value = []byte{}
	return output

}

func (s *Service) ResolveOutputs(o claims.Outputs, bun cnab.ExtendedBundle) (claims.Outputs, error) {
	resolved := make([]claims.Output, 0, o.Len())
	for _, ot := range o.Value() {
		sensitive, err := bun.IsOutputSensitive(ot.Name)
		if err != nil || !sensitive {
			resolved = append(resolved, ot)
			continue
		}

		r, err := s.ResolveOutput(ot)
		if err != nil {
			return o, errors.WithMessagef(err, "failed to resolve output %q using key %q", ot.Name, ot.Key)
		}
		resolved = append(resolved, r)
	}

	return claims.NewOutputs(resolved), nil
}

func (s *Service) ResolveOutput(output claims.Output) (claims.Output, error) {
	resolved, err := s.secrets.Resolve(secrets.SourceSecret, string(output.Key))
	if err != nil {
		return output, err
	}

	output.Value = []byte(resolved)
	return output, nil
}

func encodeSecretParam(param secrets.Strategy, id string) secrets.Strategy {
	param.Source.Key = secrets.SourceSecret
	param.Source.Value = id + param.Name
	return param
}
