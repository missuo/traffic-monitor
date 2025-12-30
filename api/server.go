package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"

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
	Name       string      `json:"name"`
	Protocol   string      `json:"protocol"`
	ListenPort int         `json:"listen_port"`
	TargetPort int         `json:"target_port"`
	Total      TrafficData `json:"total"`
	Monthly    MonthlyData `json:"monthly"`
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
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	r.Use(gin.Recovery())

	r.GET("/health", s.handleHealth)

	api := r.Group("/api")
	if s.token != "" {
		api.Use(s.authMiddleware())
	}
	{
		api.GET("/stats", s.handleStats)
		api.GET("/stats/:name", s.handleStatsByName)
	}

	s.server = &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: r,
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
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.server.Shutdown(ctx)
	}
	return nil
}

func (s *Server) authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		parts := strings.SplitN(auth, " ", 2)
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" || parts[1] != s.token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}

func (s *Server) handleHealth(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (s *Server) handleStats(c *gin.Context) {
	allStats := s.manager.GetAll()
	response := StatsResponse{
		Proxies: make([]ProxyStatsResponse, 0, len(allStats)),
	}

	for _, stat := range allStats {
		response.Proxies = append(response.Proxies, s.convertToResponse(stat))
	}

	c.JSON(http.StatusOK, response)
}

func (s *Server) handleStatsByName(c *gin.Context) {
	name := c.Param("name")
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "proxy name required"})
		return
	}

	stat := s.manager.Get(name)
	if stat == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "proxy not found"})
		return
	}

	c.JSON(http.StatusOK, s.convertToResponse(stat))
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
