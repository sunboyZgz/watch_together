package mediactl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// StorageUploader persists generated HLS assets to the configured backend.
type StorageUploader interface {
	Store(ctx context.Context, options IngestOptions, result HLSResult) (HLSResult, error)
}

func UploadIngestAssets(ctx context.Context, options IngestOptions, result HLSResult) (HLSResult, error) {
	uploader, err := newStorageUploader(options.Storage)
	if err != nil {
		return HLSResult{}, err
	}
	stored, err := uploader.Store(ctx, options, result)
	if err != nil {
		return HLSResult{}, err
	}
	return ApplyIngestPublicURLs(options, stored), nil
}

func ApplyIngestPublicURLs(options IngestOptions, result HLSResult) HLSResult {
	result.MediaURL = publicMediaURL(options)
	result.CoverURL = plannedCoverURL(options)
	for index := range result.Variants {
		result.Variants[index].PlaylistURL = publicVariantURL(options, result.Variants[index].Key)
	}
	return result
}

func isSupportedStorageDriver(driver string) bool {
	switch normalizedStorageDriver(driver) {
	case "local", "minio", "s3":
		return true
	default:
		return false
	}
}

func normalizedStorageDriver(driver string) string {
	return strings.ToLower(strings.TrimSpace(driver))
}

func newStorageUploader(config StorageConfig) (StorageUploader, error) {
	switch normalizedStorageDriver(config.Driver) {
	case "local", "":
		if strings.TrimSpace(config.LocalRoot) == "" {
			return nil, errors.New("MEDIA_LOCAL_ROOT is required for local storage driver")
		}
		return localStorageUploader{localRoot: config.LocalRoot}, nil
	case "minio", "s3":
		return newS3StorageUploader(config)
	default:
		return nil, fmt.Errorf("unsupported storage driver %q", config.Driver)
	}
}

type localStorageUploader struct {
	localRoot string
}

func (u localStorageUploader) Store(_ context.Context, options IngestOptions, result HLSResult) (HLSResult, error) {
	finalHLSDir := filepath.Join(u.localRoot, sourceObjectKey(options), "hls")
	if err := ensureLocalHLSFiles(result.OutputDir, finalHLSDir); err != nil {
		return HLSResult{}, err
	}

	result.OutputDir = finalHLSDir
	result.PlaylistPath = filepath.Join(finalHLSDir, "master.m3u8")
	if options.Cover == "" {
		return result, nil
	}

	finalCoverPath := filepath.Join(u.localRoot, sourceObjectKey(options), "cover", plannedCoverFilename(options.Cover))
	if err := copyFile(options.Cover, finalCoverPath); err != nil {
		return HLSResult{}, fmt.Errorf("store local cover: %w", err)
	}
	return result, nil
}

func ensureLocalHLSFiles(sourceDir string, targetDir string) error {
	if filepath.Clean(sourceDir) == filepath.Clean(targetDir) {
		return nil
	}
	if err := copyTree(sourceDir, targetDir); err != nil {
		return fmt.Errorf("store local hls tree: %w", err)
	}
	return nil
}

type s3StorageUploader struct {
	client   *s3.Client
	bucket   string
	endpoint string
}

