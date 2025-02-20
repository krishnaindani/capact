package local

import "fmt"

var typeInstanceWithUsesFields = fmt.Sprintf(`
		%s
		uses {
			%s
		}
		usedBy {
			%s
		}
`, typeInstanceFields, typeInstanceFields, typeInstanceFields)

var typeInstanceFields = fmt.Sprintf(`
		id
		typeRef {
			path
			revision
		}
		lockedBy

		latestResourceVersion {
			%s
		}

		firstResourceVersion {
			%s
		}
	
		previousResourceVersion {
			%s
		}
	
		resourceVersions {
			%s
		}
	
		resourceVersion(resourceVersion: 1) {
			%s
		}
`, typeInstanceResourceVersion, typeInstanceResourceVersion, typeInstanceResourceVersion, typeInstanceResourceVersion, typeInstanceResourceVersion)

const typeInstanceResourceVersion = `
		resourceVersion
		createdBy
		metadata {
			attributes {
				path
				revision
			}
		}
		spec {
			value
		}
`
