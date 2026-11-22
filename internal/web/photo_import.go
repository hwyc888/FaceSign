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
	mu          sync.Mutex
	ID          string
	OwnerUserID int64
	CreatedAt   time.Time
	Items       []*photoImportItem
}

type photoImportItem struct {
	ID                 int
	FileName           string
	Name               string
	StudentNo          string
	ClassName          string
	Angle              string
	Format             string
	Quality            string
	QualityNote        string
	Status             string
	Reason             string
	Feature            []float32
	Thumbnail          []byte
	Existing           *store.Student
	MatchedByStudentNo bool
	Similarity         float64
	DuplicateFile      string
	Committed          bool
}

type photoImportPublicItem struct {
	ID                 int            `json:"id"`
	FileName           string         `json:"file_name"`
	Name               string         `json:"name"`
	StudentNo          string         `json:"student_no"`
	ClassName          string         `json:"class_name"`
	Angle              string         `json:"angle"`
	MatchedByStudentNo bool           `json:"matched_by_student_no,omitempty"`
	Format             string         `json:"format"`
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

type photoImportSourceFile struct {
	Name string
	Size uint64
	Open func() (io.ReadCloser, error)
}

func photoImportSourcesFromMultipart(r *http.Request) ([]photoImportSourceFile, string, func(), error) {
	if r.MultipartForm == nil {
		return nil, "", func() {}, errors.New("请选择图片文件或 ZIP 照片包")
	}
	archives := r.MultipartForm.File["file"]
	uploads := r.MultipartForm.File["files"]
	if len(archives) > 0 && len(uploads) > 0 {
		return nil, "", func() {}, errors.New("请单独上传一个 ZIP 包，或直接多选图片，不要混合上传")
	}
	if len(archives) > 1 {
		return nil, "", func() {}, errors.New("一次只能上传一个 ZIP 照片包")
	}
	if len(archives) == 1 {
		header := archives[0]
		if header.Size <= 0 || header.Size > maxPhotoImportArchive {
			return nil, "", func() {}, errors.New("ZIP 压缩包大小必须大于 0 且小于 256MB")
		}
		file, err := header.Open()
		if err != nil {
			return nil, "", func() {}, fmt.Errorf("无法打开 ZIP 压缩包: %w", err)
		}
		size, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			file.Close()
			return nil, "", func() {}, errors.New("无法读取压缩包大小")
		}
		if size <= 0 || size > maxPhotoImportArchive {
			file.Close()
			return nil, "", func() {}, errors.New("ZIP 压缩包大小必须小于 256MB")
		}
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			file.Close()
			return nil, "", func() {}, errors.New("无法读取压缩包")
		}
		zr, err := zip.NewReader(file, size)
		if err != nil {
			file.Close()
			return nil, "", func() {}, errors.New("文件不是有效的 ZIP 压缩包")
		}
		sources := make([]photoImportSourceFile, 0, len(zr.File))
		for _, archived := range zr.File {
			if archived.FileInfo().IsDir() {
				continue
			}
			entry := archived
			sources = append(sources, photoImportSourceFile{
				Name: entry.Name,
				Size: entry.UncompressedSize64,
				Open: func() (io.ReadCloser, error) {
					return entry.Open()
				},
			})
		}
		return sources, header.Filename, func() { _ = file.Close() }, nil
	}
	if len(uploads) == 0 {
		return nil, "", func() {}, errors.New("请选择 JPG、PNG 等图片，或上传 ZIP 照片包")
	}
	if len(uploads) > maxPhotoImportItems {
		return nil, "", func() {}, fmt.Errorf("一次最多选择 %d 张图片", maxPhotoImportItems)
	}
	var totalSize int64
	sources := make([]photoImportSourceFile, 0, len(uploads))
	for _, uploaded := range uploads {
		header := uploaded
		if header.Size < 0 || header.Size > maxPhotoImportImage {
			return nil, "", func() {}, fmt.Errorf("图片“%s”超过单张 25MB 限制", path.Base(header.Filename))
		}
		totalSize += header.Size
		if totalSize > maxPhotoImportArchive {
			return nil, "", func() {}, errors.New("多张图片总大小不能超过 256MB")
		}
		sources = append(sources, photoImportSourceFile{
			Name: path.Base(header.Filename),
			Size: uint64(header.Size),
			Open: func() (io.ReadCloser, error) {
				return header.Open()
			},
		})
	}
	return sources, fmt.Sprintf("直接上传 %d 张图片", len(sources)), func() {}, nil
}

