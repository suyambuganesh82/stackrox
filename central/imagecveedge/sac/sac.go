package sac

import (
	"github.com/stackrox/rox/central/dackbox"
	"github.com/stackrox/rox/central/role/resources"
	"github.com/stackrox/rox/pkg/sac"
	"github.com/stackrox/rox/pkg/search/filtered"
	"github.com/stackrox/rox/pkg/sync"
	"github.com/stackrox/rox/pkg/utils"
)

var (
	imageSAC = sac.ForResource(resources.Image)

	imageCVEEdgeSACFilter filtered.Filter
	once                  sync.Once
)

// GetSACFilter returns the sac filter for image cve edge ids.
func GetSACFilter() filtered.Filter {
	once.Do(func() {
		var err error
		imageCVEEdgeSACFilter, err = filtered.NewSACFilter(
			filtered.WithResourceHelper(imageSAC),
			filtered.WithScopeTransform(dackbox.ImageCVEEdgeSACTransform),
			filtered.WithReadAccess(),
		)
		utils.Must(err)
	})
	return imageCVEEdgeSACFilter
}
