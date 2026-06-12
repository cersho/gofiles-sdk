package files

import "testing"

func TestProviderCatalogIncludesPortedProviders(t *testing.T) {
	for _, slug := range []string{"memory", "fs", "s3-compatible", "vercel-blob"} {
		if _, ok := GetProvider(slug); !ok {
			t.Fatalf("provider catalog is missing %q", slug)
		}
	}
}
