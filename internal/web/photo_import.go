package web

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	stddraw "image/draw"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
	_ "golang.org/x/image/bmp"
	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"
)

const (
	maxPhotoImportArchive = int64(256 << 20)
	maxPhotoImportImage   = int64(25 << 20)
	maxPhotoImportPixels  = int64(40_000_000)
	maxPhotoImportItems         = 500
	photoImportTTL              = 30 * time.Minute
)

type photoImportSession struct {
	mu        sync.Mutex
	ID        string
	CreatedAt time.Time
	Items     []*photoImportItem
}

type photoImportItem struct {
	ID            int
	FileName      string
	Name          string
	StudentNo     string
	ClassName     string
	Format        string
	Quality       string
	QualityNote   string
	Status        string
	Reason        string
	Feature       []float32
	Thumbnail     []byte
	Existing      *store.Student
	Similarity    float64
	DuplicateFile string
	Committed     bool
}

type photoImportPublicItem struct {
	ID                int            `json:"id"`
	FileName          string         `json:"file_name"`
	Name              string         `json:"name"`
	StudentNo         string         `json:"student_no"`
	ClassName         string         `json:"class_name"`
	Format            string         `json:"format"`
	Quality           string         `json:"quality"`
	QualityNote       string         `json:"quality_note"`
	Status            string         `json:"status"`
	Reason            string         `json:"reason"`
	Existing          *store.Student `json:"existing,omitempty"`
	Similarity        float64        `json:"similarity"`
	DuplicateFile     string         `json:"duplicate_file,omitempty"`
	RecommendedAction string         `json:"recommended_action"`
	ThumbnailURL      string         `json:"thumbnail_url,omitempty"`
	Committed         bool           `json:"committed"`
}

type photoKnownFace struct {
	Student store.Student
	Feature []float32
}

