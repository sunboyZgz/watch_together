package mediactl

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/schollz/progressbar/v3"
	"golang.org/x/term"
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
	completedBytes int64
	completedFiles int
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
	event := t.snapshotLocked(UploadStageProgress)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) setCurrentFileProgress(current int64) {
	if t == nil {
		return
	}
	if current < 0 {
		current = 0
	}
	t.mu.Lock()
	if current > t.currentFile.SizeBytes && t.currentFile.SizeBytes > 0 {
		current = t.currentFile.SizeBytes
	}
	t.currentSent = current
	event := t.snapshotLocked(UploadStageProgress)
	t.mu.Unlock()
	t.sink.OnUploadProgress(event)
}

func (t *uploadProgressTracker) finishFile() {
	if t == nil {
		return
	}
	t.mu.Lock()
	t.currentSent = t.currentFile.SizeBytes
	t.completedBytes += t.currentFile.SizeBytes
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
		TransferredBytes: t.completedBytes + t.currentSent,
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

type progressReadSeeker struct {
	reader io.ReadSeeker
	onRead func(int64)
	onSeek func(int64)
}

func (r progressReadSeeker) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	if n > 0 && r.onRead != nil {
		currentOffset, seekErr := r.reader.Seek(0, io.SeekCurrent)
		if seekErr == nil {
			r.onRead(currentOffset)
		}
	}
	return n, err
}

func (r progressReadSeeker) Seek(offset int64, whence int) (int64, error) {
	next, err := r.reader.Seek(offset, whence)
	if err == nil && r.onSeek != nil {
		r.onSeek(next)
	}
	return next, err
}

type cliUploadProgressRenderer struct {
	writer             io.Writer
	bar                *progressbar.ProgressBar
	dynamic            bool
	lastPrintedPercent int64
}

func newCLIUploadProgressRenderer(writer io.Writer) UploadProgressSink {
	if writer == nil {
		return nil
	}
	return &cliUploadProgressRenderer{
		writer:             writer,
		dynamic:            isInteractiveTerminal(writer),
		lastPrintedPercent: -1,
	}
}

func (r *cliUploadProgressRenderer) OnUploadProgress(event UploadProgressEvent) {
	switch event.Stage {
	case UploadStageStarted:
		if !r.dynamic {
			fmt.Fprintf(
				r.writer,
				"upload started driver=%s files=%d totalBytes=%d target=%s\n",
				event.Driver,
				event.TotalFiles,
				event.TotalBytes,
				event.TargetRoot,
			)
			return
		}
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
			progressbar.OptionUseANSICodes(true),
			progressbar.OptionThrottle(120000000),
			progressbar.OptionClearOnFinish(),
			progressbar.OptionSetDescription(
				fmt.Sprintf("upload %d files", event.TotalFiles),
			),
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
		if !r.dynamic {
			r.renderFallbackProgress(event)
			return
		}
		if r.bar == nil {
			return
		}
		r.bar.Describe(fmt.Sprintf(
			"upload %d/%d",
			event.CompletedFiles,
			event.TotalFiles,
		))
		maxBytes := event.TotalBytes
		if maxBytes <= 0 {
			maxBytes = 1
		}
		r.bar.ChangeMax64(maxBytes)
		_ = r.bar.Set64(clampProgressValue(event.TransferredBytes, maxBytes))
	case UploadStageCompleted:
		if !r.dynamic {
			fmt.Fprintf(
				r.writer,
				"upload completed files=%d/%d bytes=%d/%d\n",
				event.CompletedFiles,
				event.TotalFiles,
				event.TransferredBytes,
				event.TotalBytes,
			)
			return
		}
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

func isInteractiveTerminal(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func (r *cliUploadProgressRenderer) renderFallbackProgress(event UploadProgressEvent) {
	percent := int64(0)
	if event.TotalBytes > 0 {
		percent = (event.TransferredBytes * 100) / event.TotalBytes
	}
	if percent > 100 {
		percent = 100
	}
	shouldPrint := event.Stage == UploadStageFileDone || event.Stage == UploadStageCompleted
	if !shouldPrint {
		if percent >= r.lastPrintedPercent+5 || r.lastPrintedPercent < 0 {
			shouldPrint = true
		}
	}
	if !shouldPrint {
		return
	}
	r.lastPrintedPercent = percent
	fmt.Fprintf(
		r.writer,
		"upload progress %d%% files=%d/%d current=%s bytes=%d/%d\n",
		percent,
		event.CompletedFiles,
		event.TotalFiles,
		event.CurrentFileName,
		event.TransferredBytes,
		event.TotalBytes,
	)
}

func uploadTargetRoot(options IngestOptions) string {
	switch normalizedStorageDriver(options.Storage.Driver) {
	case "minio", "s3":
		return strings.TrimSpace(options.Storage.Bucket)
	default:
		return filepath.Join(options.Storage.LocalRoot, sourceObjectKey(options), "hls")
	}
}