func (s *Server) photoImportAnalyze(w http.ResponseWriter, r *http.Request) {
	user := authUser(r)
	if !requirePermission(w, r, user.CanAddStudents || user.CanResampleFaces, "没有新增人员或重新采样的权限") {
		return
	}
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}

	s.cleanupPhotoImportSessions(time.Now())
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoImportArchive+(4<<20))
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("读取上传文件失败: %w", err))
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	sources, uploadName, closeSources, err := photoImportSourcesFromMultipart(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defer closeSources()

	classes, err := s.store.ListClasses(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	classNames := make(map[string]bool, len(classes))
	for _, class := range classes {
		classNames[class.Name] = true
	}
	defaultClassName, err := photoImportDefaultClassName(r, classNames)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	defaultAngle, err := photoImportDefaultAngleName(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	students, err := s.store.ListStudents(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	studentsByNo := make(map[string]store.Student, len(students))
	for _, student := range students {
		studentsByNo[photoImportStudentKey(student.StudentNo)] = student
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
	session := &photoImportSession{ID: sessionID, OwnerUserID: authUser(r).ID, CreatedAt: time.Now()}
	ignored := 0
	processed := 0
	packageFeatures := make([]*photoImportItem, 0)

	for _, source := range sources {
		if strings.HasPrefix(strings.ReplaceAll(source.Name, "\\", "/"), "__MACOSX/") {
			continue
		}
		if processed >= maxPhotoImportItems {
			writeError(w, http.StatusBadRequest, fmt.Errorf("一次最多分析 %d 张图片", maxPhotoImportItems))
			return
		}

		displayName := path.Base(source.Name)
		if displayName == "." || displayName == "" {
			continue
		}
		if source.Size > uint64(maxPhotoImportImage) {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "单张图片不能超过25MB"))
				processed++
			} else {
				ignored++
			}
			continue
		}

		rc, err := source.Open()
		if err != nil {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "无法读取图片"))
				processed++
			} else {
				ignored++
			}
			continue
		}
		data, readErr := io.ReadAll(io.LimitReader(rc, maxPhotoImportImage+1))
		rc.Close()
		if readErr != nil || int64(len(data)) > maxPhotoImportImage {
			if looksLikePhotoName(displayName) {
				session.Items = append(session.Items, rejectedPhotoItem(len(session.Items)+1, displayName, "图片读取失败或文件过大"))
				processed++
			} else {
				ignored++
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
		detected, err := s.engine.ExtractPhotoSample(img, s.currentRecognitionThresholds().DetectionThreshold)
		if err != nil {
			item := rejectedPhotoItem(len(session.Items)+1, displayName, photoImportFaceError(err))
			item.Format = strings.ToUpper(format)
			session.Items = append(session.Items, item)
			processed++
			continue
		}

		angle := photoImportAngleForSource(source.Name, defaultAngle)
		quality, note, ok := assessImportedPhotoForAngle(img, detected, angle)
		item := &photoImportItem{
			ID:          len(session.Items) + 1,
			FileName:    displayName,
			Angle:       angle,
			Format:      strings.ToUpper(format),
			Quality:     quality,
			QualityNote: note,
			Feature:     append([]float32(nil), detected.Feature...),
		}
		item.StudentNo, item.Name = photoImportIdentity(displayName)
		item.ClassName = photoImportClassForSource(source.Name, defaultClassName, classNames)
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
		reviewThreshold := math.Max(0.55, s.currentRecognitionThresholds().MatchThreshold-0.04)
		if identityStudent, ok := studentsByNo[photoImportStudentKey(item.StudentNo)]; item.StudentNo != "" && ok {
			item.Existing = &identityStudent
			item.MatchedByStudentNo = true
			item.Status = "existing"
			item.ClassName = identityStudent.ClassName
			item.Name = identityStudent.Name
			item.StudentNo = identityStudent.StudentNo
			item.Similarity = bestPhotoStudentScore(item.Feature, identityStudent.ID, known)
			item.Reason = "按文件名学号匹配到现有学生，请核对照片后选择补充所选角度样本"
		} else if bestStudent.ID != 0 && bestScore >= s.currentRecognitionThresholds().MatchThreshold {
			copyStudent := bestStudent
			item.Existing = &copyStudent
			item.Status = "existing"
			item.ClassName = bestStudent.ClassName
			item.Name = bestStudent.Name
			item.StudentNo = bestStudent.StudentNo
			item.Reason = "已匹配现有学生，默认不重复建立；需要时可补充所选角度样本"
		} else if bestStudent.ID != 0 && bestScore >= reviewThreshold {
			copyStudent := bestStudent
			item.Existing = &copyStudent
			item.Status = "review"
			item.Reason = fmt.Sprintf("与“%s”相似度接近阈值，批量导入暂不自动入库，请人工核对", bestStudent.Name)
		} else {
			item.Status = "ready"
			for _, previous := range packageFeatures {
				if photoImportSameStudentDifferentAngle(item, previous) {
					continue
				}
				score := face.Similarity(item.Feature, previous.Feature)
				if score >= s.currentRecognitionThresholds().MatchThreshold {
					item.Status = "package_duplicate"
					item.Similarity = score
					item.DuplicateFile = previous.FileName
					item.Reason = fmt.Sprintf("与批次中的“%s”高度相似，疑似同一个人", previous.FileName)
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
		writeError(w, http.StatusBadRequest, errors.New("没有找到可处理的图片；请上传 JPG、PNG 等图片或 ZIP 照片包"))
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
		"session_id":         session.ID,
		"archive_name":       uploadName,
		"items":              public,
		"ignored_files":      ignored,
		"counts":             counts,
		"expires_in_minutes": int(photoImportTTL.Minutes()),
		"supported_formats":  []string{"JPEG/JPG", "PNG", "WEBP", "GIF", "BMP", "TIFF"},
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
	if session.OwnerUserID != authUser(r).ID && !authUser(r).IsAdmin {
		writeError(w, http.StatusNotFound, errors.New("照片导入会话不存在"))
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
		Angle     string `json:"angle"`
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
	batchStudents := make(map[string]store.Student)
	batchAngles := make(map[string]map[string]bool)

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

		angle := strings.TrimSpace(requested.Angle)
		if angle == "" {
			angle = item.Angle
		}
		if !validPhotoImportAngle(angle) {
			failed++
			results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "请选择有效的人脸采集角度"})
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
			if !authUser(r).IsAdmin && !authUser(r).CanAddStudents {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "没有新增学生的权限"})
				continue
			}
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

			batchKey := photoImportStudentKey(studentNo)
			if existingBatchStudent, ok := batchStudents[batchKey]; batchKey != "" && ok {
				if existingBatchStudent.Name != name || existingBatchStudent.ClassName != className {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "同一批次中该学号的姓名或班级不一致，请先统一资料"})
					continue
				}
				if batchAngles[batchKey][angle] {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "同一批次中该学生已经有该角度样本，请检查是否重复选择"})
					continue
				}
				comparison, err := s.compareFeature(r.Context(), item.Feature, existingBatchStudent.ID)
				if err != nil {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
					continue
				}
				if comparison.Other.ID != 0 &&
					comparison.OtherScore >= s.currentRecognitionThresholds().MatchThreshold &&
					(!comparison.SameFound || comparison.OtherScore > comparison.SameScore) {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "照片更接近其他学生，已阻止追加到当前学号"})
					continue
				}
				if comparison.SameFound && comparison.SameScore >= 0.995 {
					skipped++
					item.Committed = true
					results = append(results, map[string]any{"id": item.ID, "status": "skipped", "message": "与该学生已有样本几乎完全相同，无需重复保存"})
					continue
				}
				if _, err := s.store.AddFaceSampleWithVersion(r.Context(), existingBatchStudent.ID, angle, s.engine.ModelVersion(), face.Encode(item.Feature)); err != nil {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
					continue
				}
				batchAngles[batchKey][angle] = true
				item.Angle = angle
				item.Committed = true
				imported++
				results = append(results, map[string]any{"id": item.ID, "status": "imported", "message": "已为同一学号追加" + angle + "样本"})
				continue
			}

			comparison, err := s.compareFeature(r.Context(), item.Feature, 0)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if comparison.Other.ID != 0 && comparison.OtherScore >= s.currentRecognitionThresholds().MatchThreshold {
				failed++
				results = append(results, map[string]any{
					"id": item.ID, "status": "failed",
					"message": fmt.Sprintf("入库前复核发现该人脸已属于 %s，已阻止重复建立", comparison.Other.Name),
				})
				continue
			}
			student, _, err := s.store.CreateStudentWithFaceVersion(
				r.Context(), studentNo, name, className, angle, s.engine.ModelVersion(), face.Encode(item.Feature),
			)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": friendlyStudentError(err).Error()})
				continue
			}
			if batchKey != "" {
				batchStudents[batchKey] = student
				batchAngles[batchKey] = map[string]bool{angle: true}
			}
			item.Angle = angle
			item.Committed = true
			imported++
			results = append(results, map[string]any{"id": item.ID, "status": "imported", "message": "已建立学生并录入" + angle + "样本", "student": student})

		case "supplement":
			if !authUser(r).IsAdmin && !authUser(r).CanResampleFaces {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "没有补充或重新采样人脸的权限"})
				continue
			}
			targetID := int64(0)
			batchKey := ""
			fromCurrentBatch := false
			if item.Existing != nil && item.Existing.ID > 0 {
				targetID = item.Existing.ID
			} else {
				batchKey = photoImportStudentKey(requested.StudentNo)
				if batchKey == "" {
					batchKey = photoImportStudentKey(item.StudentNo)
				}
				if created, ok := batchStudents[batchKey]; batchKey != "" && ok {
					targetID = created.ID
					fromCurrentBatch = true
				}
			}
			if targetID <= 0 {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "没有可补充的已匹配学生"})
				continue
			}
			if fromCurrentBatch && batchAngles[batchKey][angle] {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "同一批次中该学生已经有该角度样本，请检查是否重复选择"})
				continue
			}

			comparison, err := s.compareFeature(r.Context(), item.Feature, targetID)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if comparison.Other.ID != 0 &&
				comparison.OtherScore >= s.currentRecognitionThresholds().MatchThreshold &&
				(!comparison.SameFound || comparison.OtherScore > comparison.SameScore) {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "照片更接近其他学生，已阻止错误补充"})
				continue
			}
			allowByStudentNo := item.MatchedByStudentNo && item.Existing != nil && item.Existing.ID == targetID
			if !fromCurrentBatch && !allowByStudentNo {
				if !comparison.SameFound || comparison.SameScore < s.currentRecognitionThresholds().MatchThreshold {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "照片与已匹配学生复核不一致"})
					continue
				}
			}
			if comparison.SameFound && comparison.SameScore >= 0.995 {
				skipped++
				item.Committed = true
				results = append(results, map[string]any{"id": item.ID, "status": "skipped", "message": "与已有样本几乎完全相同，无需重复保存"})
				continue
			}

			currentStudent, err := s.store.StudentByID(r.Context(), targetID)
			if err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if strings.TrimSpace(currentStudent.ClassName) != className &&
				(!authUser(r).IsAdmin && !authUser(r).CanEditStudents) {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": "没有修改学生班级的权限"})
				continue
			}
			if strings.TrimSpace(currentStudent.ClassName) != className {
				if err := s.store.UpdateStudentClass(r.Context(), targetID, className); err != nil {
					failed++
					results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
					continue
				}
			}
			if _, err := s.store.AddFaceSampleWithVersion(r.Context(), targetID, angle, s.engine.ModelVersion(), face.Encode(item.Feature)); err != nil {
				failed++
				results = append(results, map[string]any{"id": item.ID, "status": "failed", "message": err.Error()})
				continue
			}
			if fromCurrentBatch {
				batchAngles[batchKey][angle] = true
			}
			item.Angle = angle
			item.Committed = true
			imported++
			results = append(results, map[string]any{"id": item.ID, "status": "imported", "message": "已补充现有学生" + angle + "样本"})
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
	return assessImportedPhotoForAngle(img, detected, "正面")
}

