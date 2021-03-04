package public

import (
	"testing"

	"github.com/stretchr/testify/assert"
	gqlpublicapi "projectvoltron.dev/voltron/pkg/och/api/graphql/public"
)

func TestSortImplementationRevisionsByPathAndRevision(t *testing.T) {
	// given
	expRevision := []gqlpublicapi.ImplementationRevision{
		fixImplementationRevision("path1", "0.3.0"),
		fixImplementationRevision("path1", "0.2.0"),
		fixImplementationRevision("path1", "0.1.0"),
		fixImplementationRevision("path2", "1.0.0"),
		fixImplementationRevision("path2", "0.1.0"),
		fixImplementationRevision("path3", "0.1.0"),
		{Metadata: nil},
	}

	revisionToSort := []gqlpublicapi.ImplementationRevision{
		{Metadata: nil},
		fixImplementationRevision("path1", "0.1.0"),
		fixImplementationRevision("path1", "0.3.0"),
		fixImplementationRevision("path3", "0.1.0"),
		fixImplementationRevision("path2", "0.1.0"),
		fixImplementationRevision("path2", "1.0.0"),
		fixImplementationRevision("path1", "0.2.0"),
	}

	getOpts := &ListImplementationRevisionsOptions{}
	getOpts.Apply(WithSortingByPathAscAndRevisionDesc())

	// when
	gotRevs := SortImplementationRevisions(revisionToSort, getOpts)

	// then
	assert.Equal(t, gotRevs, expRevision)
}
