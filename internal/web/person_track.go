package web

import (
	"fmt"
	"image"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/person"
	"github.com/hwyc888/FaceSign/internal/store"
)

const (
	personTrackTTL              = 2 * time.Second
	minRecognitionFaceQuality   = 0.45
	bestFaceImprovement         = 0.04
	unknownFaceRetryInterval    = 850 * time.Millisecond
)

type personTrack struct {
	ID            string
	Rectangle     image.Rectangle
	Score         float64
	FirstSeen     time.Time
	LastSeen      time.Time
	BestQuality   float64
	LastFeatureAt time.Time
	Student       *store.Student
	Similarity    float64
}

type personTrackSession struct {
	Tracks map[string]*personTrack
	NextID uint64
}

type personTracker struct {
	mu       sync.Mutex
	sessions map[string]*personTrackSession
}

type personTrackObservation struct {
	TrackID    string
	Rectangle  image.Rectangle
	Score      float64
	BestQuality float64
	Student    *store.Student
	Similarity float64
}

type faceQualityResult struct {
	Score      float64
	Status     string
	Size       float64
	Pose       float64
	Sharpness  float64
	Exposure   float64
	Detector   float64
}

func newPersonTracker() *personTracker {
	return &personTracker{sessions: make(map[string]*personTrackSession)}
}

func (t *personTracker) Observe(sessionID string, detections []person.Detection, now time.Time) []personTrackObservation {
	t.mu.Lock()
	defer t.mu.Unlock()

	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		session = &personTrackSession{Tracks: make(map[string]*personTrack)}
		t.sessions[sessionID] = session
	}
	for id, track := range session.Tracks {
		if now.Sub(track.LastSeen) > personTrackTTL {
			delete(session.Tracks, id)
		}
	}

	type candidate struct {
		detection int
		trackID   string
		score     float64
	}
	candidates := make([]candidate, 0, len(detections)*len(session.Tracks))
	for i, detection := range detections {
		for id, track := range session.Tracks {
			score := personAssociationScore(track.Rectangle, detection.Rectangle)
			if score >= 0.20 {
				candidates = append(candidates, candidate{detection: i, trackID: id, score: score})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })

	matchedDetection := make(map[int]string)
	usedTracks := make(map[string]bool)
	for _, candidate := range candidates {
		if _, ok := matchedDetection[candidate.detection]; ok || usedTracks[candidate.trackID] {
			continue
		}
		matchedDetection[candidate.detection] = candidate.trackID
		usedTracks[candidate.trackID] = true
	}

	out := make([]personTrackObservation, len(detections))
	for i, detection := range detections {
		trackID := matchedDetection[i]
		track := session.Tracks[trackID]
		if track == nil {
			session.NextID++
			trackID = fmt.Sprintf("%s-P%d", sessionID, session.NextID)
			track = &personTrack{
				ID:        trackID,
				FirstSeen: now,
			}
			session.Tracks[trackID] = track
		}
		if !track.Rectangle.Empty() {
			track.Rectangle = blendRectangle(track.Rectangle, detection.Rectangle, 0.68)
		} else {
			track.Rectangle = detection.Rectangle
		}
		track.Score = detection.Score
		track.LastSeen = now

		var student *store.Student
		if track.Student != nil {
			copy := *track.Student
			student = &copy
		}
		out[i] = personTrackObservation{
			TrackID:     track.ID,
			Rectangle:   track.Rectangle,
			Score:       track.Score,
			BestQuality: track.BestQuality,
			Student:     student,
			Similarity:  track.Similarity,
		}
	}
	return out
}

