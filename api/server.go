package api

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"

	"traffic-monitor/stats"
)

type Server struct {
	port    int
	token   string
	manager *stats.StatsManager
	server  *http.Server
}

type StatsResponse struct {
	Proxies []ProxyStatsResponse `json:"proxies"`
}

type ProxyStatsResponse struct {
	Name       string        `json:"name"`
	Protocol   string        `json:"protocol"`
	ListenPort int           `json:"listen_port"`
	TargetPort int           `json:"target_port"`
	Total      TrafficData   `json:"total"`
	Monthly    MonthlyData   `json:"monthly"`
}

type TrafficData struct {
	Upload        int64  `json:"upload"`
	Download      int64  `json:"download"`
	UploadHuman   string `json:"upload_human"`
	DownloadHuman string `json:"download_human"`
}

type MonthlyData struct {
	Month         string `json:"month"`
	Upload        int64  `json:"upload"`
	Download      int64  `json:"download"`
	UploadHuman   string `json:"upload_human"`
	DownloadHuman string `json:"download_human"`
}

func NewServer(port int, token string, manager *stats.StatsManager) *Server {
	return &Server{
		port:    port,
		token:   token,
		manager: manager,
	}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/stats", s.authMiddleware(s.handleStats))
	mux.HandleFunc("/api/stats/", s.authMiddleware(s.handleStatsByName))
	mux.HandleFunc("/health", s.handleHealth)

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	log.Printf("[API] Server listening on :%d", s.port)

	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[API] Server error: %v", err)
		}
	}()

	return nil
}

func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.token != "" {
			auth := r.Header.Get("Authorization")
			if auth == "" {
				http.Error(w, `{"error": "missing authorization header"}`, http.StatusUnauthorized)
				return
			}

			parts := strings.SplitN(auth, " ", 2)
			if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" || parts[1] != s.token {
				http.Error(w, `{"error": "invalid token"}`, http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status": "ok"}`))
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	allStats := s.manager.GetAll()
	response := StatsResponse{
		Proxies: make([]ProxyStatsResponse, 0, len(allStats)),
	}

	for _, stat := range allStats {
		response.Proxies = append(response.Proxies, s.convertToResponse(stat))
	}

	s.writeJSON(w, response)
}

func (s *Server) handleStatsByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error": "method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	name := strings.TrimPrefix(r.URL.Path, "/api/stats/")
	if name == "" {
		http.Error(w, `{"error": "proxy name required"}`, http.StatusBadRequest)
		return
	}

	stat := s.manager.Get(name)
	if stat == nil {
		http.Error(w, `{"error": "proxy not found"}`, http.StatusNotFound)
		return
	}

	s.writeJSON(w, s.convertToResponse(stat))
}

func (s *Server) convertToResponse(stat *stats.ProxyStats) ProxyStatsResponse {
	totalUpload := atomic.LoadInt64(&stat.TotalUpload)
	totalDownload := atomic.LoadInt64(&stat.TotalDownload)
	monthlyUpload := atomic.LoadInt64(&stat.MonthlyUpload)
	monthlyDownload := atomic.LoadInt64(&stat.MonthlyDownload)

	return ProxyStatsResponse{
		Name:       stat.Name,
		Protocol:   stat.Protocol,
		ListenPort: stat.ListenPort,
		TargetPort: stat.TargetPort,
		Total: TrafficData{
			Upload:        totalUpload,
			Download:      totalDownload,
			UploadHuman:   stats.FormatBytes(totalUpload),
			DownloadHuman: stats.FormatBytes(totalDownload),
		},
		Monthly: MonthlyData{
			Month:         stat.CurrentMonth,
			Upload:        monthlyUpload,
			Download:      monthlyDownload,
			UploadHuman:   stats.FormatBytes(monthlyUpload),
			DownloadHuman: stats.FormatBytes(monthlyDownload),
		},
	}
}

func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
