package web

import (
	"errors"
	"image"
	"image/draw"
	"math"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/person"
)

const (
	personFaceProbeEstimatedPixels = 30.0
	personFaceReadyEstimatedPixels = 45.0
)

func estimatedFacePixels(rect image.Rectangle) float64 {
	if rect.Empty() {
		return 0
	}
	return math.Min(float64(rect.Dx())*0.38, float64(rect.Dy())*0.16)
}

func anyPersonReadyForFaceProbe(people []person.Detection) bool {
	for _, item := range people {
		if estimatedFacePixels(item.Rectangle) >= personFaceProbeEstimatedPixels {
			return true
		}
	}
	return false
}

func personWaitingStatus(rect image.Rectangle) string {
	if estimatedFacePixels(rect) < personFaceReadyEstimatedPixels {
		return "等待靠近"
	}
	return "等待露脸"
}

func scalePersonDetections(src []person.Detection, from, to image.Rectangle) []person.Detection {
	if len(src) == 0 {
		return nil
	}
	out := make([]person.Detection, 0, len(src))
	for _, item := range src {
		item.Rectangle = scaleRectangle(item.Rectangle, from, to)
		if !item.Rectangle.Empty() {
			out = append(out, item)
		}
	}
	return out
}

func scaleRectangle(rect, from, to image.Rectangle) image.Rectangle {
	if rect.Empty() || from.Dx() <= 0 || from.Dy() <= 0 || to.Dx() <= 0 || to.Dy() <= 0 {
		return image.Rectangle{}
	}
	sx := float64(to.Dx()) / float64(from.Dx())
	sy := float64(to.Dy()) / float64(from.Dy())
	return image.Rect(
		to.Min.X+int(math.Round(float64(rect.Min.X-from.Min.X)*sx)),
		to.Min.Y+int(math.Round(float64(rect.Min.Y-from.Min.Y)*sy)),
		to.Min.X+int(math.Round(float64(rect.Max.X-from.Min.X)*sx)),
		to.Min.Y+int(math.Round(float64(rect.Max.Y-from.Min.Y)*sy)),
	).Intersect(to)
}

func personHeadROI(personRect, bounds image.Rectangle) image.Rectangle {
	personRect = personRect.Intersect(bounds)
	if personRect.Empty() {
		return image.Rectangle{}
	}
	w := float64(personRect.Dx())
	h := float64(personRect.Dy())
	cx := float64(personRect.Min.X+personRect.Max.X) / 2
	roiW := math.Max(48, w*0.92)
	roiH := math.Max(56, math.Min(h*0.46, w*1.45))
	x1 := int(math.Round(cx - roiW/2))
	x2 := int(math.Round(cx + roiW/2))
	y1 := int(math.Round(float64(personRect.Min.Y) - h*0.04))
	y2 := int(math.Round(float64(y1) + roiH))
	return image.Rect(x1, y1, x2, y2).Intersect(bounds)
}

func cropImage(img image.Image, rect image.Rectangle) image.Image {
	rect = rect.Intersect(img.Bounds())
	if rect.Empty() {
		return nil
	}
	if sub, ok := img.(interface{ SubImage(image.Rectangle) image.Image }); ok {
		return sub.SubImage(rect)
	}
	out := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(out, out.Bounds(), img, rect.Min, draw.Src)
	return out
}

func translateFaceDetection(detected face.Detection, offset image.Point) face.Detection {
	detected.Rectangle = detected.Rectangle.Add(offset)
	for i := range detected.Landmarks {
		detected.Landmarks[i] = detected.Landmarks[i].Add(offset)
	}
	return detected
}

func (s *Server) detectFacesInPersonROIs(img image.Image, people []person.Detection) ([]face.Detection, error) {
	out := make([]face.Detection, 0, len(people))
	for _, item := range people {
		roi := personHeadROI(item.Rectangle, img.Bounds())
		if roi.Dx() < 40 || roi.Dy() < 40 {
			continue
		}
		cropped := cropImage(img, roi)
		if cropped == nil {
			continue
		}
		detected, err := s.engine.DetectAll(cropped, s.detectionThreshold)
		if err != nil {
			if errors.Is(err, face.ErrNoFace) {
				continue
			}
			return nil, err
		}
		if len(detected) == 0 {
			continue
		}
		best := detected[0]
		for _, candidate := range detected[1:] {
			if candidate.Score > best.Score {
				best = candidate
			}
		}
		out = append(out, translateFaceDetection(best, roi.Min))
	}
	return out, nil
}
