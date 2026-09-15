package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultVideoBaseURL   = "https://graph-video.facebook.com"
	maxVideoTransferSteps = 10000
	maxVideoChunkRetries  = 3
)

type videoUploadOffsets struct {
	StartOffset     string `json:"start_offset"`
	EndOffset       string `json:"end_offset"`
	UploadSessionID string `json:"upload_session_id"`
	VideoID         string `json:"video_id"`
}

// UploadVideoResumable implements Meta's resumable ad-video upload protocol:
// start -> one or more transfer chunks -> finish. The returned video id is
// populated as soon as Meta accepts the start phase, even when a later phase
// fails, so callers can surface a recoverable partial mutation instead of
// silently starting a second upload.
func (c *Client) UploadVideoResumable(ctx context.Context, path, filename string, size int64, open func() (io.ReadCloser, error)) (string, error) {
	if c == nil {
		return "", fmt.Errorf("meta: client is nil")
	}
	if size <= 0 {
		return "", fmt.Errorf("meta: video upload size must be positive")
	}
	if open == nil {
		return "", fmt.Errorf("meta: video upload source is required")
	}
	filename = strings.TrimSpace(filename)
	if filename == "" {
		filename = "upload.mp4"
	}

	var started videoUploadOffsets
	if err := c.postVideoForm(ctx, path, url.Values{
		"file_size":    {strconv.FormatInt(size, 10)},
		"upload_phase": {"start"},
	}, &started); err != nil {
		return "", fmt.Errorf("start resumable upload: %w", err)
	}
	videoID := strings.TrimSpace(started.VideoID)
	if videoID == "" {
		return "", fmt.Errorf("start resumable upload: Meta returned no video_id")
	}
	sessionID := strings.TrimSpace(started.UploadSessionID)
	if sessionID == "" {
		return videoID, fmt.Errorf("start resumable upload: Meta returned no upload_session_id")
	}
	start, end, err := parseVideoOffsets(started.StartOffset, started.EndOffset, size)
	if err != nil {
		return videoID, fmt.Errorf("start resumable upload: %w", err)
	}

	for step := 0; start != end; step++ {
		if step >= maxVideoTransferSteps {
			return videoID, fmt.Errorf("transfer resumable upload: exceeded %d chunks", maxVideoTransferSteps)
		}
		previousStart, previousEnd := start, end
		var transferred videoUploadOffsets
		var transferErr error
		for attempt := 0; attempt < maxVideoChunkRetries; attempt++ {
			rc, openErr := open()
			if openErr != nil {
				return videoID, fmt.Errorf("transfer resumable upload: open source: %w", openErr)
			}
			if start > 0 {
				if _, copyErr := io.CopyN(io.Discard, rc, start); copyErr != nil {
					_ = rc.Close()
					return videoID, fmt.Errorf("transfer resumable upload: seek to offset %d: %w", start, copyErr)
				}
			}
			chunkSize := end - start
			transferred = videoUploadOffsets{}
			transferErr = c.postVideoMultipartStream(ctx, path, url.Values{
				"upload_phase":     {"transfer"},
				"start_offset":    {strconv.FormatInt(start, 10)},
				"upload_session_id": {sessionID},
			}, "video_file_chunk", filename, io.LimitReader(rc, chunkSize), chunkSize, &transferred)
			_ = rc.Close()
			if transferErr == nil {
				break
			}
			var apiErr *Error
			if !errorAs(transferErr, &apiErr) || !apiErr.IsTransient() || attempt+1 >= maxVideoChunkRetries {
				return videoID, fmt.Errorf("transfer resumable upload at offset %d: %w", start, transferErr)
			}
		}
		if transferErr != nil {
			return videoID, fmt.Errorf("transfer resumable upload at offset %d: %w", start, transferErr)
		}
		start, end, err = parseVideoOffsets(transferred.StartOffset, transferred.EndOffset, size)
		if err != nil {
			return videoID, fmt.Errorf("transfer resumable upload: %w", err)
		}
		if start == previousStart && end == previousEnd {
			return videoID, fmt.Errorf("transfer resumable upload: Meta returned stalled offsets %d-%d", start, end)
		}
	}

	var finished struct {
		Success bool `json:"success"`
	}
	if err := c.postVideoForm(ctx, path, url.Values{
		"upload_phase":      {"finish"},
		"upload_session_id": {sessionID},
		"title":             {filename},
	}, &finished); err != nil {
		return videoID, fmt.Errorf("finish resumable upload: %w", err)
	}
	if !finished.Success {
		return videoID, fmt.Errorf("finish resumable upload: Meta did not report success")
	}
	return videoID, nil
}

func parseVideoOffsets(rawStart, rawEnd string, size int64) (int64, int64, error) {
	start, err := strconv.ParseInt(strings.TrimSpace(rawStart), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start_offset %q", rawStart)
	}
	end, err := strconv.ParseInt(strings.TrimSpace(rawEnd), 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end_offset %q", rawEnd)
	}
	if start < 0 || end < start || end > size {
		return 0, 0, fmt.Errorf("invalid upload offsets %d-%d for %d-byte file", start, end, size)
	}
	return start, end, nil
}

func (c *Client) postVideoForm(ctx context.Context, path string, form url.Values, out any) error {
	return c.doVideo(ctx, path, strings.NewReader(form.Encode()), "application/x-www-form-urlencoded", -1, out)
}

func (c *Client) postVideoMultipartStream(ctx context.Context, path string, fields url.Values, fileField, filename string, r io.Reader, size int64, out any) error {
	if r == nil {
		return fmt.Errorf("meta: upload body is required")
	}
	if size < 0 {
		return fmt.Errorf("meta: upload size is invalid")
	}
	var prefix bytes.Buffer
	writer := multipart.NewWriter(&prefix)
	for key, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(key, value); err != nil {
				return err
			}
		}
	}
	if _, err := writer.CreateFormFile(fileField, filename); err != nil {
		return err
	}
	suffix := "\r\n--" + writer.Boundary() + "--\r\n"
	body := io.MultiReader(&prefix, io.LimitReader(r, size), strings.NewReader(suffix))
	contentLength := int64(prefix.Len()) + size + int64(len(suffix))
	return c.doVideo(ctx, path, body, writer.FormDataContentType(), contentLength, out)
}

func (c *Client) doVideo(ctx context.Context, path string, body io.Reader, contentType string, contentLength int64, out any) error {
	if c == nil {
		return fmt.Errorf("meta: client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := c.cfg.UploadTimeout
	if timeout <= 0 {
		timeout = c.cfg.Timeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cleanPath, err := normalizePath(path)
	if err != nil {
		return err
	}
	baseURL := c.baseURL
	if baseURL == strings.TrimRight(DefaultBaseURL, "/") {
		baseURL = defaultVideoBaseURL
	}
	endpoint := baseURL + "/" + Version + "/" + cleanPath
	operation := Redact("POST "+cleanPath+" (video upload)", c.cfg.AccessToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, body)
	if err != nil {
		return transportError(operation, err, c.cfg.AccessToken)
	}
	if contentLength >= 0 {
		req.ContentLength = contentLength
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.AccessToken)
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", userAgent)
	resp, err := c.http.Do(req)
	if err != nil {
		return transportError(operation, err, c.cfg.AccessToken)
	}
	defer resp.Body.Close()
	return c.decodeJSONResponse(resp, operation, out)
}

// errorAs is kept local so resumable upload can inspect transient Meta errors
// without exporting another helper from this package.
func errorAs(err error, target any) bool {
	return errorsAs(err, target)
}
