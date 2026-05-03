package bucketfill

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultConfigFile is the file LoadConfig looks for when no explicit path is given.
const DefaultConfigFile = "bucketfill.yaml"

// DefaultMigrationDir is the directory bucketfill walks for v<N>/ folders.
const DefaultMigrationDir = "migrations"

// Config is the runtime configuration shared between the lib and the CLI.
//
// Secrets (S3 access keys, GCS credential file paths) should come from env
// vars or flags, not from the YAML file. The loader emits a warning if it
// encounters secrets in YAML.
type Config struct {
	Provider     string `yaml:"provider"`
	Bucket       string `yaml:"bucket"`
	MigrationDir string `yaml:"migrationDir"`

	FS  *FSConfig  `yaml:"fs,omitempty"`
	GCS *GCSConfig `yaml:"gcs,omitempty"`
	S3  *S3Config  `yaml:"s3,omitempty"`
}

// FSConfig configures the local-filesystem provider. Root is the directory
// that holds the "bucket" — files are stored under <Root>/<Bucket>/<key>.
type FSConfig struct {
	Root string `yaml:"root"`
}

// GCSConfig configures the Google Cloud Storage provider.
type GCSConfig struct {
	ProjectID       string `yaml:"projectID"`
	CredentialsFile string `yaml:"credentialsFile"` // blank = ADC
}

// S3Config configures the AWS S3 (or S3-compatible) provider.
type S3Config struct {
	Region          string `yaml:"region"`
	Endpoint        string `yaml:"endpoint"`     // set for MinIO etc.
	UsePathStyle    bool   `yaml:"usePathStyle"` // MinIO often needs this
	AccessKeyID     string `yaml:"-"`            // env/flag only
	SecretAccessKey string `yaml:"-"`            // env/flag only
}

// Overrides are values supplied by CLI flags. Empty fields are not applied.
type Overrides struct {
	ConfigFile   string
	Provider     string
	Bucket       string
	MigrationDir string
}

// LoadConfig builds a Config by layering: defaults < bucketfill.yaml < env < flags.
// configPath="" uses DefaultConfigFile if it exists, otherwise starts from defaults.
func LoadConfig(o Overrides) (*Config, error) {
	cfg := &Config{
		Provider:     "fs",
		MigrationDir: DefaultMigrationDir,
		FS:           &FSConfig{Root: "./local-bucket"},
	}

	configPath := o.ConfigFile
	if configPath == "" {
		configPath = DefaultConfigFile
	}
	if data, err := os.ReadFile(configPath); err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("bucketfill: parse %s: %w", configPath, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) || o.ConfigFile != "" {
		// Missing default file is fine; missing explicitly-provided file is an error.
		if o.ConfigFile != "" {
			return nil, fmt.Errorf("bucketfill: read %s: %w", configPath, err)
		}
	}

	applyEnv(cfg)
	applyOverrides(cfg, o)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate ensures required fields are present for the chosen provider.
func (c *Config) Validate() error {
	switch c.Provider {
	case "fs":
		if c.FS == nil || c.FS.Root == "" {
			return errors.New("bucketfill: fs provider requires fs.root")
		}
	case "gcs":
		if c.Bucket == "" {
			return errors.New("bucketfill: gcs provider requires bucket")
		}
	case "s3":
		if c.Bucket == "" {
			return errors.New("bucketfill: s3 provider requires bucket")
		}
		if c.S3 == nil || c.S3.Region == "" {
			return errors.New("bucketfill: s3 provider requires s3.region")
		}
	default:
		return fmt.Errorf("bucketfill: unknown provider %q (want fs|gcs|s3)", c.Provider)
	}
	if c.MigrationDir == "" {
		return errors.New("bucketfill: migrationDir is required")
	}
	return nil
}

func applyEnv(c *Config) {
	if v := os.Getenv("BUCKETFILL_PROVIDER"); v != "" {
		c.Provider = v
	}
	if v := os.Getenv("BUCKETFILL_BUCKET"); v != "" {
		c.Bucket = v
	}
	if v := os.Getenv("BUCKETFILL_MIGRATION_DIR"); v != "" {
		c.MigrationDir = v
	}

	if v := os.Getenv("BUCKETFILL_FS_ROOT"); v != "" {
		if c.FS == nil {
			c.FS = &FSConfig{}
		}
		c.FS.Root = v
	}

	if v := os.Getenv("BUCKETFILL_GCS_PROJECT_ID"); v != "" {
		if c.GCS == nil {
			c.GCS = &GCSConfig{}
		}
		c.GCS.ProjectID = v
	}
	if v := os.Getenv("BUCKETFILL_GCS_CREDENTIALS_FILE"); v != "" {
		if c.GCS == nil {
			c.GCS = &GCSConfig{}
		}
		c.GCS.CredentialsFile = v
	}

	if v := os.Getenv("BUCKETFILL_S3_REGION"); v != "" {
		if c.S3 == nil {
			c.S3 = &S3Config{}
		}
		c.S3.Region = v
	}
	if v := os.Getenv("BUCKETFILL_S3_ENDPOINT"); v != "" {
		if c.S3 == nil {
			c.S3 = &S3Config{}
		}
		c.S3.Endpoint = v
	}
	if v := os.Getenv("BUCKETFILL_S3_USE_PATH_STYLE"); v != "" {
		if c.S3 == nil {
			c.S3 = &S3Config{}
		}
		if b, err := strconv.ParseBool(v); err == nil {
			c.S3.UsePathStyle = b
		}
	}
	if v := os.Getenv("BUCKETFILL_S3_ACCESS_KEY_ID"); v != "" {
		if c.S3 == nil {
			c.S3 = &S3Config{}
		}
		c.S3.AccessKeyID = v
	}
	if v := os.Getenv("BUCKETFILL_S3_SECRET_ACCESS_KEY"); v != "" {
		if c.S3 == nil {
			c.S3 = &S3Config{}
		}
		c.S3.SecretAccessKey = v
	}
	// Standard AWS env names also accepted as fallbacks.
	if c.S3 != nil {
		if c.S3.AccessKeyID == "" {
			c.S3.AccessKeyID = os.Getenv("AWS_ACCESS_KEY_ID")
		}
		if c.S3.SecretAccessKey == "" {
			c.S3.SecretAccessKey = os.Getenv("AWS_SECRET_ACCESS_KEY")
		}
		if c.S3.Region == "" {
			c.S3.Region = os.Getenv("AWS_REGION")
		}
	}
}

func applyOverrides(c *Config, o Overrides) {
	if o.Provider != "" {
		c.Provider = o.Provider
	}
	if o.Bucket != "" {
		c.Bucket = o.Bucket
	}
	if o.MigrationDir != "" {
		c.MigrationDir = strings.TrimRight(o.MigrationDir, "/")
	}
}
