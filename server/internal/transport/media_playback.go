package transport

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	defaultMediaPlaybackSigningSecret = "watch_together_local_dev_media_playback_secret_change_me"
	defaultMediaPlaybackURLTTL        = 2 * time.Hour
	//DeliveryMode:
	MediaDeliverySignedRedirect   = "signed_redirect"
	MediaDeliveryMinIOPresign     = "minio_presign"
	MediaDeliveryNginxAuthRequest = "nginx_auth_request"
)

/*
core info:
签名密钥 URL 有效期 媒体分发模式 外部访问地址 内部访问地址 MinIO / S3 存储配置 时间函数
*/
type MediaPlaybackConfig struct {
	DeliveryMode           string
	SigningSecret          string
	URLTTL                 time.Duration
	PublicBaseURL          string
	InternalBaseURL        string
	StorageEndpoint        string
	StorageBucket          string
	StorageRegion          string
	StorageAccessKeyID     string
	StorageSecretAccessKey string
	StorageForcePathStyle  bool
	Now                    func() time.Time
}

type mediaPlaybackSigner struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

func newMediaPlaybackSigner(config MediaPlaybackConfig) *mediaPlaybackSigner {
	secret := strings.TrimSpace(config.SigningSecret)
	if secret == "" {
		secret = defaultMediaPlaybackSigningSecret
	}
	ttl := config.URLTTL
	if ttl <= 0 {
		ttl = defaultMediaPlaybackURLTTL
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &mediaPlaybackSigner{
		secret: []byte(secret),
		ttl:    ttl,
		now:    now,
	}
}

// 生成 master.m3u8 播放地址
func (s *mediaPlaybackSigner) SignedPlaybackURL(r *http.Request, episodeID string) string {
	return s.SignedAssetPlaybackURL(r, episodeID, "master.m3u8")
}

func (s *mediaPlaybackSigner) SignedAssetPlaybackURL(r *http.Request, episodeID string, assetPath string) string {
	if s == nil {
		s = newMediaPlaybackSigner(MediaPlaybackConfig{})
	}
	episodeID = strings.TrimSpace(episodeID)
	assetPath = cleanMediaAssetPath(assetPath)
	expires := s.now().UTC().Add(s.ttl).Unix()
	base := requestBaseURL(r)
	playbackPath := path.Join("/media/playback", url.PathEscape(episodeID), assetPath)
	query := url.Values{}
	query.Set("expires", strconv.FormatInt(expires, 10))
	query.Set("sig", s.playbackSignature(episodeID, assetPath, expires))
	return base + playbackPath + "?" + query.Encode()
}

func (s *mediaPlaybackSigner) VerifyPlayback(episodeID string, assetPath string, expiresRaw string, signature string) bool {
	if s == nil {
		s = newMediaPlaybackSigner(MediaPlaybackConfig{})
	}
	episodeID = strings.TrimSpace(episodeID)
	assetPath = cleanMediaAssetPath(assetPath)
	if episodeID == "" || assetPath == "" || strings.TrimSpace(signature) == "" {
		return false
	}
	expires, err := strconv.ParseInt(strings.TrimSpace(expiresRaw), 10, 64)
	if err != nil {
		return false
	}
	if s.now().UTC().Unix() > expires {
		return false
	}
	expected := s.playbackSignature(episodeID, assetPath, expires)
	return subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) == 1
}

// 生成 Nginx 鉴权 Cookie
func (s *mediaPlaybackSigner) SignedNginxCookieValue(rootKey string) string {
	if s == nil {
		s = newMediaPlaybackSigner(MediaPlaybackConfig{})
	}
	rootKey = cleanMediaAssetPath(rootKey)
	expires := s.now().UTC().Add(s.ttl).Unix()
	query := url.Values{}
	query.Set("root", rootKey)
	query.Set("expires", strconv.FormatInt(expires, 10))
	query.Set("sig", s.nginxSignature(rootKey, expires))
	return query.Encode()
}

func (s *mediaPlaybackSigner) VerifyNginxCookieValue(rawCookie string, originalURI string) bool {
	if s == nil {
		s = newMediaPlaybackSigner(MediaPlaybackConfig{})
	}
	values, err := url.ParseQuery(rawCookie)
	if err != nil {
		return false
	}
	rootKey := cleanMediaAssetPath(values.Get("root"))
	expiresRaw := strings.TrimSpace(values.Get("expires"))
	signature := strings.TrimSpace(values.Get("sig"))
	if rootKey == "" || expiresRaw == "" || signature == "" {
		return false
	}
	expires, err := strconv.ParseInt(expiresRaw, 10, 64)
	if err != nil || s.now().UTC().Unix() > expires {
		return false
	}
	expected := s.nginxSignature(rootKey, expires)
	if subtle.ConstantTimeCompare([]byte(expected), []byte(signature)) != 1 {
		return false
	}
	cleanURIPath := cleanMediaAssetPath(strings.TrimPrefix(strings.Split(originalURI, "?")[0], "/"))
	return cleanURIPath == rootKey || strings.HasPrefix(cleanURIPath, strings.TrimSuffix(rootKey, "/")+"/")
}

func (s *mediaPlaybackSigner) playbackSignature(episodeID string, assetPath string, expires int64) string {
	return s.signature("playback", episodeID, assetPath, strconv.FormatInt(expires, 10))
}

func (s *mediaPlaybackSigner) nginxSignature(rootKey string, expires int64) string {
	return s.signature("nginx", rootKey, strconv.FormatInt(expires, 10))
}

// 真正的 HMAC-SHA256 签名
func (s *mediaPlaybackSigner) signature(parts ...string) string {
	payload := "v1\n" + strings.Join(parts, "\n")
	mac := hmac.New(sha256.New, s.secret)
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func requestBaseURL(r *http.Request) string {
	if r == nil {
		return ""
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}
	host := strings.TrimSpace(r.Header.Get("X-Forwarded-Host"))
	if host == "" {
		host = r.Host
	}
	if host == "" {
		return ""
	}
	return proto + "://" + host
}

// 从播放 URL 路径解析 episodeID 和 assetPath
func episodeAssetFromPlaybackPath(rawPath string) (string, string, bool) {
	const prefix = "/media/playback/"
	if !strings.HasPrefix(rawPath, prefix) {
		return "", "", false
	}
	parts := strings.SplitN(strings.TrimPrefix(rawPath, prefix), "/", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	episodeID, err := url.PathUnescape(strings.TrimSpace(parts[0]))
	if err != nil {
		return "", "", false
	}
	episodeID = strings.TrimSpace(episodeID)
	assetPath := cleanMediaAssetPath(parts[1])
	return episodeID, assetPath, episodeID != "" && assetPath != ""
}

// 清理资源路径，防止路径穿越
func cleanMediaAssetPath(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	value = strings.TrimPrefix(value, "/")
	if value == "" {
		return ""
	}
	cleaned := path.Clean(value)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") || strings.Contains(cleaned, "/../") {
		return ""
	}
	return cleaned
}

func normalizeMediaDeliveryMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	switch normalized {
	case "":
		return MediaDeliverySignedRedirect
	case MediaDeliverySignedRedirect:
		return MediaDeliverySignedRedirect
	case MediaDeliveryMinIOPresign:
		return MediaDeliveryMinIOPresign
	case MediaDeliveryNginxAuthRequest:
		return MediaDeliveryNginxAuthRequest
	default:
		return normalized
	}
}
