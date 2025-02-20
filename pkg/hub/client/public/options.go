package public

import gqlpublicapi "capact.io/capact/pkg/hub/api/graphql/public"

// ListImplementationRevisionsOptions stores Implementation Revision filtering parameters.
type ListImplementationRevisionsOptions struct {
	attrFilter                   map[gqlpublicapi.FilterRule]map[string]*string
	implPathPattern              *string
	requirementsSatisfiedBy      map[string]*string
	requires                     map[string]*string
	sortByPathAscAndRevisionDesc bool
}

// Apply is used to configure the ListImplementationRevisionsOptions.
func (o *ListImplementationRevisionsOptions) Apply(opts ...GetImplementationOption) {
	for _, opt := range opts {
		opt(o)
	}
}

// GetImplementationOption provides an option to configure the get request for Implementations.
type GetImplementationOption func(*ListImplementationRevisionsOptions)

// WithFilter returns an options, which adds a filter for ImplementationRevisions.
func WithFilter(filter gqlpublicapi.ImplementationRevisionFilter) GetImplementationOption {
	return func(opt *ListImplementationRevisionsOptions) {
		// 1. Process attributes
		opt.attrFilter = map[gqlpublicapi.FilterRule]map[string]*string{}

		for _, attr := range filter.Attributes {
			if attr == nil || attr.Rule == nil {
				continue
			}

			if opt.attrFilter[*attr.Rule] == nil {
				opt.attrFilter[*attr.Rule] = map[string]*string{}
			}

			opt.attrFilter[*attr.Rule][attr.Path] = attr.Revision
		}

		// 2. Process path pattern
		opt.implPathPattern = filter.PathPattern

		// 3. Process TypeInstances, which should satisfy requirements
		if len(filter.RequirementsSatisfiedBy) > 0 {
			opt.requirementsSatisfiedBy = map[string]*string{}
			for _, req := range filter.RequirementsSatisfiedBy {
				if req.TypeRef == nil {
					continue
				}
				opt.requirementsSatisfiedBy[req.TypeRef.Path] = req.TypeRef.Revision
			}
		}

		// 4. Process TypeInstances, which should be defined in `requires` section
		if len(filter.Requires) > 0 {
			opt.requires = map[string]*string{}
			for _, req := range filter.Requires {
				if req == nil {
					continue
				}
				opt.requires[req.Path] = req.Revision
			}
		}
	}
}

// WithSortingByPathAscAndRevisionDesc returns an options, which ensures
// that the returned ImplementationRevision slice will be sorted
// in ascending order by the Implementation path
// and descending by the Implementation revision.
func WithSortingByPathAscAndRevisionDesc() GetImplementationOption {
	return func(options *ListImplementationRevisionsOptions) {
		options.sortByPathAscAndRevisionDesc = true
	}
}