func (t *personTracker) NeedFeature(sessionID, trackID string, quality float64, now time.Time) bool {
	if quality < minRecognitionFaceQuality || trackID == "" {
		return trackID == "" && quality >= minRecognitionFaceQuality
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	track := t.trackLocked(sessionID, trackID)
	if track == nil {
		return true
	}
	if track.LastFeatureAt.IsZero() {
		return true
	}
	if track.Student == nil {
		return quality >= track.BestQuality+bestFaceImprovement || now.Sub(track.LastFeatureAt) >= unknownFaceRetryInterval
	}
	return quality >= track.BestQuality+bestFaceImprovement
}

func (t *personTracker) RecordFeature(sessionID, trackID string, quality float64, student *store.Student, similarity float64, now time.Time) {
	if trackID == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	track := t.trackLocked(sessionID, trackID)
	if track == nil {
		return
	}
	previousBest := track.BestQuality
	track.LastFeatureAt = now
	if quality > track.BestQuality {
		track.BestQuality = quality
	}
	if student == nil {
		return
	}
	if track.Student != nil && track.Student.ID != student.ID {
		if quality < previousBest+0.08 || similarity < track.Similarity+0.03 {
			return
		}
	}
	copy := *student
	track.Student = &copy
	track.Similarity = similarity
}

func (t *personTracker) Identity(sessionID, trackID string) (*store.Student, float64, float64) {
	if trackID == "" {
		return nil, 0, 0
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	track := t.trackLocked(sessionID, trackID)
	if track == nil || track.Student == nil {
		if track == nil {
			return nil, 0, 0
		}
		return nil, 0, track.BestQuality
	}
	copy := *track.Student
	return &copy, track.Similarity, track.BestQuality
}

func (t *personTracker) trackLocked(sessionID, trackID string) *personTrack {
	if sessionID == "" {
		sessionID = "default"
	}
	session := t.sessions[sessionID]
	if session == nil {
		return nil
	}
	return session.Tracks[trackID]
}

func personAssociationScore(a, b image.Rectangle) float64 {
	if a.Empty() || b.Empty() {
		return 0
	}
	iou := rectangleIoU(a, b)
	acx, acy := rectangleCenter(a)
	bcx, bcy := rectangleCenter(b)
	distance := math.Hypot(acx-bcx, acy-bcy)
	scale := math.Max(float64(maxInt(a.Dx(), b.Dx())), float64(maxInt(a.Dy(), b.Dy())))
	if scale <= 0 {
		return iou
	}
	centerScore := 1 - math.Min(1, distance/(scale*1.25))
	return iou*0.70 + centerScore*0.30
}

func rectangleIoU(a, b image.Rectangle) float64 {
	intersection := a.Intersect(b)
	if intersection.Empty() {
		return 0
	}
	intersectionArea := float64(intersection.Dx() * intersection.Dy())
	unionArea := float64(a.Dx()*a.Dy()+b.Dx()*b.Dy()) - intersectionArea
	if unionArea <= 0 {
		return 0
	}
	return intersectionArea / unionArea
}

func rectangleCenter(rect image.Rectangle) (float64, float64) {
	return float64(rect.Min.X+rect.Max.X) / 2, float64(rect.Min.Y+rect.Max.Y) / 2
}

func blendRectangle(previous, current image.Rectangle, currentWeight float64) image.Rectangle {
	if previous.Empty() {
		return current
	}
	if currentWeight <= 0 || currentWeight >= 1 {
		return current
	}
	previousWeight := 1 - currentWeight
	return image.Rect(
		int(math.Round(float64(previous.Min.X)*previousWeight+float64(current.Min.X)*currentWeight)),
		int(math.Round(float64(previous.Min.Y)*previousWeight+float64(current.Min.Y)*currentWeight)),
		int(math.Round(float64(previous.Max.X)*previousWeight+float64(current.Max.X)*currentWeight)),
		int(math.Round(float64(previous.Max.Y)*previousWeight+float64(current.Max.Y)*currentWeight)),
	)
}

func associateFaceToPerson(rect image.Rectangle, tracks []personTrackObservation) (int, bool) {
	if rect.Empty() || len(tracks) == 0 {
		return -1, false
	}
	cx, cy := rectangleCenter(rect)
	bestIndex := -1
	bestScore := -1.0
	for i, track := range tracks {
		body := track.Rectangle
		if body.Empty() {
			continue
		}
		inside := cx >= float64(body.Min.X) && cx <= float64(body.Max.X) &&
			cy >= float64(body.Min.Y) && cy <= float64(body.Max.Y)
		if !inside {
			continue
		}
		bodyWidth := math.Max(1, float64(body.Dx()))
		bodyHeight := math.Max(1, float64(body.Dy()))
		normalizedX := math.Abs(cx-float64(body.Min.X+body.Max.X)/2) / bodyWidth
		normalizedY := math.Abs(cy-float64(body.Min.Y)-bodyHeight*0.18) / bodyHeight
		score := 1 - normalizedX - normalizedY*0.5
		if score > bestScore {
			bestScore = score
			bestIndex = i
		}
	}
	return bestIndex, bestIndex >= 0
}

func addFaceFallbackPersons(people []person.Detection, faces []face.Detection, bounds image.Rectangle) []person.Detection {
	out := append([]person.Detection(nil), people...)
	for _, detected := range faces {
		cx, cy := rectangleCenter(detected.Rectangle)
		covered := false
		for _, item := range out {
			if cx >= float64(item.Rectangle.Min.X) && cx <= float64(item.Rectangle.Max.X) &&
				cy >= float64(item.Rectangle.Min.Y) && cy <= float64(item.Rectangle.Max.Y) {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		faceRect := detected.Rectangle
		faceWidth := float64(faceRect.Dx())
		faceHeight := float64(faceRect.Dy())
		body := image.Rect(
			int(math.Round(cx-faceWidth*1.4)),
			int(math.Round(float64(faceRect.Min.Y)-faceHeight*0.35)),
			int(math.Round(cx+faceWidth*1.4)),
			int(math.Round(float64(faceRect.Min.Y)+faceHeight*5.8)),
		).Intersect(bounds)
		if !body.Empty() {
			out = append(out, person.Detection{Rectangle: body, Score: 0.20})
		}
	}
	return out
}

func scoreFaceQuality(img image.Image, detected face.Detection) faceQualityResult {
	rect := detected.Rectangle.Intersect(img.Bounds())
	if rect.Empty() {
		return faceQualityResult{Status: "等待清晰人脸"}
	}
	minSide := float64(minInt(rect.Dx(), rect.Dy()))
	sizeScore := clamp01((minSide - 45) / 95)
	detectorScore := clamp01((float64(detected.Score) - 0.45) / 0.50)
	poseScore := facePoseScore(detected.Landmarks)
	sharpnessScore, exposureScore := faceImageQuality(img, rect)
	score := sizeScore*0.32 + poseScore*0.25 + sharpnessScore*0.18 + detectorScore*0.15 + exposureScore*0.10
	score = clamp01(score)

	status := "等待清晰人脸"
	switch {
	case score >= 0.78:
		status = "优秀"
	case score >= 0.62:
		status = "清晰"
	case score >= minRecognitionFaceQuality:
		status = "可识别"
	}
	return faceQualityResult{
		Score:     score,
		Status:    status,
		Size:      sizeScore,
		Pose:      poseScore,
		Sharpness: sharpnessScore,
		Exposure:  exposureScore,
		Detector:  detectorScore,
	}
}

func facePoseScore(landmarks [5]image.Point) float64 {
	leftEye, rightEye := landmarks[0], landmarks[1]
	nose := landmarks[2]
	leftMouth, rightMouth := landmarks[3], landmarks[4]
	eyeDistance := math.Hypot(float64(leftEye.X-rightEye.X), float64(leftEye.Y-rightEye.Y))
	if eyeDistance < 4 {
		return 0.35
	}
	eyeLevel := 1 - math.Min(1, math.Abs(float64(leftEye.Y-rightEye.Y))/(eyeDistance*0.35))
	eyeMidX := float64(leftEye.X+rightEye.X) / 2
	noseCenter := 1 - math.Min(1, math.Abs(float64(nose.X)-eyeMidX)/(eyeDistance*0.55))
	mouthMidX := float64(leftMouth.X+rightMouth.X) / 2
	mouthCenter := 1 - math.Min(1, math.Abs(mouthMidX-eyeMidX)/(eyeDistance*0.65))
	return clamp01(eyeLevel*0.40 + noseCenter*0.40 + mouthCenter*0.20)
}

func faceImageQuality(img image.Image, rect image.Rectangle) (float64, float64) {
	step := maxInt(1, minInt(rect.Dx(), rect.Dy())/42)
	var brightnessSum float64
	var differenceSum float64
	var samples int
	var differences int
	for y := rect.Min.Y; y < rect.Max.Y; y += step {
		for x := rect.Min.X; x < rect.Max.X; x += step {
			current := luminanceAt(img, x, y)
			brightnessSum += current
			samples++
			if x+step < rect.Max.X {
				differenceSum += math.Abs(current - luminanceAt(img, x+step, y))
				differences++
			}
			if y+step < rect.Max.Y {
				differenceSum += math.Abs(current - luminanceAt(img, x, y+step))
				differences++
			}
		}
	}
	if samples == 0 {
		return 0, 0
	}
	brightness := brightnessSum / float64(samples)
	exposure := 1.0
	switch {
	case brightness < 35:
		exposure = brightness / 35
	case brightness < 60:
		exposure = 0.65 + (brightness-35)/25*0.35
	case brightness > 235:
		exposure = math.Max(0, (255-brightness)/20)
	case brightness > 215:
		exposure = 0.65 + (235-brightness)/20*0.35
	}
	sharpness := 0.0
	if differences > 0 {
		meanDifference := differenceSum / float64(differences)
		sharpness = clamp01((meanDifference - 2.5) / 18)
	}
	return sharpness, clamp01(exposure)
}

func luminanceAt(img image.Image, x, y int) float64 {
	r, g, b, _ := img.At(x, y).RGBA()
	r8 := float64(r) / 257
	g8 := float64(g) / 257
	b8 := float64(b) / 257
	return 0.299*r8 + 0.587*g8 + 0.114*b8
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
