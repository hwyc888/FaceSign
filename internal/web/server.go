package web

import (
	"io/fs"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/hwyc888/FaceSign/internal/face"
	"github.com/hwyc888/FaceSign/internal/store"
)

type Server struct {
	logger             *slog.Logger
	store              *store.Store
	engine             *face.Engine
	matchThreshold     float64
	detectionThreshold float64
	version            string
	home               []byte
	static             http.Handler
	photoImportMu      sync.Mutex
	photoImports       map[string]*photoImportSession
}

func New(logger *slog.Logger, st *store.Store, engine *face.Engine, matchThreshold, detectionThreshold float64, version string) (*Server, error) {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return nil, err
	}
	home, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return nil, err
	}
	return &Server{
		logger:             logger,
		store:              st,
		engine:             engine,
		matchThreshold:     matchThreshold,
		detectionThreshold: detectionThreshold,
		version:            version,
		home:               home,
		static:             http.FileServer(http.FS(sub)),
		photoImports:       make(map[string]*photoImportSession),
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", s.health)
	mux.HandleFunc("/api/version", s.versionInfo)
	mux.HandleFunc("/api/students", s.students)
	mux.HandleFunc("/api/students/", s.studentAction)
	mux.HandleFunc("/api/classes", s.classes)
	mux.HandleFunc("/api/classes/", s.classAction)
	mux.HandleFunc("/api/enrollment/check", s.enrollmentCheck)
	mux.HandleFunc("/api/enrollment/create", s.createEnrollment)
	mux.HandleFunc("/api/recognize", s.recognize)
	mux.HandleFunc("/api/attendance", s.attendance)
	mux.HandleFunc("/api/photo-import/analyze", s.photoImportAnalyze)
	mux.HandleFunc("/api/photo-import/", s.photoImportAction)
	mux.HandleFunc("/", s.root)
	return s.logging(mux)
}

func (s *Server) logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("http request", "method", r.Method, "path", r.URL.Path, "elapsed", time.Since(started))
	})
}

func (s *Server) root(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-FaceSign-Version", s.version)
		s.static.ServeHTTP(w, r)
		return
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		methodNotAllowed(w)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-FaceSign-Version", s.version)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(s.home)
	}
}