func (s *Server) photoImportAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	s.cleanupPhotoImportSessions(time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoImportArchive+(4<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("读取压缩包失败: %w", err))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("请选择 ZIP 照片压缩包"))
		return
	}
	defer file.Close()

	size, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("无法读取压缩包大小"))
		return
	}
	if size <= 0 || size > maxPhotoImportArchive {
		writeError(w, http.StatusBadRequest, errors.New("ZIP 压缩包大小必须小于 256MB"))
		return
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("无法读取压缩包"))
		return
	}

	zr, err := zip.NewReader(file, size)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("文件不是有效的 ZIP 压缩包"))
		return
	}

	classes, err := s.store.ListClasses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	classNames := make(map[string]bool, len(classes))
	for _, class := range classes {
		classNames[class.Name] = true
	}

	known, err := s.loadPhotoKnownFaces(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	sessionID, err := newPhotoImportSessionID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	session := &photoImportSession{ID: sessionID, CreatedAt: time.Now()}
	ignored := 0
	processed := 0
	packageFeatures := make([]*photoImportItem, 0)

	for _, archived := range zr.File {
		if archived.FileInfo().IsDir() || strings.HasPrefix(archived.Name, "__MACOSX/") {
			continue
		}
		if processed >= maxPhotoImportItems {
			writeError(w, http.StatusBadRequest, fmt.Errorf("一个压缩包最多导入 %d 张图片", maxPhotoImportItems))
			return
		}

		displayName := path.Base(archived.Name)
		if displayName == "." || displayName == "" {
			continue
		}
		if archived.UncompressedSize64 > uint64(maxPhotoImportImage) {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "单张图片不能超过25MB"))
				processed++
			} else {
				ignored++
			}
			continue
		}

		rc, err := archived.Open()
		if err != nil {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "无法读取图片"))
				processed++
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxPhotoImportImage+1))
		rc.Close()
		if readErr != nil || int64(len(data)) > maxPhotoImportImage {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "图片读取失败或文件过大"))
				processed++
			}
			continue
		}

		config, format, err := image.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "图片损坏或格式暂不支持"))
				processed++
			} else {
				ignored++
			}
			continue
		}
		if !supportedPhotoFormat(format) {
			session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "该图片格式暂不支持"))
			processed++
			continue
		}
		if config.Width < 80 || config.Height < 80 ||
			config.Width > 10000 || config.Height > 10000 ||
			int64(config.Width)*int64(config.Height) > maxPhotoImportPixels {
			session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "图片尺寸不符合要求"))
			processed++
			continue
		}

		img, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "图片解码失败"))
			processed++
			continue
		}
		detected, err := s.engine.ExtractPhotoSample(img, s.detectionThreshold)
		if err != nil {
			item := rejectedPhotoItem(len(session.Items)+1, displayName, photoImportFaceError(err))
			item.Format = strings.ToUpper(format)
			session.Items = append(session.Items, item)
			processed++
			continue
		}

		quality, note, ok := assessImportedPhoto(img, detected)
		item := &photoImportItem{
			ID:          len(session.Items) + 1,
			FileName:    displayName,
			Format:      strings.ToUpper(format),
			Quality:     quality,
			QualityNote: note,
			Feature:     append([]float32(nil), detected.Feature...),
		}
		item.StudentNo, item.Name = photoImportIdentity(displayName)
		item.ClassName = photoImportClassHint(archived.Name, classNames)
		item.Thumbnail = makeFaceThumbnail(img, detected.Rectangle)
		if !ok {
			item.Status = "rejected"
			item.Reason = note
			session.Items = append(session.Items, item)
			processed++
			continue
		}

		bestStudent, bestScore := bestPhotoKnownMatch(item.Feature, known)
		item.Similarity = bestScore
		reviewThreshold := math.Max(0.55, s.matchThreshold-0.04)
		if bestStudent.ID != 0 && bestScore >= s.matchThreshold {
			copyStudent := bestStudent
			item.Existing = &copyStudent
			item.Status = "existing"
			item.ClassName = bestStudent.ClassName
			item.Name = bestStudent.Name
			item.StudentNo = bestStudent.StudentNo
			item.Reason = "已匹配现有学生，默认不重复建立；需要时可作为新的正面样本补充"
		} else if bestStudent.ID != 0 && bestScore >= reviewThreshold {
			copyStudent := bestStudent
			item.Existing = &copyStudent
			item.Status = "review"
			item.Reason = fmt.Sprintf("与“%s”相似度接近阈值，批量导入暂不自动入库，请人工核对", bestStudent.Name)
		} else {
			item.Status = "ready"
			for _, previous := range packageFeatures {
				score := face.Similarity(item.Feature, previous.Feature)
				if score >= s.matchThreshold {
					item.Status = "package_duplicate"
					item.Similarity = score
					item.DuplicateFile = previous.FileName
					item.Reason = fmt.Sprintf("与压缩包中的“%s”高度相似，疑似同一个人", previous.FileName)
					break
				}
			}
			if item.Status == "ready" {
				packageFeatures = append(packageFeatures, item)
			}
		}

		session.Items = append(session.Items, item)
		processed++
	}

	if len(session.Items) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("压缩包中没有找到可处理的图片"))
		return
	}

	s.photoImportMu.Lock()
	if s.photoImports == nil {
		s.photoImports = make(map[string]*photoImportSession)
	}
	s.photoImports[session.ID] = session
	s.photoImportMu.Unlock()

	counts := photoImportCounts(session.Items)
	public := make([]photoImportPublicItem, 0, len(session.Items))
	for _, item := range session.Items {
		public = append(public, s.publicPhotoImportItem(session, item))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"session_id":       session.ID,
		"archive_name":     header.Filename,
		"items":            public,
		"ignored_files":    ignored,
		"counts":           counts,
		"expires_in_minutes": int(photoImportTTL.Minutes()),
		"supported_formats": []string{"JPEG/JPG", "PNG", "WEBP", "GIF", "BMP", "TIFF"},
	})
}

func (s *Server) photoImportAction(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/photo-import/"), "/")
	parts := strings.Split(rest, "/")
	if len(parts) < 2 || parts[0] == "" {
		writeError(w, http.StatusNotFound, errors.New("导入会话不存在"))
		return
	}
	session, ok := s.getPhotoImportSession(parts[0])
	if !ok {
		writeError(w, http.StatusNotFound, errors.New("照片导入会话已过期，请重新上传 ZIP"))
		return
	}

	if len(parts) == 3 && parts[1] == "thumbnail" && r.Method == http.MethodGet {
		id, err := strconv.Atoi(parts[2])
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("无效图片编号"))
			return
		}
		s.photoImportThumbnail(w, session, id)
		return
	}
	if len(parts) == 2 && parts[1] == "commit" && r.Method == http.MethodPost {
		s.photoImportCommit(w, r, session)
		return
	}
	methodNotAllowed(w)
}