func newS3StorageUploader(config StorageConfig) (StorageUploader, error) {
	if strings.TrimSpace(config.Endpoint) == "" {
		return nil, errors.New("MEDIA_STORAGE_ENDPOINT is required for minio/s3 storage driver")
	}
	if strings.TrimSpace(config.Bucket) == "" {
		return nil, errors.New("MEDIA_STORAGE_BUCKET is required for minio/s3 storage driver")
	}
	if strings.TrimSpace(config.Region) == "" {
		return nil, errors.New("MEDIA_STORAGE_REGION is required for minio/s3 storage driver")
	}
	if strings.TrimSpace(config.AccessKeyID) == "" {
		return nil, errors.New("MEDIA_STORAGE_ACCESS_KEY_ID is required for minio/s3 storage driver")
	}
	if strings.TrimSpace(config.SecretAccessKey) == "" {
		return nil, errors.New("MEDIA_STORAGE_SECRET_ACCESS_KEY is required for minio/s3 storage driver")
	}

	forcePathStyle, err := strconv.ParseBool(strings.TrimSpace(config.ForcePathStyle))
	if err != nil {
		return nil, fmt.Errorf("parse MEDIA_STORAGE_FORCE_PATH_STYLE: %w", err)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		context.Background(),
		awsconfig.WithRegion(strings.TrimSpace(config.Region)),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(config.AccessKeyID),
				strings.TrimSpace(config.SecretAccessKey),
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load s3-compatible config: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.UsePathStyle = forcePathStyle
		options.BaseEndpoint = aws.String(strings.TrimSpace(config.Endpoint))
	})
	return s3StorageUploader{
		client:   client,
		bucket:   strings.TrimSpace(config.Bucket),
		endpoint: strings.TrimSpace(config.Endpoint),
	}, nil
}

func (u s3StorageUploader) Store(ctx context.Context, options IngestOptions, result HLSResult) (HLSResult, error) {
	if err := uploadDirectoryTree(ctx, u.client, u.bucket, result.OutputDir, path.Join(sourceObjectKey(options), "hls")); err != nil {
		return HLSResult{}, err
	}
	if options.Cover != "" {
		coverKey := path.Join(sourceObjectKey(options), "cover", plannedCoverFilename(options.Cover))
		if err := uploadFile(ctx, u.client, u.bucket, options.Cover, coverKey); err != nil {
			return HLSResult{}, fmt.Errorf("upload remote cover: %w", err)
		}
	}
	return result, nil
}

func uploadDirectoryTree(ctx context.Context, client *s3.Client, bucket string, localRoot string, keyRoot string) error {
	return filepath.WalkDir(localRoot, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relativePath, err := filepath.Rel(localRoot, current)
		if err != nil {
			return fmt.Errorf("derive upload relative path: %w", err)
		}
		objectKey := path.Join(keyRoot, filepath.ToSlash(relativePath))
		if err := uploadFile(ctx, client, bucket, current, objectKey); err != nil {
			return fmt.Errorf("upload %s: %w", objectKey, err)
		}
		return nil
	})
}

func uploadFile(ctx context.Context, client *s3.Client, bucket string, localPath string, objectKey string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("open upload file %q: %w", localPath, err)
	}
	defer file.Close()

	contentType := detectContentType(localPath)
	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", objectKey, err)
	}
	return nil
}

func copyTree(sourceDir string, targetDir string) error {
	return filepath.WalkDir(sourceDir, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relativePath, err := filepath.Rel(sourceDir, current)
		if err != nil {
			return fmt.Errorf("derive relative copy path: %w", err)
		}
		destination := filepath.Join(targetDir, relativePath)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		return copyFile(current, destination)
	})
}

func copyFile(source string, target string) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source file: %w", err)
	}
	defer in.Close()

	out, err := os.Create(target)
	if err != nil {
		return fmt.Errorf("create target file: %w", err)
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy file contents: %w", err)
	}
	if err := out.Chmod(0o644); err != nil {
		return fmt.Errorf("chmod target file: %w", err)
	}
	return nil
}

func plannedCoverURL(options IngestOptions) string {
	if strings.TrimSpace(options.Cover) == "" {
		return ""
	}
	return joinURLPath(options.Storage.PublicBaseURL, sourceObjectKey(options), "cover", plannedCoverFilename(options.Cover))
}

func plannedCoverFilename(source string) string {
	extension := strings.ToLower(strings.TrimSpace(filepath.Ext(source)))
	if extension == "" {
		extension = ".jpg"
	}
	return "cover" + extension
}

func detectContentType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".m3u8":
		return "application/vnd.apple.mpegurl"
	case ".ts":
		return "video/mp2t"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		if detected := mime.TypeByExtension(filepath.Ext(path)); detected != "" {
			return detected
		}
		return "application/octet-stream"
	}
}
