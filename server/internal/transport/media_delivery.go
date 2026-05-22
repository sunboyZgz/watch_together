package transport

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const nginxMediaCookieName = "wt_media_access"

var errMediaDeliveryUnavailable = errors.New("media delivery mode is not configured")

/*
@description:
	客户端请求播放
	↓
	服务端确认权限
	↓
	生成临时播放入口
	↓
	客户端访问 /media/playback/{episodeID}/master.m3u8?expires=...&sig=...
	↓
	mediaDelivery 根据配置决定怎么返回真实媒体资源
*/

type mediaDelivery struct {
	mode          string
	config        MediaPlaybackConfig
	signer        *mediaPlaybackSigner
	s3Client      *s3.Client
	s3Presign     *s3.PresignClient
	presignTTL    time.Duration
	publicBaseURL string
	initErr       error
}

func newMediaDelivery(config MediaPlaybackConfig) *mediaDelivery {
	config.DeliveryMode = normalizeMediaDeliveryMode(config.DeliveryMode)
	signer := newMediaPlaybackSigner(config)
	delivery := &mediaDelivery{
		mode:          config.DeliveryMode,
		config:        config,
		signer:        signer,
		presignTTL:    signer.ttl,
		publicBaseURL: strings.TrimRight(strings.TrimSpace(config.PublicBaseURL), "/"),
	}
	if delivery.mode == MediaDeliveryMinIOPresign {
		client, err := newMediaDeliveryS3Client(context.Background(), config)
		if err != nil {
			delivery.initErr = err
			return delivery
		}
		delivery.s3Client = client
		delivery.s3Presign = s3.NewPresignClient(client)
	}
	return delivery
}

func (d *mediaDelivery) PlaybackURL(r *http.Request, episodeID string, rawMediaURL string) string {
	if d == nil {
		return newMediaPlaybackSigner(MediaPlaybackConfig{}).SignedPlaybackURL(r, episodeID)
	}
	switch d.mode {
	default:
		return d.signer.SignedPlaybackURL(r, episodeID)
	}
}

func (d *mediaDelivery) ServePlayback(w http.ResponseWriter, r *http.Request, episodeID string, assetPath string, rawMediaURL string) error {
	if d == nil {
		return errMediaDeliveryUnavailable
	}
	if d.initErr != nil {
		return d.initErr
	}
	switch d.mode {
	case MediaDeliverySignedRedirect:
		if assetPath != "master.m3u8" {
			return errMediaDeliveryUnavailable
		}
		w.Header().Set("Cache-Control", "private, max-age=60")
		http.Redirect(w, r, rawMediaURL, http.StatusFound)
		return nil
	case MediaDeliveryMinIOPresign:
		return d.serveMinIOPresignedPlayback(w, r, assetPath, rawMediaURL)
	case MediaDeliveryNginxAuthRequest:
		return d.serveNginxAuthPlayback(w, r, assetPath, rawMediaURL)
	default:
		return errMediaDeliveryUnavailable
	}
}

func (d *mediaDelivery) VerifyNginxAuthRequest(r *http.Request) bool {
	if d == nil {
		return false
	}
	cookie, err := r.Cookie(nginxMediaCookieName)
	if err != nil {
		return false
	}
	originalURI := strings.TrimSpace(r.Header.Get("X-Original-URI"))
	if originalURI == "" {
		originalURI = r.URL.RequestURI()
	}
	return d.signer.VerifyNginxCookieValue(cookie.Value, originalURI)
}

func (d *mediaDelivery) serveMinIOPresignedPlayback(w http.ResponseWriter, r *http.Request, assetPath string, rawMediaURL string) error {
	rootKey, err := objectKeyFromMediaURL(rawMediaURL, d.config.PublicBaseURL, d.config.StorageBucket)
	if err != nil {
		return err
	}
	rootDir := path.Dir(rootKey)
	objectKey := path.Join(rootDir, assetPath)
	if !strings.HasSuffix(strings.ToLower(assetPath), ".m3u8") {
		presignedURL, err := d.presignObjectURL(r.Context(), objectKey)
		if err != nil {
			return err
		}
		http.Redirect(w, r, presignedURL, http.StatusFound)
		return nil
	}

	content, err := d.getS3ObjectText(r.Context(), objectKey)
	if err != nil {
		return err
	}
	rewritten, err := d.rewriteHLSPlaylist(r, rootDir, assetPath, content)
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Header().Set("Cache-Control", "private, max-age=30")
	_, err = w.Write([]byte(rewritten))
	return err
}

