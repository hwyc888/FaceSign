package face

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("人脸识别服务不可用")
var ErrInvalidAPIKey = errors.New("人脸识别服务 API Key 无效")
var ErrNoMatch = errors.New("未识别到已登记的人脸")
var ErrMultipleFaces = errors.New("画面中检测到多张人脸")

type Match struct {
	Subject    string
	Similarity float64
}

type Provider interface {
	Name() string
	Enabled() bool
	Check(ctx context.Context) error
	Enroll(ctx context.Context, subject string, image []byte) (string, error)
	Recognize(ctx context.Context, image []byte) (Match, error)
}

type Disabled struct{}

func (Disabled) Name() string                                           { return "disabled" }
func (Disabled) Enabled() bool                                          { return false }
func (Disabled) Check(context.Context) error                            { return ErrUnavailable }
func (Disabled) Enroll(context.Context, string, []byte) (string, error) { return "", ErrUnavailable }
func (Disabled) Recognize(context.Context, []byte) (Match, error)       { return Match{}, ErrUnavailable }
