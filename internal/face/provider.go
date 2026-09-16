package face

import (
	"context"
	"errors"
)

var ErrUnavailable = errors.New("face recognition service is unavailable")
var ErrNoMatch = errors.New("no matching face found")
var ErrMultipleFaces = errors.New("multiple faces detected")

type Match struct {
	Subject    string
	Similarity float64
}

type Provider interface {
	Name() string
	Enabled() bool
	Enroll(ctx context.Context, subject string, image []byte) (string, error)
	Recognize(ctx context.Context, image []byte) (Match, error)
}

type Disabled struct{}

func (Disabled) Name() string                                           { return "disabled" }
func (Disabled) Enabled() bool                                          { return false }
func (Disabled) Enroll(context.Context, string, []byte) (string, error) { return "", ErrUnavailable }
func (Disabled) Recognize(context.Context, []byte) (Match, error)       { return Match{}, ErrUnavailable }
