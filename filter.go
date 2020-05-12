// The iofl package provides configurable I/O filter chains.
package iofl

import (
	"errors"
	"fmt"
	"io"
)

// Closed is returned by a filter that has been closed.
var Closed = errors.New("closed")

// Config configures a ChainSet.
type Config struct {
	// Chains maps a name to a Chain.
	Chains map[string]Chain
}

// Chain defines a list of Filters that are to be applied in order.
type Chain []LinkDef

// LinkDef specifies a Filter to be used in a Chain, and describes its
// configuration.
type LinkDef struct {
	// Filter is the name of the Filter registered with a ChainSet.
	Filter string
	// Params configure the Filter.
	Params Params
}

// Params contains a set of parameters that configure a Filter.
type Params map[string]interface{}

// GetString returns the value of key as a string, or an empty string if the key
// is not present or the value is not a string.
func (p Params) GetString(key string) string {
	v, _ := p[key].(string)
	return v
}

// GetInt returns the value of key as an int, or 0 if the key is not present or
// the value is not a number.
func (p Params) GetInt(key string) int {
	// TODO: all numbers.
	v, _ := p[key].(float64)
	return int(v)
}

// Filter is implemented by any value that reads from an underlying source while
// being read. The Close method must close the Source.
type Filter interface {
	io.ReadCloser
	// Source returns the source from which the Filter is reading, or nil if
	// there is no source.
	Source() io.ReadCloser
}

// Root wraps a general io.ReadCloser to be used as a Filter by returning a nil
// source.
type Root struct {
	io.ReadCloser
}

// Source implements Filter. Returns nil.
func (Root) Source() io.ReadCloser { return nil }

// NewFilter returns a new Filter, configured by the given parameters. An
// optional io.ReadCloser specifies the source from which data will be read.
// NewFilter may ignore the io.ReadCloser, or return an error if an
// io.ReadCloser is required.
type NewFilter func(params Params, r io.ReadCloser) (f Filter, err error)

// ChainSet contains Filters, and Chains composed of those Filters.
type ChainSet struct {
	registry map[string]NewFilter
	config   Config
}

// FilterDef describes a filter to be added to a ChainSet.
type FilterDef struct {
	Name string
	New  NewFilter
}

// NewChainSet returns a ChainSet registered with the given filter definitions.
// Panics if registration returns an error.
func NewChainSet(filters ...FilterDef) *ChainSet {
	s := &ChainSet{}
	for _, filter := range filters {
		s.MustRegister(filter)
	}
	return s
}

// Register registers a filter definition. Returns an error if the filter of the
// given name already exists.
func (s *ChainSet) Register(filter FilterDef) error {
	if s.registry[filter.Name] != nil {
		return fmt.Errorf("filter %q already registered", filter.Name)
	}
	if s.registry == nil {
		s.registry = map[string]NewFilter{}
	}
	s.registry[filter.Name] = filter.New
	return nil
}

// MustRegister behaves the same as Register, but panics if an error occurs.
func (s *ChainSet) MustRegister(filter FilterDef) {
	if err := s.Register(filter); err != nil {
		panic(err)
	}
}

// Configure sets the configuration to be used by the ChainSet.
func (s *ChainSet) Configure(config Config) error {
	s.config = config
	return nil
}

// MustConfigure behaves the same as Configure, but panics if an error occurs.
// Returns the ChainSet.
func (s *ChainSet) MustConfigure(config Config) *ChainSet {
	s.config = config
	return nil
}

// Resolve locates the chain of the given name, and produces a Filter that
// recursively applies all filters in the chain. If vars is non-nil, then any
// Filters that implement Expander will be called with vars. If src is non-nil,
// then it will be used as the source of the first filter in the chain.
func (s *ChainSet) Resolve(chain string, src io.ReadCloser) (filter Filter, err error) {
	filterChain, ok := s.config.Chains[chain]
	if !ok {
		return nil, fmt.Errorf("unknown chain %q", chain)
	}
	if f, ok := src.(Filter); ok {
		filter = f
	} else {
		filter = Root{src}
	}
	for i, def := range filterChain {
		newFilter, ok := s.registry[def.Filter]
		if !ok {
			return nil, fmt.Errorf("%s[%d]: unknown filter %q", chain, i, def.Filter)
		}
		if filter, err = newFilter(def.Params, filter); err != nil {
			return nil, fmt.Errorf("%s[%d]%s: %w", chain, i, def.Filter, err)
		}
	}
	return filter, nil
}

// Apply calls cb for each io.ReadCloser that implements Filter. The filter's
// chain is traversed upward until a non-Filter is found. If cb returns and
// error, that error is returned by Apply.
func Apply(r io.ReadCloser, cb func(io.ReadCloser) error) error {
	for r != nil {
		if err := cb(r); err != nil {
			return err
		}
		if f, ok := r.(Filter); ok {
			r = f.Source()
		} else {
			break
		}
	}
	return nil
}
