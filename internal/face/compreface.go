package face

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

type CompreFace struct {
	name               string
	baseURL            string
	apiKey             string
	requireAPIKey      bool
	similarity         float64
	detectionThreshold float64
	client             *http.Client
}

func NewCompreFace(baseURL, apiKey string, similarity, detectionThreshold float64) *CompreFace {
	return &CompreFace{
		name:               "compreface",
		baseURL:            baseURL,
		apiKey:             apiKey,
		requireAPIKey:      true,
		similarity:         similarity,
		detectionThreshold: detectionThreshold,
		client:             &http.Client{Timeout: 15 * time.Second},
	}
}

func NewLocalCPU(baseURL string, similarity, detectionThreshold float64) *CompreFace {
	return &CompreFace{
		name:               "localcpu",
		baseURL:            baseURL,
		requireAPIKey:      false,
		similarity:         similarity,
		detectionThreshold: detectionThreshold,
		client:             &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *CompreFace) Name() string { return c.name }
func (c *CompreFace) Enabled() bool {
	return !c.requireAPIKey || c.apiKey != ""
}

func (c *CompreFace) Check(ctx context.Context) error {
	if c.requireAPIKey && c.apiKey == "" {
		return ErrInvalidAPIKey
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v1/recognition/subjects/", nil)
	if err != nil {
		return err
	}
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrInvalidAPIKey
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("%w: HTTP %s %s", ErrUnavailable, resp.Status, bytes.TrimSpace(message))
	}
	return nil
}

func (c *CompreFace) Enroll(ctx context.Context, subject string, image []byte) (string, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/recognition/faces/")
	if err != nil {
		return "", err
	}
	query := endpoint.Query()
	query.Set("subject", subject)
	query.Set("det_prob_threshold", strconv.FormatFloat(c.detectionThreshold, 'f', 2, 64))
	endpoint.RawQuery = query.Encode()

	var response struct {
		ImageID string `json:"image_id"`
		Subject string `json:"subject"`
	}
	if err := c.postImage(ctx, endpoint.String(), image, &response); err != nil {
		return "", err
	}
	if response.ImageID == "" {
		return "", fmt.Errorf("人脸识别服务返回了空的 image_id")
	}
	return response.ImageID, nil
}

func (c *CompreFace) Recognize(ctx context.Context, image []byte) (Match, error) {
	endpoint, err := url.Parse(c.baseURL + "/api/v1/recognition/recognize")
	if err != nil {
		return Match{}, err
	}
	query := endpoint.Query()
	query.Set("prediction_count", "1")
	query.Set("det_prob_threshold", strconv.FormatFloat(c.detectionThreshold, 'f', 2, 64))
	endpoint.RawQuery = query.Encode()

	var response struct {
		Result []struct {
			Subjects []struct {
				Similarity float64 `json:"similarity"`
				Subject    string  `json:"subject"`
			} `json:"subjects"`
		} `json:"result"`
	}
	if err := c.postImage(ctx, endpoint.String(), image, &response); err != nil {
		return Match{}, err
	}
	if len(response.Result) > 1 {
		return Match{}, ErrMultipleFaces
	}

	best := Match{}
	for _, result := range response.Result {
		for _, subject := range result.Subjects {
			if subject.Similarity > best.Similarity {
				best = Match{Subject: subject.Subject, Similarity: subject.Similarity}
			}
		}
	}
	if best.Subject == "" || best.Similarity < c.similarity {
		return Match{}, ErrNoMatch
	}
	return best, nil
}

func (c *CompreFace) postImage(ctx context.Context, endpoint string, image []byte, target any) error {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "capture.jpg")
	if err != nil {
		return err
	}
	if _, err := part.Write(image); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if c.apiKey != "" {
		req.Header.Set("x-api-key", c.apiKey)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrInvalidAPIKey
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		var problem struct {
			Detail string `json:"detail"`
			Error  string `json:"error"`
		}
		if json.Unmarshal(message, &problem) == nil {
			if problem.Detail != "" {
				message = []byte(problem.Detail)
			} else if problem.Error != "" {
				message = []byte(problem.Error)
			}
		}
		if resp.StatusCode >= 500 {
			return fmt.Errorf("%w: HTTP %s %s", ErrUnavailable, resp.Status, bytes.TrimSpace(message))
		}
		return fmt.Errorf("人脸识别服务返回 HTTP %s: %s", resp.Status, bytes.TrimSpace(message))
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(target); err != nil {
		return fmt.Errorf("decode compreface response: %w", err)
	}
	return nil
}
