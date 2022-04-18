package claims

import (
	"time"

	"get.porter.sh/porter/pkg/cnab"
	"get.porter.sh/porter/pkg/parameters"
	"get.porter.sh/porter/pkg/secrets"
	"get.porter.sh/porter/pkg/storage"
	"github.com/cnabio/cnab-go/bundle"
	"github.com/cnabio/cnab-go/schema"
)

var _ storage.Document = Run{}

// Run represents the execution of an installation's bundle.
type Run struct {
	// SchemaVersion of the document.
	SchemaVersion schema.Version `json:"schemaVersion" yaml:"schemaVersion" toml:"schemaVersion"`

	// ID of the Run.
	ID string `json:"_id" yaml:"_id", toml:"_id"`

	// Created timestamp of the Run.
	Created time.Time `json:"created" yaml:"created", toml:"created"`

	// Namespace of the installation.
	Namespace string `json:"namespace" yaml:"namespace", toml:"namespace"`

	// Installation name.
	Installation string `json:"installation" yaml:"installation", toml:"installation"`

	// Revision of the installation.
	Revision string `json:"revision" yaml:"revision", toml:"revision"`

	// Action executed against the installation.
	Action string `json:"action" yaml:"action", toml:"action"`

	// Bundle is the definition of the bundle.
	Bundle bundle.Bundle `json:"bundle" yaml:"bundle", toml:"bundle"`

	// BundleReference is the canonical reference to the bundle used in the action.
	BundleReference string `json:"bundleReference" yaml:"bundleReference", toml:"bundleReference"`

	// BundleDigest is the digest of the bundle.
	// TODO(carolynvs): populate this
	BundleDigest string `json:"bundleDigest" yaml:"bundleDigest", toml:"bundleDigest"`

	// ParameterOverrides are the key/value parameter overrides (taking precedence over
	// parameters specified in a parameter set) specified during the run.
	ParameterOverrides map[string]interface{} `json:"parameterOverrides" yaml:"parameterOverrides", toml:"parameterOverrides"`

	// CredentialSets is a list of the credential set names used during the run.
	CredentialSets []string `json:"credentialSets,omitempty" yaml:"credentialSets,omitempty" toml:"credentialSets,omitempty"`

	// ParameterSets is the list of parameter set names used during the run.
	ParameterSets []parameters.ParameterSet `json:"parameterSets,omitempty" yaml:"parameterSets,omitempty" toml:"parameterSets,omitempty"`

	// Parameters is the full set of resolved parameters stored on the claim.
	// This includes internal parameters, resolved parameter sources, values resolved from parameter sets, etc.
	Parameters map[string]interface{} `json:"-" yaml:"-" toml:"-"`

	// Custom extension data applicable to a given runtime.
	// TODO(carolynvs): remove custom and populate it in ToCNAB
	Custom interface{} `json:"custom" yaml:"custom", toml:"custom"`
}

func (r Run) DefaultDocumentFilter() interface{} {
	return map[string]interface{}{"_id": r.ID}
}

// NewRun creates a run with default values initialized.
func NewRun(namespace string, installation string) Run {
	return Run{
		SchemaVersion: SchemaVersion,
		ID:            cnab.NewULID(),
		Revision:      cnab.NewULID(),
		Created:       time.Now(),
		Namespace:     namespace,
		Installation:  installation,
	}
}

// ShouldRecord the current run in the Installation history.
// Runs are only recorded for actions that modify the bundle resources,
// or for stateful actions. Stateless actions do not require an existing
// installation or credentials, and are for actions such as documentation, dry-run, etc.
func (r Run) ShouldRecord() bool {
	// Assume all actions modify bundle resources, and should be recorded.
	stateful := true
	modifies := true

	if action, err := r.Bundle.GetAction(r.Action); err == nil {
		modifies = action.Modifies
		stateful = !action.Stateless
	}

	return modifies || stateful
}

// ToCNAB associated with the Run.
func (r Run) ToCNAB() cnab.Claim {
	return cnab.Claim{
		SchemaVersion: CNABSchemaVersion(),
		ID:            r.ID,
		// CNAB doesn't have the concept of namespace, so we smoosh them together to make a unique name
		Installation:    r.Namespace + "/" + r.Installation,
		Revision:        r.Revision,
		Created:         r.Created,
		Action:          r.Action,
		Bundle:          r.Bundle,
		BundleReference: r.BundleReference,
		Parameters:      r.Parameters,
		Custom:          r.Custom,
	}
}

// NewRun creates a result for the current Run.
func (r Run) NewResult(status string) Result {
	result := NewResult()
	result.RunID = r.ID
	result.Namespace = r.Namespace
	result.Installation = r.Installation
	result.Status = status
	return result
}

// NewResultFrom creates a result from the output of a CNAB run.
func (r Run) NewResultFrom(cnabResult cnab.Result) Result {
	return Result{
		SchemaVersion:  SchemaVersion,
		ID:             cnabResult.ID,
		Namespace:      r.Namespace,
		Installation:   r.Installation,
		RunID:          r.ID,
		Created:        cnabResult.Created,
		Status:         cnabResult.Status,
		Message:        cnabResult.Message,
		OutputMetadata: cnabResult.OutputMetadata,
		Custom:         cnabResult.Custom,
	}
}

// EncodeInternalParameterSet encodes sensitive data in internal parameter sets
// so it will be associated with the current run record.
// It returns the encoded parameter set and a boolean value indicating whether
// the operation is successful. The boolean value is set to false when no
// internal parameter can be found.
func (r *Run) EncodeInternalParameterSet() (parameters.ParameterSet, bool) {
	for i, pset := range r.ParameterSets {
		if !pset.IsInternalParameterSet() {
			continue
		}

		bun := cnab.ExtendedBundle{r.Bundle}

		for i, param := range pset.Parameters {
			if !bun.IsSensitiveParameter(param.Name) {
				continue
			}
			param.Source.Key = secrets.SourceSecret
			param.Source.Value = r.ID + param.Name
			pset.Parameters[i] = param
		}

		r.ParameterSets[i] = pset
		return pset, true
	}

	return parameters.ParameterSet{}, false

}

// ResolveSensitiveData resolves sensitive value on a run record.
// Currently, it's resolving sensitive parameter values.
func (r Run) ResolveSensitiveData(resolver parameters.Provider) (Run, error) {
	bun := cnab.ExtendedBundle{r.Bundle}

	resolved := make(map[string]interface{})
	for _, pset := range r.ParameterSets {
		params, err := pset.Resolve(resolver, bun)
		if err != nil {
			return r, err
		}
		for key, value := range params {
			resolved[key] = value
		}
	}

	r.Parameters = resolved
	return r, nil
}