func (s *Server) photoImportThumbnail(w http.ResponseWriter, session *photoImportSession, id int) {
	session.mu.Lock()
	defer session.mu.Unlock()
	item := findPhotoImportItem(session.Items, id)
	if item == nil || len(item.Thumbnail) == 0 {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=1800")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(item.Thumbnail)
}

type photoImportCommitRequest struct {
	Items []struct {
		ID        int    `json:"id"`
		Action    string `json:"action"`
		StudentNo string `json:"student_no"`
		Name      string `json:"name"`
		ClassName string `json:"class_name"`
	} `json:"items"`
}

func (s *Server) photoImportCommit(w http.ResponseWriter, r *http.Request, session *photoImportSession) {
	var request photoImportCommitRequest
	if err := decodeJSON(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if len(request.Items) == 0 {
		writeError(w, http.StatusBadRequest, errors.New("没有选择要导入的照片"))
		return
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	results := make([]map[string]any, 0, len(request.Items))
	imported := 0
	skipped := 0
	failed := 0

	for _, requested := range request.Items {
		item := findPhotoImportItem(session.Items, requested.ID)
		if item == nil {
			failed++
			results = append(results, map[string]any{"id": requested.ID, "status": "failed", "message": "照片记录不存在"})
			continue
		}
		if requested.Action == "skip" || requested.Action == "" {
			skipped++
			results = append(results, map[string]any{"id": item.ID, "status": "skipped", "message": "已跳过"})
			continue
		}
		if item.Committed {
			skipped++
			results = append(results, map[string]any{"id": item.ID, "status": "skipped", "message": "该照片已经入库"})
			continue
		}
		if item.Status == "rejected" || item.Status == "review" || item.Status == "package_duplicate" {
			failed++
			results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": item.Reason})
			continue
		}

		className := strings.TrimSpace(requested.ClassName)
		exists, err := s.store.ClassExists(r.Context(), className)
		if err != nil {
			failed++
			results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
			continue
		}
		if !exists {
			failed++
			results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "请选择设置中已经建立的班级"})
			continue
		}

		switch requested.Action {
		case "new":
			if item.Status != "ready" {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "该照片不能建立新学生"})
				continue
			}
			studentNo := strings.TrimSpace(requested.StudentNo)
			name := strings.TrimSpace(requested.Name)
			if studentNo == "" || name == "" {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "新学生必须填写学号和姓名"})
				continue
			}
			comparison, err := s.compareFeature(r.Context(), item.Feature, 0)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if comparison.Other.ID != 0 && comparison.OtherScore >= s.matchThreshold {
				failed++
				results = append(results, map[string]any{
					"id": item.ID, "status": "failed",
					"message": fmt.Sprintf("入库前复核发现该人脸已属于 %s，已阻止重复建立", comparison.Other.Name),
				})
				continue
			}
			student, _, err := s.store.CreateStudentWithFace(
				r.Context(), studentNo, name, className, "照片导入-正面", face.Encode(item.Feature),
			)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": friendlyStudentError(err).Error()})
				continue
			}
			item.Committed = true
			imported++
			results = append(results, map[string]any{"id": item.ID, "status": "imported", "message": "已建立学生并录入正面样本", "student": student})

		case "supplement":
			if item.Existing == nil || item.Existing.ID <= 0 {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "没有可补充的已匹配学生"})
				continue
			}
			targetID := item.Existing.ID
			comparison, err := s.compareFeature(r.Context(), item.Feature, targetID)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if comparison.Other.ID != 0 &&
				comparison.OtherScore >= s.matchThreshold &&
				(!comparison.SameFound || comparison.OtherScore > comparison.SameScore) {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "照片更接近其他学生，已阻止错误补充"})
				continue
			}
			if !comparison.SameFound || comparison.SameScore < s.matchThreshold {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "照片与已匹配学生复核不一致"})
				continue
			}
			if comparison.SameScore >= 0.995 {
				skipped++
				item.Committed = true
				results = append(results, map[string]any{"id": item.ID, "status": "skipped", "message": "与已有样本几乎完全相同，无需重复保存"})
				continue
			}
			if err := s.store.UpdateStudentClass(r.Context(), targetID, className); err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if _, err := s.store.AddFaceSample(r.Context(), targetID, "照片导入-正面", face.Encode(item.Feature)); err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			item.Committed = true
			imported++
			results = append(results, map[string]any{"id": item.ID, "status": "imported", "message": "已补充现有学生正面样本"})
		default:
			failed++
			results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "未知导入操作"})
		}
	}

	if imported > 0 {
		s.refreshFaceCacheAfterMutation(r.Context())
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"imported": imported,
		"skipped":  skipped,
		"failed":   failed,
		"results":  results,
	})
}

