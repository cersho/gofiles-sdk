package files

type EnvVar struct {
	Key         string
	Aliases     []string
	Description string
	Secret      bool
	ReadBy      string
}

type EnvGroup struct {
	Label string
	Vars  []EnvVar
}

type ProviderEnvSpec struct {
	Config          []string
	CredentialModes []EnvGroup
	Required        []EnvVar
	Optional        []EnvVar
	Notes           string
}

type Provider struct {
	Slug        string
	Name        string
	Description string
	Env         ProviderEnvSpec
	PeerDeps    []string
}

var Providers = map[string]Provider{
	"digitalocean-spaces": {
		Slug:        "digitalocean-spaces",
		Name:        "DigitalOcean Spaces",
		Description: "DigitalOcean Spaces via the S3-compatible API.",
		Env: ProviderEnvSpec{
			Config: []string{"bucket", "region"},
			CredentialModes: []EnvGroup{{
				Label: "Access key",
				Vars: []EnvVar{
					{Key: "DO_SPACES_KEY", Description: "Spaces access key", Secret: true, ReadBy: "gofiles-sdk"},
					{Key: "DO_SPACES_SECRET", Description: "Spaces secret key", Secret: true, ReadBy: "gofiles-sdk"},
				},
			}},
		},
		PeerDeps: []string{"github.com/aws/aws-sdk-go-v2/service/s3"},
	},
	"fs": {
		Slug:        "fs",
		Name:        "Filesystem",
		Description: "Local filesystem storage for development, tests, and single-node deployments.",
		Env: ProviderEnvSpec{
			Config: []string{"root"},
			Notes:  "Stores metadata in sidecar JSON files beside objects.",
		},
	},
	"memory": {
		Slug:        "memory",
		Name:        "Memory",
		Description: "In-memory storage for tests and ephemeral workloads.",
		Env: ProviderEnvSpec{
			Notes: "No environment variables are required. Data is process-local and is lost when the process exits.",
		},
	},
	"r2": {
		Slug:        "r2",
		Name:        "Cloudflare R2",
		Description: "Cloudflare R2 via the S3-compatible HTTP API.",
		Env: ProviderEnvSpec{
			Config: []string{"bucket"},
			Required: []EnvVar{
				{Key: "R2_ACCOUNT_ID", Description: "Cloudflare account ID", Secret: false, ReadBy: "gofiles-sdk"},
			},
			CredentialModes: []EnvGroup{{
				Label: "Access key",
				Vars: []EnvVar{
					{Key: "R2_ACCESS_KEY_ID", Description: "R2 access key ID", Secret: true, ReadBy: "gofiles-sdk"},
					{Key: "R2_SECRET_ACCESS_KEY", Description: "R2 secret access key", Secret: true, ReadBy: "gofiles-sdk"},
				},
			}},
			Notes: "Workers binding mode is intentionally not part of the Go port.",
		},
		PeerDeps: []string{"github.com/aws/aws-sdk-go-v2/service/s3"},
	},
	"s3": {
		Slug:        "s3",
		Name:        "S3",
		Description: "AWS S3 and S3-compatible object stores.",
		Env: ProviderEnvSpec{
			Config: []string{"bucket"},
			Required: []EnvVar{
				{Key: "AWS_REGION", Aliases: []string{"AWS_DEFAULT_REGION"}, Description: "Bucket region", Secret: false, ReadBy: "gofiles-sdk"},
			},
			CredentialModes: []EnvGroup{{
				Label: "AWS SDK credential chain",
				Vars: []EnvVar{
					{Key: "AWS_ACCESS_KEY_ID", Description: "AWS access key ID", Secret: true, ReadBy: "sdk-chain"},
					{Key: "AWS_SECRET_ACCESS_KEY", Description: "AWS secret access key", Secret: true, ReadBy: "sdk-chain"},
				},
			}},
			Optional: []EnvVar{
				{Key: "AWS_SESSION_TOKEN", Description: "AWS session token", Secret: true, ReadBy: "sdk-chain"},
			},
		},
		PeerDeps: []string{"github.com/aws/aws-sdk-go-v2/service/s3"},
	},
	"s3-compatible": {
		Slug:        "s3-compatible",
		Name:        "S3 Compatible",
		Description: "Generic S3-compatible object store using a custom endpoint.",
		Env: ProviderEnvSpec{
			Config: []string{"bucket", "region", "endpoint"},
			CredentialModes: []EnvGroup{{
				Label: "Access key",
				Vars: []EnvVar{
					{Key: "S3_COMPATIBLE_ACCESS_KEY_ID", Description: "S3-compatible access key ID", Secret: true, ReadBy: "gofiles-sdk"},
					{Key: "S3_COMPATIBLE_SECRET_ACCESS_KEY", Description: "S3-compatible secret access key", Secret: true, ReadBy: "gofiles-sdk"},
				},
			}},
		},
		PeerDeps: []string{"github.com/aws/aws-sdk-go-v2/service/s3"},
	},
	"uploadthing": {
		Slug:        "uploadthing",
		Name:        "UploadThing",
		Description: "UploadThing UFS API using custom IDs as files-sdk keys.",
		Env: ProviderEnvSpec{
			CredentialModes: []EnvGroup{{
				Label: "API token",
				Vars: []EnvVar{
					{Key: "UPLOADTHING_TOKEN", Description: "UploadThing token", Secret: true, ReadBy: "gofiles-sdk"},
				},
			}},
		},
	},
	"vercel-blob": {
		Slug:        "vercel-blob",
		Name:        "Vercel Blob",
		Description: "Vercel Blob via the Blob HTTP API.",
		Env: ProviderEnvSpec{
			Config: []string{"access"},
			CredentialModes: []EnvGroup{
				{
					Label: "OIDC",
					Vars: []EnvVar{
						{Key: "VERCEL_OIDC_TOKEN", Description: "Vercel OIDC token", Secret: true, ReadBy: "gofiles-sdk"},
						{Key: "BLOB_STORE_ID", Description: "Vercel Blob store ID", Secret: false, ReadBy: "gofiles-sdk"},
					},
				},
				{
					Label: "Read-write token",
					Vars: []EnvVar{
						{Key: "BLOB_READ_WRITE_TOKEN", Description: "Vercel Blob read-write token", Secret: true, ReadBy: "gofiles-sdk"},
					},
				},
			},
			Notes: "OIDC is preferred on Vercel when both VERCEL_OIDC_TOKEN and BLOB_STORE_ID are available. Token authentication is used as the fallback.",
		},
	},
}

var ProviderNames = []string{"digitalocean-spaces", "fs", "memory", "r2", "s3", "s3-compatible", "uploadthing", "vercel-blob"}

func GetProvider(slug string) (Provider, bool) {
	provider, ok := Providers[slug]
	return provider, ok
}

func ListEnvVars(slug string) []EnvVar {
	provider, ok := Providers[slug]
	if !ok {
		return nil
	}
	seen := map[string]bool{}
	var vars []EnvVar
	add := func(v EnvVar) {
		if seen[v.Key] {
			return
		}
		seen[v.Key] = true
		vars = append(vars, v)
	}
	for _, v := range provider.Env.Required {
		add(v)
	}
	for _, group := range provider.Env.CredentialModes {
		for _, v := range group.Vars {
			add(v)
		}
	}
	for _, v := range provider.Env.Optional {
		add(v)
	}
	return vars
}

func GetSecretEnvVars(slug string) []EnvVar {
	all := ListEnvVars(slug)
	out := make([]EnvVar, 0, len(all))
	for _, v := range all {
		if v.Secret {
			out = append(out, v)
		}
	}
	return out
}
