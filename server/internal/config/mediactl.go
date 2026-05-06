package config

type MediaToolConfig struct {
	FFmpegBin  string
	FFprobeBin string
}

type MediaStorageConfig struct {
	Driver          string
	LocalRoot       string
	PublicBaseURL   string
	ObjectKeyPrefix string
	Endpoint        string
	Bucket          string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	ForcePathStyle  string
}

type MediactlConfig struct {
	DatabaseURL string
	Storage     MediaStorageConfig
	Tools       MediaToolConfig
}

func (c MediactlConfig) LookupEnv(name string) string {
	switch name {
	case "DATABASE_URL":
		return c.DatabaseURL
	case "MEDIA_STORAGE_DRIVER":
		return c.Storage.Driver
	case "MEDIA_LOCAL_ROOT":
		return c.Storage.LocalRoot
	case "MEDIA_PUBLIC_BASE_URL":
		return c.Storage.PublicBaseURL
	case "MEDIA_OBJECT_KEY_PREFIX":
		return c.Storage.ObjectKeyPrefix
	case "MEDIA_STORAGE_ENDPOINT":
		return c.Storage.Endpoint
	case "MEDIA_STORAGE_BUCKET":
		return c.Storage.Bucket
	case "MEDIA_STORAGE_REGION":
		return c.Storage.Region
	case "MEDIA_STORAGE_ACCESS_KEY_ID":
		return c.Storage.AccessKeyID
	case "MEDIA_STORAGE_SECRET_ACCESS_KEY":
		return c.Storage.SecretAccessKey
	case "MEDIA_STORAGE_FORCE_PATH_STYLE":
		return c.Storage.ForcePathStyle
	case "FFMPEG_BIN":
		return c.Tools.FFmpegBin
	case "FFPROBE_BIN":
		return c.Tools.FFprobeBin
	default:
		return ""
	}
}

func LoadMediactlConfig(configDir string) (MediactlConfig, error) {
	defaults := map[string]any{
		"MEDIA_STORAGE_DRIVER":           "local",
		"MEDIA_LOCAL_ROOT":               "../media/tmp",
		"MEDIA_PUBLIC_BASE_URL":          "http://127.0.0.1:9000/media/tmp",
		"MEDIA_OBJECT_KEY_PREFIX":        "media",
		"MEDIA_STORAGE_FORCE_PATH_STYLE": "true",
		"FFMPEG_BIN":                     "ffmpeg",
		"FFPROBE_BIN":                    "ffprobe",
	}
	keys := []string{
		"DATABASE_URL",
		"MEDIA_STORAGE_DRIVER",
		"MEDIA_LOCAL_ROOT",
		"MEDIA_PUBLIC_BASE_URL",
		"MEDIA_OBJECT_KEY_PREFIX",
		"MEDIA_STORAGE_ENDPOINT",
		"MEDIA_STORAGE_BUCKET",
		"MEDIA_STORAGE_REGION",
		"MEDIA_STORAGE_ACCESS_KEY_ID",
		"MEDIA_STORAGE_SECRET_ACCESS_KEY",
		"MEDIA_STORAGE_FORCE_PATH_STYLE",
		"FFMPEG_BIN",
		"FFPROBE_BIN",
	}

	loader, err := newLoader(configDir, defaults, keys)
	if err != nil {
		return MediactlConfig{}, err
	}

	return MediactlConfig{
		DatabaseURL: trimmedString(loader, "DATABASE_URL"),
		Storage: MediaStorageConfig{
			Driver:          trimmedString(loader, "MEDIA_STORAGE_DRIVER"),
			LocalRoot:       trimmedString(loader, "MEDIA_LOCAL_ROOT"),
			PublicBaseURL:   trimmedString(loader, "MEDIA_PUBLIC_BASE_URL"),
			ObjectKeyPrefix: trimmedString(loader, "MEDIA_OBJECT_KEY_PREFIX"),
			Endpoint:        trimmedString(loader, "MEDIA_STORAGE_ENDPOINT"),
			Bucket:          trimmedString(loader, "MEDIA_STORAGE_BUCKET"),
			Region:          trimmedString(loader, "MEDIA_STORAGE_REGION"),
			AccessKeyID:     trimmedString(loader, "MEDIA_STORAGE_ACCESS_KEY_ID"),
			SecretAccessKey: trimmedString(loader, "MEDIA_STORAGE_SECRET_ACCESS_KEY"),
			ForcePathStyle:  trimmedString(loader, "MEDIA_STORAGE_FORCE_PATH_STYLE"),
		},
		Tools: MediaToolConfig{
			FFmpegBin:  trimmedString(loader, "FFMPEG_BIN"),
			FFprobeBin: trimmedString(loader, "FFPROBE_BIN"),
		},
	}, nil
}