func (s *Server) loadPhotoKnownFaces(ctx context.Context) ([]photoKnownFace, error) {
	samples, err := s.cachedFaceSamples(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]photoKnownFace, 0, len(samples))
	for _, sample := range samples {
		out = append(out, photoKnownFace{Student: sample.Student, Feature: sample.Feature})
	}
	return out, nil
}

func bestPhotoKnownMatch(feature []float32, known []photoKnownFace) (store.Student, float64) {
	var best store.Student
	bestScore := 0.0
	for _, candidate := range known {
		score := face.Similarity(feature, candidate.Feature)
		if score > bestScore {
			bestScore = score
			best = candidate.Student
		}
	}
	return best, bestScore
}

func assessImportedPhoto(img image.Image, detected face.Detection) (string, string, bool) {
	bounds := img.Bounds()
	rect := detected.Rectangle.Intersect(bounds)
	if rect.Empty() || bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "不合格", "未获得有效人脸区域", false
	}
	minSide := math.Min(float64(rect.Dx()), float64(rect.Dy()))
	faceRatio := float64(rect.Dx()*rect.Dy()) / float64(bounds.Dx()*bounds.Dy())
	if minSide < 96 || faceRatio < 0.025 {
		return "不合格", "人脸在照片中太小，建议使用更清晰、距离更近的正面照片", false
	}

	rightEye, leftEye := detected.Landmarks[0], detected.Landmarks[1]
	nose := detected.Landmarks[2]
	rightMouth, leftMouth := detected.Landmarks[3], detected.Landmarks[4]
	eyeDistance := math.Hypot(float64(leftEye.X-rightEye.X), float64(leftEye.Y-rightEye.Y))
	if eyeDistance < 24 {
		return "不合格", "眼部关键点分辨率过低，照片不适合作为高质量人脸样本", false
	}
	eyeMidX := float64(leftEye.X+rightEye.X) / 2
	mouthMidX := float64(leftMouth.X+rightMouth.X) / 2
	yawOffset := math.Abs(float64(nose.X)-eyeMidX) / eyeDistance
	mouthOffset := math.Abs(mouthMidX-eyeMidX) / eyeDistance
	roll := math.Abs(float64(leftEye.Y-rightEye.Y)) / eyeDistance
	if yawOffset > 0.35 || mouthOffset > 0.40 {
		return "不合格", "人脸侧转角度较大，不建议作为正面样本", false
	}
	if roll > 0.30 {
		return "不合格", "头部倾斜较大，请使用较端正的正面照片", false
	}

	if detected.Score >= 0.92 && minSide >= 160 && faceRatio >= 0.06 && yawOffset <= 0.18 && roll <= 0.15 {
		return "优", "正面度、清晰度和人脸占比良好", true
	}
	return "合格", "照片可用于正面样本；后续仍建议补充摄像头多角度样本", true
}

func photoImportIdentity(fileName string) (studentNo, name string) {
	base := strings.TrimSpace(strings.TrimSuffix(path.Base(fileName), path.Ext(fileName)))
	if base == "" {
		return "", ""
	}
	parts := strings.FieldsFunc(base, func(r rune) bool {
		return r == '_' || r == '-' || r == ' ' || r == '　'
	})
	if len(parts) >= 2 && isASCIIStudentNo(parts[0]) {
		return parts[0], strings.TrimSpace(strings.Join(parts[1:], " "))
	}
	return "", base
}

func isASCIIStudentNo(value string) bool {
	if value == "" {
		return false
	}
	hasDigit := false
	for _, r := range value {
		switch {
		case r >= '0' && r <= '9':
			hasDigit = true
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		default:
			return false
		}
	}
	return hasDigit
}

