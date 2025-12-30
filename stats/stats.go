package stats

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type ProxyStats struct {
	Name            string `json:"name"`
	Protocol        string `json:"protocol"`
	ListenPort      int    `json:"listen_port"`
	TargetPort      int    `json:"target_port"`
	TotalUpload     int64  `json:"total_upload"`
	TotalDownload   int64  `json:"total_download"`
	MonthlyUpload   int64  `json:"monthly_upload"`
	MonthlyDownload int64  `json:"monthly_download"`
	CurrentMonth    string `json:"current_month"`
	Limit           int64  `json:"limit"` // 0 = unlimited
}

type StatsManager struct {
	mu    sync.RWMutex
	stats map[string]*ProxyStats
}

func NewStatsManager() *StatsManager {
	return &StatsManager{
		stats: make(map[string]*ProxyStats),
	}
}

func (m *StatsManager) Register(name, protocol string, listenPort, targetPort int, limit int64) *ProxyStats {
	m.mu.Lock()
	defer m.mu.Unlock()

	if s, exists := m.stats[name]; exists {
		// Update limit if changed in config
		s.Limit = limit
		return s
	}

	s := &ProxyStats{
		Name:         name,
		Protocol:     protocol,
		ListenPort:   listenPort,
		TargetPort:   targetPort,
		CurrentMonth: currentMonth(),
		Limit:        limit,
	}
	m.stats[name] = s
	return s
}

func (m *StatsManager) Get(name string) *ProxyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.stats[name]
}

func (m *StatsManager) GetAll() []*ProxyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]*ProxyStats, 0, len(m.stats))
	for _, s := range m.stats {
		result = append(result, s)
	}
	return result
}

func (m *StatsManager) SetStats(stats map[string]*ProxyStats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = stats
}

func (m *StatsManager) GetStatsMap() map[string]*ProxyStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ProxyStats)
	for k, v := range m.stats {
		result[k] = v
	}
	return result
}

func (s *ProxyStats) AddUpload(n int64) {
	s.checkMonthReset()
	atomic.AddInt64(&s.TotalUpload, n)
	atomic.AddInt64(&s.MonthlyUpload, n)
}

func (s *ProxyStats) AddDownload(n int64) {
	s.checkMonthReset()
	atomic.AddInt64(&s.TotalDownload, n)
	atomic.AddInt64(&s.MonthlyDownload, n)
}

func (s *ProxyStats) checkMonthReset() {
	current := currentMonth()
	if s.CurrentMonth != current {
		atomic.StoreInt64(&s.MonthlyUpload, 0)
		atomic.StoreInt64(&s.MonthlyDownload, 0)
		s.CurrentMonth = current
	}
}

func (s *ProxyStats) IsLimitExceeded() bool {
	if s.Limit <= 0 {
		return false
	}
	total := atomic.LoadInt64(&s.TotalUpload) + atomic.LoadInt64(&s.TotalDownload)
	return total >= s.Limit
}

func (s *ProxyStats) GetTotal() int64 {
	return atomic.LoadInt64(&s.TotalUpload) + atomic.LoadInt64(&s.TotalDownload)
}

func currentMonth() string {
	return time.Now().Format("2006-01")
}

func FormatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch {
	case bytes >= TB:
		return fmt.Sprintf("%.2f TB", float64(bytes)/TB)
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func ParseBytes(s string) (int64, error) {
	if s == "" || s == "0" {
		return 0, nil
	}

	s = strings.TrimSpace(strings.ToUpper(s))

	re := regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(B|KB|MB|GB|TB)?$`)
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return 0, fmt.Errorf("invalid byte format: %s", s)
	}

	value, err := strconv.ParseFloat(matches[1], 64)
	if err != nil {
		return 0, err
	}

	unit := matches[2]
	if unit == "" {
		unit = "B"
	}

	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
		TB = GB * 1024
	)

	switch unit {
	case "TB":
		value *= TB
	case "GB":
		value *= GB
	case "MB":
		value *= MB
	case "KB":
		value *= KB
	}

	return int64(value), nil
}
