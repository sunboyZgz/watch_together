package mediactl

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/schollz/progressbar/v3"
)

type UploadProgressStage string

const (
	UploadStageStarted   UploadProgressStage = "started"
	UploadStageFileStart UploadProgressStage = "file_start"
	UploadStageProgress  UploadProgressStage = "progress"
	UploadStageFileDone  UploadProgressStage = "file_done"
	UploadStageCompleted UploadProgressStage = "completed"
)

type UploadProgressEvent struct {
	Stage            UploadProgressStage `json:"stage"`
	Driver           string              `json:"driver"`
	SourceKey        string              `json:"sourceKey"`
	TargetRoot       string              `json:"targetRoot"`
	CurrentFilePath  string              `json:"currentFilePath,omitempty"`
	CurrentFileName  string              `json:"currentFileName,omitempty"`
	CurrentObjectKey string              `json:"currentObjectKey,omitempty"`
	CompletedFiles   int                 `json:"completedFiles"`
	TotalFiles       int                 `json:"totalFiles"`
	TransferredBytes int64               `json:"transferredBytes"`
	TotalBytes       int64               `json:"totalBytes"`
	CurrentFileBytes int64               `json:"currentFileBytes"`
	CurrentFileSize  int64               `json:"currentFileSize"`
}

type UploadProgressSink interface {
	OnUploadProgress(event UploadProgressEvent)
}

type uploadAsset struct {
	LocalPath   string
	ObjectKey   string
	DisplayName string
	SizeBytes   int64
}

type uploadProgressTracker struct {
	sink           UploadProgressSink
	driver         string
	sourceKey      string
	targetRoot     string
	totalFiles     int
	totalBytes     int64
	completedFiles int
	transferred    int64
	currentFile    uploadAsset
	currentSent    int64
	mu             sync.Mutex
}

func newUploadProgressTracker(
	sink UploadProgressSink,
	driver string,
	sourceKey string,
	targetRoot string,
	assets []uploadAsset,
) *uploadProgressTracker {
	if sink == nil {
		return nil
	}
	var totalBytes int64
	for _, asset := range assets {
		totalBytes += asset.SizeBytes
	}
	return &uploadProgressTracker{
		sink:       sink,
		driver:     driver,
		sourceKey:  sourceKey,
		targetRoot: targetRoot,
		totalFiles: len(assets),
		totalBytes: totalBytes,
	}
}

func (t *uploadProgressTracker) emitStarted() {
	if t == nil {
		return
	}
	t.sink.OnUploadProgress(t.snapshot(UploadStageStarted))
}

func (t *uploadProgressTracker) startFile(asset uploadAsset) {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.currentFile = asset
	t.currentSent = 0
	event := t.snapshotLocked(UploadStageFileStart)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) addBytes(delta int64) {
	if t == nil || delta <= 0 {
		return
	}
	t.mu.Lock()
	t.currentSent += delta
	t.transferred += delta
	event := t.snapshotLocked(UploadStageProgress)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) finishFile() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.completedFiles++
	event := t.snapshotLocked(UploadStageFileDone)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) emitCompleted() {
	if t == nil {
		return
	}
	t.mu.Lock()
	event := t.snapshotLocked(UploadStageCompleted)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) snapshot(stage UploadProgressStage) UploadProgressEvent {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.snapshotLocked(stage)
}

func (t *uploadProgressTracker) snapshotLocked(stage UploadProgressStage) UploadProgressEvent {
	return UploadProgressEvent{
		Stage:            stage,
		Driver:           t.driver,
		SourceKey:        t.sourceKey,
		TargetRoot:       t.targetRoot,
		CurrentFilePath:  t.currentFile.LocalPath,
		CurrentFileName:  t.currentFile.DisplayName,
		CurrentObjectKey: t.currentFile.ObjectKey,
		CompletedFiles:   t.completedFiles,
		TotalFiles:       t.totalFiles,
		TransferredBytes: t.transferred,
		TotalBytes:       t.totalBytes,
		CurrentFileBytes: t.currentSent,
		CurrentFileSize:  t.currentFile.SizeBytes,
	}
}

type progressReader struct {
	reader  io.Reader
	onBytes func(int64)
}

func (r progressReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.onBytes != nil {
		r.onBytes(int64(n))
	}
	return n, err
}

type cliUploadProgressRenderer struct {
	writer io.Writer
	bar    *progressbar.ProgressBar
}

func newCLIUploadProgressRenderer(writer io.Writer) UploadProgressSink {
	if writer == nil {
		return nil
	}
	return &cliUploadProgressRenderer{writer: writer}
}

func (r *cliUploadProgressRenderer) OnUploadProgress(event UploadProgressEvent) {
	switch event.Stage {
	case UploadStageStarted:
		totalBytes := event.TotalBytes
		if totalBytes <= 0 {
			totalBytes = 1
		}
		r.bar = progressbar.NewOptions64(
			totalBytes,
			progressbar.OptionSetWriter(r.writer),
			progressbar.OptionSetWidth(24),
			progressbar.OptionShowBytes(true),
			progressbar.OptionShowTotalBytes(true),
			progressbar.OptionUseIECUnits(true),
			progressbar.OptionThrottle(120000000),
			progressbar.OptionSetDescription(
				fmt.Sprintf("upload %s (%d files)", event.Driver, event.TotalFiles),
			),
			progressbar.OptionShowDescriptionAtLineEnd(),
			progressbar.OptionSetTheme(progressbar.Theme{
				Saucer:        "=",
				SaucerHead:    ">",
				SaucerPadding: "-",
				BarStart:      "[",
				BarEnd:        "]",
			}),
		)
		if r.bar != nil {
			_ = r.bar.RenderBlank()
		}
	case UploadStageFileStart, UploadStageProgress, UploadStageFileDone:
		if r.bar == nil {
			return
		}
		r.bar.Describe(fmt.Sprintf(
			"upload %d/%d %s",
			event.CompletedFiles,
			event.TotalFiles,
			event.CurrentFileName,
		))
		maxBytes := event.TotalBytes
		if maxBytes <= 0 {
			maxBytes = 1
		}
		r.bar.ChangeMax64(maxBytes)
		_ = r.bar.Set64(clampProgressValue(event.TransferredBytes, maxBytes))
	case UploadStageCompleted:
		if r.bar == nil {
			return
		}
		maxBytes := event.TotalBytes
		if maxBytes <= 0 {
			maxBytes = 1
		}
		r.bar.Describe(fmt.Sprintf("upload %d/%d completed", event.CompletedFiles, event.TotalFiles))
		r.bar.ChangeMax64(maxBytes)
		_ = r.bar.Set64(maxBytes)
		_ = r.bar.Finish()
	}
}

func clampProgressValue(value int64, max int64) int64 {
	if value < 0 {
		return 0
	}
	if value > max {
		return max
	}
	return value
}

func uploadTargetRoot(options IngestOptions) string {
	switch normalizedStorageDriver(options.Storage.Driver) {
	case "minio", "s3":
		return strings.TrimSpace(options.Storage.Bucket)
	default:
		return filepath.Join(options.Storage.LocalRoot, sourceObjectKey(options), "hls")
	}
}