func assessImportedPhotoForAngle(img image.Image, detected face.Detection, angle string) (string, string, bool) {
	if !validPhotoImportAngle(angle) {
		return "不合格", "未选择有效的人脸角度", false
	}
	bounds := img.Bounds()
	rect := detected.Rectangle.Intersect(bounds)
	if rect.Empty() || bounds.Dx() <= 0 || bounds.Dy() <= 0 {
		return "不合格", "未获得有效人脸区域", false
	}
	minSide := math.Min(float64(rect.Dx()), float64(rect.Dy()))
	faceRatio := float64(rect.Dx()*rect.Dy()) / float64(bounds.Dx()*bounds.Dy())
	if minSide < 96 || faceRatio < 0.025 {
		return "不合格", "人脸在照片中太小，建议使用更清晰、距离更近的照片", false
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

	switch angle {
	case "正面", "轻微抬头", "轻微低头":
		if yawOffset > 0.35 || mouthOffset > 0.40 {
			return "不合格", "检测到明显侧转，与所选正面/抬头/低头角度不一致", false
		}
	case "左侧", "右侧":
		if yawOffset < 0.12 {
			return "不合格", "照片接近正面，与所选侧脸角度不一致；请调整角度或重新选择标签", false
		}
		if yawOffset > 0.75 {
			return "不合格", "侧转角度过大，五点关键点不够稳定，请使用稍微侧转的清晰照片", false
		}
	case "其他":
		// Keep other angles available for manual review; geometry and image quality
		// checks below still apply.
	}
	if roll > 0.30 {
		return "不合格", "头部倾斜较大，请使用更端正的照片", false
	}

	if angle == "正面" && detected.Score >= 0.92 && minSide >= 160 && faceRatio >= 0.06 && yawOffset <= 0.18 && roll <= 0.15 {
		return "优", "正面度、清晰度和人脸占比良好", true
	}
	if angle == "左侧" || angle == "右侧" {
		return "合格", "侧脸角度与五点关键点检测基本匹配；请在预览中人工确认左右方向", true
	}
	if angle == "轻微抬头" || angle == "轻微低头" {
		return "合格", "清晰度和人脸关键点合格；抬头/低头方向请在预览中人工确认", true
	}
	if angle == "其他" {
		return "合格", "人脸质量合格；请人工确认该照片的具体角度", true
	}
	return "合格", "正面样本清晰度和关键点合格", true
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

func photoImportAngleNames() []string {
	return []string{"正面", "左侧", "右侧", "轻微抬头", "轻微低头", "其他"}
}

func validPhotoImportAngle(angle string) bool {
	angle = strings.TrimSpace(angle)
	for _, allowed := range photoImportAngleNames() {
		if angle == allowed {
			return true
		}
	}
	return false
}

func normalizePhotoImportAngle(folder string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(folder))
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	aliases := map[string]string{
		"正面": "正面", "front": "正面", "frontal": "正面", "frontface": "正面",
		"左侧": "左侧", "左脸": "左侧", "左侧脸": "左侧", "left": "左侧", "leftside": "左侧", "leftprofile": "左侧",
		"右侧": "右侧", "右脸": "右侧", "右侧脸": "右侧", "right": "右侧", "rightside": "右侧", "rightprofile": "右侧",
		"轻微抬头": "轻微抬头", "抬头": "轻微抬头", "仰头": "轻微抬头", "up": "轻微抬头", "upward": "轻微抬头", "lookup": "轻微抬头",
		"轻微低头": "轻微低头", "低头": "轻微低头", "俯头": "轻微低头", "down": "轻微低头", "downward": "轻微低头", "lookdown": "轻微低头",
		"其他": "其他", "other": "其他",
	}
	angle, ok := aliases[value]
	return angle, ok
}

func photoImportAngleHint(sourceName string) string {
	normalized := strings.ReplaceAll(sourceName, "\\", "/")
	dir := path.Dir(normalized)
	if dir == "." || dir == "/" {
		return ""
	}
	for _, part := range strings.Split(dir, "/") {
		if angle, ok := normalizePhotoImportAngle(part); ok {
			return angle
		}
	}
	return ""
}

func photoImportDefaultAngleName(r *http.Request) (string, error) {
	angle := strings.TrimSpace(r.FormValue("angle"))
	if angle == "" {
		return "正面", nil
	}
	if !validPhotoImportAngle(angle) {
		return "", errors.New("所选默认角度无效，请从角度列表中选择")
	}
	return angle, nil
}

func photoImportAngleForSource(sourceName, defaultAngle string) string {
	if hint := photoImportAngleHint(sourceName); hint != "" {
		return hint
	}
	if validPhotoImportAngle(defaultAngle) {
		return defaultAngle
	}
	return "正面"
}

func photoImportStudentKey(studentNo string) string {
	return strings.ToUpper(strings.TrimSpace(studentNo))
}

func photoImportSameStudentDifferentAngle(current, previous *photoImportItem) bool {
	if current == nil || previous == nil || current.Angle == previous.Angle {
		return false
	}
	currentNo := photoImportStudentKey(current.StudentNo)
	previousNo := photoImportStudentKey(previous.StudentNo)
	if currentNo != "" && currentNo == previousNo {
		return true
	}
	return current.Name != "" && previous.Name != "" && strings.TrimSpace(current.Name) == strings.TrimSpace(previous.Name)
}

func bestPhotoStudentScore(feature []float32, studentID int64, known []photoKnownFace) float64 {
	best := 0.0
	for _, candidate := range known {
		if candidate.Student.ID != studentID {
			continue
		}
		if score := face.Similarity(feature, candidate.Feature); score > best {
			best = score
		}
	}
	return best
}

func photoImportDefaultClassName(r *http.Request, classes map[string]bool) (string, error) {
	name := strings.TrimSpace(r.FormValue("class_name"))
	if name != "" && !classes[name] {
		return "", errors.New("所选默认班级不存在，请刷新班级列表后重新选择")
	}
	return name, nil
}

func photoImportClassForSource(sourceName, defaultClassName string, classes map[string]bool) string {
	// A matching class folder inside a ZIP is more specific than the batch
	// default. Direct uploads and ZIP root files use the supplied default.
	if hint := photoImportClassHint(sourceName, classes); hint != "" {
		return hint
	}
	if classes[defaultClassName] {
		return defaultClassName
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
		ID:                 item.ID,
		FileName:           item.FileName,
		Name:               item.Name,
		StudentNo:          item.StudentNo,
		ClassName:          item.ClassName,
		Angle:              item.Angle,
		MatchedByStudentNo: item.MatchedByStudentNo,
		Format:             item.Format,
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
