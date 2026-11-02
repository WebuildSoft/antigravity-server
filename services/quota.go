package services

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"antigravity-bridge/config"
)

type QuotaItem struct {
	Model     string `json:"model"`
	LimitType string `json:"limitType"`
	Remaining int    `json:"remaining"`
	ResetsAt  string `json:"resetsAt"`
}

type QuotaResponse struct {
	Items     []QuotaItem `json:"items"`
	Timestamp string      `json:"timestamp"`
	Cached    bool        `json:"cached"`
}

var (
	quotaCache      *QuotaResponse
	quotaCacheTime  time.Time
	quotaMutex      sync.RWMutex
	quotaFetchMutex sync.Mutex
)

func StartQuotaBackgroundWorker() {
	go func() {
		// Warm up quota cache immediately on server startup
		_, _ = fetchQuotaFromAgy()

		// Refresh periodically in background every 45s without blocking any HTTP/UI requests
		ticker := time.NewTicker(45 * time.Second)
		for range ticker.C {
			_, _ = fetchQuotaFromAgy()
		}
	}()
}

func fetchQuotaFromAgy() (*QuotaResponse, error) {
	// Prevent duplicate concurrent executions of agy -p "/usage"
	if !quotaFetchMutex.TryLock() {
		quotaMutex.RLock()
		defer quotaMutex.RUnlock()
		if quotaCache != nil {
			return quotaCache, nil
		}
		return nil, fmt.Errorf("fetch in progress")
	}
	defer quotaFetchMutex.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, config.AppConfig.AgyBin, "-p", "/usage")
	out, err := cmd.Output()
	if err != nil {
		quotaMutex.RLock()
		if quotaCache != nil {
			fallback := *quotaCache
			fallback.Cached = true
			quotaMutex.RUnlock()
			return &fallback, nil
		}
		quotaMutex.RUnlock()
		return nil, fmt.Errorf("failed to fetch quota from agy: %v", err)
	}

	lines := strings.Split(string(out), "\n")
	var items []QuotaItem

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Quota:") {
			continue
		}

		var parts []string
		if strings.Contains(line, "\t") {
			parts = strings.Split(line, "\t")
		} else {
			for _, p := range strings.Split(line, "  ") {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					parts = append(parts, trimmed)
				}
			}
		}

		if len(parts) >= 4 {
			model := strings.TrimSpace(parts[0])
			limitType := strings.TrimSpace(parts[1])
			pctStr := strings.TrimSuffix(strings.TrimSpace(parts[2]), "%")
			remaining, _ := strconv.Atoi(pctStr)
			resetsAt := strings.TrimSpace(parts[3])

			items = append(items, QuotaItem{
				Model:     model,
				LimitType: limitType,
				Remaining: remaining,
				ResetsAt:  resetsAt,
			})
		}
	}

	if items == nil {
		items = []QuotaItem{}
	}

	res := &QuotaResponse{
		Items:     items,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Cached:    false,
	}

	quotaMutex.Lock()
	quotaCache = res
	quotaCacheTime = time.Now()
	quotaMutex.Unlock()

	// Broadcast updated quota to all active WebSocket clients so UI updates instantly
	if Manager != nil {
		Manager.BroadcastAll(map[string]interface{}{
			"type":  "quota_data",
			"quota": res,
		})
	}

	return res, nil
}

func GetQuotaData(forceRefresh bool) (*QuotaResponse, error) {
	if forceRefresh {
		return InvalidateAndRefreshQuota()
	}

	quotaMutex.RLock()
	cached := quotaCache
	quotaMutex.RUnlock()

	// If we already have quota in cache, return it INSTANTLY (<0.1ms)
	if cached != nil {
		if time.Since(quotaCacheTime) > 45*time.Second {
			// Trigger fresh update in background without blocking the client
			go fetchQuotaFromAgy()
		}
		cachedCopy := *cached
		cachedCopy.Cached = true
		return &cachedCopy, nil
	}

	// First time ever: fetch synchronously
	return fetchQuotaFromAgy()
}

// InvalidateQuotaCache clears the in-memory quota cache immediately.
func InvalidateQuotaCache() {
	quotaMutex.Lock()
	quotaCache = nil
	quotaMutex.Unlock()
}

// InvalidateAndRefreshQuota purges the current quota cache and fetches fresh quotas immediately.
func InvalidateAndRefreshQuota() (*QuotaResponse, error) {
	InvalidateQuotaCache()
	return fetchQuotaFromAgy()
}