func (d *mediaDelivery) serveNginxAuthPlayback(w http.ResponseWriter, r *http.Request, assetPath string, rawMediaURL string) error {
	if assetPath != "master.m3u8" {
		return errMediaDeliveryUnavailable
	}
	rootKey, err := objectKeyFromMediaURL(rawMediaURL, d.config.PublicBaseURL, d.config.StorageBucket)
	if err != nil {
		return err
	}
	rootDir := path.Dir(rootKey)
	cookieValue := d.signer.SignedNginxCookieValue(rootDir)
	http.SetCookie(w, &http.Cookie{
		Name:     nginxMediaCookieName,
		Value:    cookieValue,
		Path:     "/",
		MaxAge:   int(d.signer.ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	signedURL, err := d.signedNginxURL(rootKey)
	if err != nil {
		return err
	}
	http.Redirect(w, r, signedURL, http.StatusFound)
	return nil
}

func (d *mediaDelivery) rewriteHLSPlaylist(r *http.Request, rootDir string, currentAssetPath string, content string) (string, error) {
	currentDir := path.Dir(cleanMediaAssetPath(currentAssetPath))
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		assetPath := cleanMediaAssetPath(path.Join(currentDir, strings.Split(trimmed, "?")[0]))
		if assetPath == "" {
			return "", fmt.Errorf("invalid hls asset path %q", trimmed)
		}
		if strings.HasSuffix(strings.ToLower(assetPath), ".m3u8") {
			lines[i] = d.signer.SignedAssetPlaybackURL(r, episodeIDFromPlaybackRequest(r), assetPath)
			continue
		}
		presignedURL, err := d.presignObjectURL(r.Context(), path.Join(rootDir, assetPath))
		if err != nil {
			return "", err
		}
		lines[i] = presignedURL
	}
	return strings.Join(lines, "\n"), nil
}

func episodeIDFromPlaybackRequest(r *http.Request) string {
	episodeID, _, ok := episodeAssetFromPlaybackPath(r.URL.Path)
	if !ok {
		return ""
	}
	return episodeID
}

func (d *mediaDelivery) getS3ObjectText(ctx context.Context, objectKey string) (string, error) {
	output, err := d.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(strings.TrimSpace(d.config.StorageBucket)),
		Key:    aws.String(objectKey),
	})
	if err != nil {
		return "", fmt.Errorf("get hls playlist %q: %w", objectKey, err)
	}
	defer output.Body.Close()
	content, err := io.ReadAll(output.Body)
	if err != nil {
		return "", fmt.Errorf("read hls playlist %q: %w", objectKey, err)
	}
	return string(content), nil
}

func (d *mediaDelivery) presignObjectURL(ctx context.Context, objectKey string) (string, error) {
	output, err := d.s3Presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(strings.TrimSpace(d.config.StorageBucket)),
		Key:    aws.String(objectKey),
	}, func(options *s3.PresignOptions) {
		options.Expires = d.presignTTL
	})
	if err != nil {
		return "", fmt.Errorf("presign object %q: %w", objectKey, err)
	}
	return output.URL, nil
}

func (d *mediaDelivery) signedNginxURL(objectKey string) (string, error) {
	if d.publicBaseURL == "" {
		return "", errMediaDeliveryUnavailable
	}
	expires := d.signer.now().UTC().Add(d.signer.ttl).Unix()
	values := url.Values{}
	values.Set("expires", strconv.FormatInt(expires, 10))
	values.Set("sig", d.signer.nginxSignature(cleanMediaAssetPath(objectKey), expires))
	return joinURLPath(d.publicBaseURL, objectKey) + "?" + values.Encode(), nil
}

func newMediaDeliveryS3Client(ctx context.Context, config MediaPlaybackConfig) (*s3.Client, error) {
	if strings.TrimSpace(config.StorageEndpoint) == "" {
		return nil, errors.New("MEDIA_STORAGE_ENDPOINT is required for minio_presign delivery")
	}
	if strings.TrimSpace(config.StorageBucket) == "" {
		return nil, errors.New("MEDIA_STORAGE_BUCKET is required for minio_presign delivery")
	}
	if strings.TrimSpace(config.StorageRegion) == "" {
		return nil, errors.New("MEDIA_STORAGE_REGION is required for minio_presign delivery")
	}
	if strings.TrimSpace(config.StorageAccessKeyID) == "" {
		return nil, errors.New("MEDIA_STORAGE_ACCESS_KEY_ID is required for minio_presign delivery")
	}
	if strings.TrimSpace(config.StorageSecretAccessKey) == "" {
		return nil, errors.New("MEDIA_STORAGE_SECRET_ACCESS_KEY is required for minio_presign delivery")
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(strings.TrimSpace(config.StorageRegion)),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				strings.TrimSpace(config.StorageAccessKeyID),
				strings.TrimSpace(config.StorageSecretAccessKey),
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load media delivery s3 config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(strings.TrimSpace(config.StorageEndpoint))
		options.UsePathStyle = config.StorageForcePathStyle
	})
	return client, nil
}

// 从媒体 URL 推导对象 key
// case 1 link could be used directly: anime/episode-01/hls/master.m3u8
// case 2 ：https://cdn.example.com/anime/episode-01/hls/master.m3u8 --> anime/episode-01/hls/master.m3u8
// case 3 : /minio-bucket/anime/episode-01/hls/master.m3u8 --> anime/episode-01/hls/master.m3u8
func objectKeyFromMediaURL(rawURL string, publicBaseURL string, bucket string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", errors.New("media url is empty")
	}
	if !strings.Contains(rawURL, "://") {
		key := cleanMediaAssetPath(rawURL)
		if key == "" {
			return "", fmt.Errorf("invalid media object key %q", rawURL)
		}
		return key, nil
	}
	base := strings.TrimRight(strings.TrimSpace(publicBaseURL), "/")
	if base != "" && strings.HasPrefix(rawURL, base+"/") {
		key := cleanMediaAssetPath(strings.TrimPrefix(rawURL, base+"/"))
		if key != "" {
			return key, nil
		}
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}
	key := strings.TrimPrefix(parsed.Path, "/")
	bucket = strings.Trim(strings.TrimSpace(bucket), "/")
	if bucket != "" && (key == bucket || strings.HasPrefix(key, bucket+"/")) {
		key = strings.TrimPrefix(strings.TrimPrefix(key, bucket), "/")
	}
	key = cleanMediaAssetPath(key)
	if key == "" {
		return "", fmt.Errorf("cannot derive object key from media url %q", rawURL)
	}
	return key, nil
}

func joinURLPath(baseURL string, parts ...string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	cleanParts := make([]string, 0, len(parts))
	for _, part := range parts {
		if cleaned := cleanMediaAssetPath(part); cleaned != "" {
			cleanParts = append(cleanParts, cleaned)
		}
	}
	if len(cleanParts) == 0 {
		return baseURL
	}
	return baseURL + "/" + path.Join(cleanParts...)
}