func photoImportClassHint(archiveName string, classes map[string]bool) string {
	dir := path.Dir(strings.ReplaceAll(archiveName, "\\", "/"))
	if dir == "." || dir == "/" {
		return ""
	}
	for _, part := range strings.Split(dir, "/") {
		part = strings.TrimSpace(part)
		if classes[part] {
			return part
		}
	}
	return ""
}

func supportedPhotoFormat(format string) bool {
	switch strings.ToLower(format) {
	case "jpeg", "png", "webp", "gif", "bmp", "tiff":
		return true
	default:
		return false
	}
}

func looksLikePhotoName(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".jpg", ".jpeg", ".jfif", ".png", ".webp", ".gif", ".bmp", ".tif", ".tiff":
		return true
	default:
		return false
	}
}

func photoImportFaceError(err error) string {
	switch {
	case errors.Is(err, face.ErrNoFace):
		return "未检测到清晰人脸"
	case errors.Is(err, face.ErrMultipleFaces):
		return "照片中有多张明显人脸，只能使用单人照片"
	default:
		return "人脸特征提取失败"
	}
}

func makeFaceThumbnail(img image.Image, faceRect image.Rectangle) []byte {
	bounds := img.Bounds()
	if faceRect.Empty() {
		return nil
	}
	padX := faceRect.Dx() / 2
	padY := faceRect.Dy() / 2
	crop := image.Rect(faceRect.Min.X-padX, faceRect.Min.Y-padY, faceRect.Max.X+padX, faceRect.Max.Y+padY).Intersect(bounds)
	if crop.Empty() {
		return nil
	}
	const size = 160
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), img, crop, stddraw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 78}); err != nil {
		return nil
	}
	return buf.Bytes()
}

func rejectedPhotoItem(id int, fileName, reason string) *photoImportItem {
	studentNo, name := photoImportIdentity(fileName)
	return &photoImportItem{
		ID:        id,
		FileName:  fileName,
		StudentNo: studentNo,
		Name:      name,
		Quality:   "不合格",
		Status:    "rejected",
		Reason:    reason,
	}
}

func newPhotoImportSessionID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

func (s *Server) cleanupPhotoImportSessions(now time.Time) {
	s.photoImportMu.Lock()
	defer s.photoImportMu.Unlock()
	for id, session := range s.photoImports {
		if now.Sub(session.CreatedAt) > photoImportTTL {
			delete(s.photoImports, id)
		}
	}
}

func (s *Server) getPhotoImportSession(id string) (*photoImportSession, bool) {
	s.cleanupPhotoImportSessions(time.Now())
	s.photoImportMu.Lock()
	defer s.photoImportMu.Unlock()
	session, ok := s.photoImports[id]
	return session, ok
}

func findPhotoImportItem(items []*photoImportItem, id int) *photoImportItem {
	for _, item := range items {
		if item.ID == id {
			return item
		}
	}
	return nil
}

func (s *Server) publicPhotoImportItem(session *photoImportSession, item *photoImportItem) photoImportPublicItem {
	action := "skip"
	switch item.Status {
	case "ready":
		action = "new"
	case "existing":
		action = "skip"
	}
	thumbnailURL := ""
	if len(item.Thumbnail) > 0 {
		thumbnailURL = fmt.Sprintf("/api/photo-import/%s/thumbnail/%d", session.ID, item.ID)
	}
	return photoImportPublicItem{
		ID:                item.ID,
		FileName:          item.FileName,
		Name:              item.Name,
		StudentNo:         item.StudentNo,
		ClassName:         item.ClassName,
		Format:            item.Format,
		Quality:           item.Quality,
		QualityNote:       item.QualityNote,
		Status:            item.Status,
		Reason:            item.Reason,
		Existing:          item.Existing,
		Similarity:        item.Similarity,
		DuplicateFile:     item.DuplicateFile,
		RecommendedAction: action,
		ThumbnailURL:      thumbnailURL,
		Committed:         item.Committed,
	}
}

func photoImportCounts(items []*photoImportItem) map[string]int {
	counts := map[string]int{"ready": 0, "existing": 0, "review": 0, "rejected": 0, "package_duplicate": 0}
	for _, item := range items {
		counts[item.Status]++
	}
	return counts
}
